package services

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestEncryptionService_LongHostnameWrongPassphrase(t *testing.T) {
	for _, length := range []int{0, 30, 32, 64, 255} {
		t.Run(fmt.Sprintf("hostname_length_%d", length), func(t *testing.T) {
			hostname := strings.Repeat("h", length)
			correct := &encryptionService{passphrase: hostname + "__correct-key"}
			wrong := &encryptionService{passphrase: hostname + "__wrong-key"}
			encrypted, err := correct.Encrypt("secret message")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := wrong.Decrypt(encrypted); err == nil {
				t.Fatal("Decrypt() must reject a wrong passphrase regardless of hostname length")
			}
		})
	}
}

func TestEncryptionService_LegacyCompatibility(t *testing.T) {
	// Fixtures captured from the original PHP-compatible CBC implementation.
	tests := []struct{ name, passphrase, ciphertext string }{
		{"short hostname", "legacy-host__legacy-key", "j412P20WhBYGsVgohMWRd1I2dVBLazJOaUpmaVkxK2xMamFNZy9rS20wQUhJOHlRV1pIcmxoaTFhZjA9"},
		{"long hostname", strings.Repeat("h", 32) + "__correct-key", "j1m2K4VGd1MiAAIVH0zEkzliTFFyYzJscVhJWDEzVUhWVHBOeDY1bUNobEFMWXFpM0Zjcyt4SlZSd2c9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &encryptionService{passphrase: tt.passphrase}
			plaintext, err := service.Decrypt(tt.ciphertext)
			if err != nil {
				t.Fatal(err)
			}
			if plaintext != "legacy secret message" {
				t.Fatalf("unexpected plaintext: %q", plaintext)
			}
			rewritten, err := service.Encrypt(plaintext)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(rewritten, "ssk:v2:") {
				t.Fatal("new writes must use the authenticated format")
			}
		})
	}
}

func TestEncryptionService_RejectsTampering(t *testing.T) {
	service := &encryptionService{passphrase: "test-host__test-key"}
	encoded, err := service.Encrypt("secret message")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "ssk:v2:") {
		t.Fatal("missing version prefix")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "ssk:v2:"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range payload {
		tampered := append([]byte(nil), payload...)
		tampered[i] ^= 1
		if _, err := service.Decrypt("ssk:v2:" + base64.StdEncoding.EncodeToString(tampered)); err == nil {
			t.Fatalf("accepted tampering at byte %d", i)
		}
	}
	for length := 0; length < len(payload); length++ {
		if _, err := service.Decrypt("ssk:v2:" + base64.StdEncoding.EncodeToString(payload[:length])); err == nil {
			t.Fatalf("accepted truncated payload of %d bytes", length)
		}
	}
}

func TestEncryptionService_RejectsMalformedLegacyCiphertext(t *testing.T) {
	service := &encryptionService{passphrase: "test-host__test-key"}
	for _, length := range []int{0, 1, 15, 17} {
		inner := base64.StdEncoding.EncodeToString(make([]byte, length))
		encoded := base64.StdEncoding.EncodeToString(append(make([]byte, 16), []byte(inner)...))
		if _, err := service.Decrypt(encoded); err == nil {
			t.Errorf("accepted %d legacy ciphertext bytes", length)
		}
	}
	for _, padding := range []byte{0, 17} {
		if _, err := service.pkcs7Unpad(append(make([]byte, 15), padding)); err == nil {
			t.Errorf("accepted padding byte %d", padding)
		}
	}
}
