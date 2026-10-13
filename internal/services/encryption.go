package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const encryptionPrefixV2 = "ssk:v2:"

type EncryptionService interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type encryptionService struct {
	passphrase string
}

func NewEncryptionService(passphrasePath string) (EncryptionService, error) {
	data, err := os.ReadFile(passphrasePath)
	if err != nil {
		return nil, fmt.Errorf("passphrase file not found: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	passphrase := hostname + "__" + string(data)

	return &encryptionService{
		passphrase: passphrase,
	}, nil
}

func (s *encryptionService) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	aead, err := s.authenticatedCipher()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	payload := aead.Seal(nonce, nonce, []byte(plaintext), []byte(encryptionPrefixV2))
	return encryptionPrefixV2 + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *encryptionService) Decrypt(encodedCiphertext string) (string, error) {
	if encodedCiphertext == "" {
		return "", nil
	}
	if strings.HasPrefix(encodedCiphertext, encryptionPrefixV2) {
		payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encodedCiphertext, encryptionPrefixV2))
		if err != nil {
			return "", fmt.Errorf("failed to decode encrypted data: %w", err)
		}
		aead, err := s.authenticatedCipher()
		if err != nil {
			return "", err
		}
		if len(payload) < aead.NonceSize()+aead.Overhead() {
			return "", fmt.Errorf("ciphertext too short")
		}
		plaintext, err := aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], []byte(encryptionPrefixV2))
		if err != nil {
			return "", fmt.Errorf("failed to authenticate encrypted data: %w", err)
		}
		return string(plaintext), nil
	}
	return s.decryptLegacy(encodedCiphertext)
}

func (s *encryptionService) authenticatedCipher() (cipher.AEAD, error) {
	// Hash the full hostname and passphrase; truncation could discard the secret.
	key := sha256.Sum256([]byte(s.passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated cipher: %w", err)
	}
	return aead, nil
}

// decryptLegacy reads the PHP-compatible base64(IV + base64(AES-CBC)) format.
// Its historical key derivation is preserved only for reading existing data.
func (s *encryptionService) decryptLegacy(encodedCiphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(data) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	iv := data[:aes.BlockSize]
	encryptedBase64 := data[aes.BlockSize:]

	ciphertext, err := base64.StdEncoding.DecodeString(string(encryptedBase64))
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted data: %w", err)
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid ciphertext length")
	}

	block, err := aes.NewCipher([]byte(s.get32ByteKey()))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext, err = s.pkcs7Unpad(plaintext)
	if err != nil {
		return "", fmt.Errorf("failed to unpad: %w", err)
	}

	return string(plaintext), nil
}

func (s *encryptionService) get32ByteKey() string {
	key := s.passphrase
	if len(key) > 32 {
		return key[:32]
	}
	for len(key) < 32 {
		key += " "
	}
	return key
}

func (s *encryptionService) pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("invalid padding: empty data")
	}

	padding := int(data[length-1])
	if padding == 0 || padding > length || padding > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding size")
	}

	for i := 0; i < padding; i++ {
		if data[length-1-i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:length-padding], nil
}
