package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

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

	block, err := aes.NewCipher([]byte(s.get32ByteKey()))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	plaintextBytes := s.pkcs7Pad([]byte(plaintext), aes.BlockSize)
	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(plaintextBytes))
	mode.CryptBlocks(ciphertext, plaintextBytes)

	// PHP's openssl_encrypt returns base64, so we match that behavior:
	// base64(IV + base64(encrypted))
	encryptedBase64 := base64.StdEncoding.EncodeToString(ciphertext)
	result := append(iv, []byte(encryptedBase64)...)
	return base64.StdEncoding.EncodeToString(result), nil
}

func (s *encryptionService) Decrypt(encodedCiphertext string) (string, error) {
	if encodedCiphertext == "" {
		return "", nil
	}

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

func (s *encryptionService) pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

func (s *encryptionService) pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("invalid padding: empty data")
	}

	padding := int(data[length-1])
	if padding > length || padding > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding size")
	}

	for i := 0; i < padding; i++ {
		if data[length-1-i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:length-padding], nil
}
