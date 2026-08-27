// Package crypto provides cryptographic utilities for envelope encryption and data protection.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// EncryptedPrefix identifies ciphertext produced by envelope encryption.
	EncryptedPrefix = "enc:v1:"
	// NonceSize is the standard 12-byte nonce for AES-GCM.
	NonceSize = 12
)

// DeriveKey derives a 32-byte AES-256 key from any input passphrase/key using SHA-256.
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// EncryptString encrypts a plaintext string using AES-256-GCM.
// Returns "enc:v1:<base64(nonce + ciphertext + tag)>".
// If secret is empty, it returns the plaintext unmodified.
func EncryptString(plaintext, secret string) (string, error) {
	if secret == "" || plaintext == "" {
		return plaintext, nil
	}

	key := DeriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts an envelope-encrypted ciphertext ("enc:v1:<base64>").
// If the input does not start with "enc:v1:", it is treated as legacy cleartext and returned as-is.
// If secret is empty and ciphertext is encrypted, returns an error.
func DecryptString(ciphertext, secret string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	if !strings.HasPrefix(ciphertext, EncryptedPrefix) {
		// Cleartext / unencrypted fallback
		return ciphertext, nil
	}

	if secret == "" {
		return "", errors.New("cannot decrypt: encryption key is not configured")
	}

	rawB64 := strings.TrimPrefix(ciphertext, EncryptedPrefix)
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	key := DeriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm decrypt failed (invalid key or tampered data): %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted returns true if the value has the envelope encryption prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, EncryptedPrefix)
}
