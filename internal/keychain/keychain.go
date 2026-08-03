// Package keychain wraps OS-native credential storage (macOS Keychain,
// Windows Credential Manager, Linux Secret Service) via zalando/go-keyring
// to hold Ballast's AES-256-GCM data-encryption key.
//
// Per Constitution Principle IV, the key MUST NOT be stored in a file
// alongside the database. Per Constitution Principle VII and research.md
// §4, if the OS keychain is unavailable (most commonly: no Secret Service
// daemon running on a minimal/headless Linux session), this package fails
// closed with a clear, specific error rather than silently falling back to
// an unencrypted key file.
package keychain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	service  = "com.ballast.app"
	account  = "drive-token-encryption-key"
	keyBytes = 32 // AES-256
)

// ErrUnavailable is returned when the OS keychain cannot be read from or
// written to. Callers MUST surface this as a clear, actionable error to the
// user (research.md §4) and MUST NOT fall back to unencrypted storage.
var ErrUnavailable = errors.New("secure credential storage isn't available on this system; Ballast cannot sign in without it")

// GetOrCreateKey fetches Ballast's AES-256 data-encryption key from the OS
// keychain, generating and persisting a new random key on first use. It
// never writes the key anywhere other than the OS keychain.
func GetOrCreateKey() ([]byte, error) {
	existing, err := keyring.Get(service, account)
	switch {
	case err == nil:
		key, decodeErr := base64.StdEncoding.DecodeString(existing)
		if decodeErr != nil || len(key) != keyBytes {
			return nil, fmt.Errorf("keychain: stored key is corrupt: %w", ErrUnavailable)
		}
		return key, nil
	case errors.Is(err, keyring.ErrNotFound):
		return createAndStoreKey()
	default:
		// Any other error (no Secret Service daemon, permission denied,
		// D-Bus unavailable, etc.) is treated as "keychain unavailable" —
		// fail closed, per Principle VII's documented fallback.
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
}

func createAndStoreKey() ([]byte, error) {
	key := make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("keychain: generate encryption key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := keyring.Set(service, account, encoded); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return key, nil
}

// DeleteKey removes the data-encryption key from the OS keychain. Not
// currently invoked by sign-out (data-model.md: sign-out deletes the
// Account row, not the key — a future sign-in reuses the same key rather
// than re-provisioning one), but exposed for completeness and tests.
func DeleteKey() error {
	err := keyring.Delete(service, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain: delete key: %w", err)
	}
	return nil
}
