package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// Encrypt AES-256-GCM-encrypts plaintext under key, using a freshly
// generated random nonce for this call. The nonce MUST be stored alongside
// the ciphertext (data-model.md's token_nonce column) and MUST NOT be
// reused for another encryption operation, including re-encryption of the
// same logical value.
//
// key must be exactly 32 bytes (AES-256). Pure function: no I/O, no
// dependency on the keychain or database packages, so it's testable in
// isolation (T009/T014).
func Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: new AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt reverses Encrypt: given the same key and the nonce that was
// generated for this specific ciphertext, returns the original plaintext.
// Returns an error if the key/nonce/ciphertext don't match (tampering,
// wrong key, or corrupt data) — GCM's authentication tag makes this
// detectable rather than silently returning garbage.
func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt (authentication failed or wrong key/nonce): %w", err)
	}
	return plaintext, nil
}
