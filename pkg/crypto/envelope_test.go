package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestSM4StandardVector tests the standard GB/T 32907-2016 single block encryption/decryption.
func TestSM4StandardVector(t *testing.T) {
	keyHex := "0123456789abcdeffedcba9876543210"
	plainHex := "0123456789abcdeffedcba9876543210"
	cipherHex := "a39c462feee46b964d80175d3294b6bd"

	key, _ := hex.DecodeString(keyHex)
	plain, _ := hex.DecodeString(plainHex)
	expectedCipher, _ := hex.DecodeString(cipherHex)

	block, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	dst := make([]byte, BlockSize)
	block.Encrypt(dst, plain)
	if !bytes.Equal(dst, expectedCipher) {
		t.Fatalf("SM4 encryption mismatch: got %x, want %x", dst, expectedCipher)
	}

	dec := make([]byte, BlockSize)
	block.Decrypt(dec, dst)
	if !bytes.Equal(dec, plain) {
		t.Fatalf("SM4 decryption mismatch: got %x, want %x", dec, plain)
	}
}

func TestEnvelopeEncryption(t *testing.T) {
	secret := "privshield-master-key-2026"
	plaintext := "{\"patient_name\":\"张三\",\"id_card\":\"510101199001011234\"}"

	// 1. Encrypt and verify format
	encrypted, err := EncryptString(plaintext, secret)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}
	if !IsEncrypted(encrypted) {
		t.Fatalf("expected encrypted string to have prefix %q, got %q", EncryptedPrefix, encrypted)
	}
	if encrypted == plaintext {
		t.Fatal("ciphertext must not match plaintext")
	}

	// 2. Decrypt with correct key
	decrypted, err := DecryptString(encrypted, secret)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted text mismatch: got %q, want %q", decrypted, plaintext)
	}

	// 3. Decrypt with wrong key must fail
	_, errWrongKey := DecryptString(encrypted, "wrong-key-value")
	if errWrongKey == nil {
		t.Fatal("expected decryption failure with incorrect key")
	}

	// 4. Legacy cleartext compatibility (unencrypted string passed to DecryptString)
	legacyCleartext := "legacy_unencrypted_sample"
	out, err := DecryptString(legacyCleartext, secret)
	if err != nil || out != legacyCleartext {
		t.Fatalf("legacy cleartext should return as-is, got %q, err=%v", out, err)
	}

	// 5. Empty secret returns plaintext
	noKeyEnc, err := EncryptString(plaintext, "")
	if err != nil || noKeyEnc != plaintext {
		t.Fatalf("empty key should return plaintext, got %q, err=%v", noKeyEnc, err)
	}
}
