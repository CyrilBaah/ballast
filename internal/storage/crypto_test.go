package storage

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("super-secret-oauth-token-value")

	ciphertext, nonce, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}
	if len(nonce) == 0 {
		t.Fatal("nonce must not be empty")
	}

	got, err := Decrypt(key, ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestEncryptNonceNeverReused(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("same-plaintext-both-times")

	_, nonce1, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}
	_, nonce2, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}
	if bytes.Equal(nonce1, nonce2) {
		t.Fatal("two independent Encrypt calls produced the same nonce")
	}
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	key := testKey(t)
	wrongKey := testKey(t)
	plaintext := []byte("secret")

	ciphertext, nonce, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if _, err := Decrypt(wrongKey, ciphertext, nonce); err == nil {
		t.Fatal("expected Decrypt to fail with the wrong key, got nil error")
	}
}

func TestDecryptFailsOnTamperedCiphertext(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("secret")

	ciphertext, nonce, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := append([]byte(nil), ciphertext...)
	tampered[0] ^= 0xFF

	if _, err := Decrypt(key, tampered, nonce); err == nil {
		t.Fatal("expected Decrypt to fail on tampered ciphertext (GCM auth tag), got nil error")
	}
}

func TestEncryptRejectsInvalidKeySize(t *testing.T) {
	if _, _, err := Encrypt([]byte("too-short"), []byte("data")); err == nil {
		t.Fatal("expected Encrypt to reject a non-32-byte key")
	}
}
