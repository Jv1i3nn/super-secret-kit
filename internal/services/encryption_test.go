package services

import (
	"os"
	"path/filepath"
	"testing"
)

// createTestPassphraseFile creates a temporary passphrase file for testing
func createTestPassphraseFile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	passphrasePath := filepath.Join(tmpDir, ".passphrase")

	if err := os.WriteFile(passphrasePath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test passphrase file: %v", err)
	}

	return passphrasePath
}

func TestEncryptionService_RoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		passphrase string
		plaintext  string
	}{
		{
			name:       "simple text",
			passphrase: "test-secret-key-123",
			plaintext:  "hello world",
		},
		{
			name:       "special characters",
			passphrase: "complex-key!@#$%",
			plaintext:  "Special: àéè@#$%^&*()",
		},
		{
			name:       "long text",
			passphrase: "long-passphrase-key",
			plaintext:  "This is a much longer text that contains multiple sentences and should still be encrypted and decrypted correctly without any issues.",
		},
		{
			name:       "unicode",
			passphrase: "unicode-key",
			plaintext:  "Hello 世界 🌍 مرحبا",
		},
		{
			name:       "json-like data",
			passphrase: "json-key",
			plaintext:  `{"username":"test","password":"secret123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passphrasePath := createTestPassphraseFile(t, tt.passphrase)
			service, err := NewEncryptionService(passphrasePath)
			if err != nil {
				t.Fatalf("NewEncryptionService() error = %v", err)
			}

			// Encrypt
			encrypted, err := service.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if encrypted == "" {
				t.Error("Encrypt() returned empty string")
			}

			if encrypted == tt.plaintext {
				t.Error("Encrypt() returned plaintext unchanged")
			}

			// Decrypt
			decrypted, err := service.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptionService_ConsistentEncryption(t *testing.T) {
	passphrase := "consistent-key"
	plaintext := "test message"
	passphrasePath := createTestPassphraseFile(t, passphrase)
	service, err := NewEncryptionService(passphrasePath)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}

	// Encrypt same text twice
	encrypted1, err1 := service.Encrypt(plaintext)
	encrypted2, err2 := service.Encrypt(plaintext)

	if err1 != nil || err2 != nil {
		t.Fatalf("Encrypt() errors: %v, %v", err1, err2)
	}

	// Different encryptions due to random IV
	if encrypted1 == encrypted2 {
		t.Error("Multiple encryptions should produce different ciphertexts (random IV)")
	}

	// Both should decrypt to same plaintext
	decrypted1, err := service.Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Decrypt(encrypted1) error = %v", err)
	}
	decrypted2, err := service.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Decrypt(encrypted2) error = %v", err)
	}

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both encrypted values should decrypt to original plaintext")
	}
}

func TestEncryptionService_InvalidDecryption(t *testing.T) {
	passphrasePath := createTestPassphraseFile(t, "test-key")
	service, err := NewEncryptionService(passphrasePath)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}

	tests := []struct {
		name      string
		encrypted string
	}{
		{
			name:      "invalid base64",
			encrypted: "not-valid-base64!@#",
		},
		{
			name:      "too short",
			encrypted: "YWJj",
		},
		{
			name:      "random data",
			encrypted: "SGVsbG8gV29ybGQ=", // Valid base64 but invalid encrypted data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Decrypt(tt.encrypted)
			if err == nil {
				t.Error("Decrypt() should fail for invalid input")
			}
		})
	}
}

func TestEncryptionService_WrongPassphrase(t *testing.T) {
	plaintext := "secret message"

	// Encrypt with first passphrase
	passphrasePath1 := createTestPassphraseFile(t, "correct-key")
	service1, err := NewEncryptionService(passphrasePath1)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}
	encrypted, err := service1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Try to decrypt with different passphrase
	passphrasePath2 := createTestPassphraseFile(t, "wrong-key")
	service2, err := NewEncryptionService(passphrasePath2)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}
	decrypted, err := service2.Decrypt(encrypted)

	// Should either error or produce garbage (not original plaintext)
	if err == nil && decrypted == plaintext {
		t.Error("Decrypt() with wrong passphrase should not produce correct plaintext")
	}
}

func TestEncryptionService_EmptyPassphrase(t *testing.T) {
	passphrasePath := createTestPassphraseFile(t, "")
	service, err := NewEncryptionService(passphrasePath)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}
	plaintext := "test"

	encrypted, err := service.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() with empty passphrase error = %v", err)
	}

	decrypted, err := service.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() with empty passphrase error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Round trip with empty passphrase failed")
	}
}

func TestEncryptionService_LargeData(t *testing.T) {
	passphrasePath := createTestPassphraseFile(t, "large-data-key")
	service, err := NewEncryptionService(passphrasePath)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}

	// Generate large plaintext (1MB)
	plaintext := ""
	for i := 0; i < 1024*64; i++ { // 64KB should be enough for testing
		plaintext += "Lorem ipsum dolor sit amet "
	}

	encrypted, err := service.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() large data error = %v", err)
	}

	decrypted, err := service.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() large data error = %v", err)
	}

	if decrypted != plaintext {
		t.Error("Large data round trip failed")
	}
}

func BenchmarkEncryptionService_Encrypt(b *testing.B) {
	tmpDir := b.TempDir()
	passphrasePath := filepath.Join(tmpDir, ".passphrase")
	_ = os.WriteFile(passphrasePath, []byte("benchmark-key"), 0600)

	service, _ := NewEncryptionService(passphrasePath)
	plaintext := "benchmark test message for encryption performance"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.Encrypt(plaintext)
	}
}

func BenchmarkEncryptionService_Decrypt(b *testing.B) {
	tmpDir := b.TempDir()
	passphrasePath := filepath.Join(tmpDir, ".passphrase")
	_ = os.WriteFile(passphrasePath, []byte("benchmark-key"), 0600)

	service, _ := NewEncryptionService(passphrasePath)
	plaintext := "benchmark test message for decryption performance"
	encrypted, _ := service.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.Decrypt(encrypted)
	}
}
