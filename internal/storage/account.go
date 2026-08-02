package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// singleAccountID is the only valid Account.id in this feature. Multi
// -account support is out of scope (spec.md Assumptions); enforced here at
// the application layer rather than as a DB constraint, since that's a
// concern of a future feature's data model, not this one's.
const singleAccountID = 1

// ErrNoAccount is returned by GetAccount when no account row exists (the
// signed-out state).
var ErrNoAccount = errors.New("storage: no account is signed in")

// Account is the in-memory representation of the single Account row
// (data-model.md).
type Account struct {
	GoogleUserID           string
	Email                  string
	AccessTokenCiphertext  []byte
	AccessTokenNonce       []byte
	RefreshTokenCiphertext []byte
	RefreshTokenNonce      []byte
	AccessTokenExpiry      time.Time
	CreatedAt              time.Time
}

// UpsertAccount creates or replaces the single Account row. Sign-in success
// (data-model.md: absent -> signed_in) always results in exactly one row
// with id=1 -- if a stale row somehow exists (it shouldn't, since sign-out
// deletes outright), it is replaced rather than left to coexist.
func (d *DB) UpsertAccount(a Account) error {
	_, err := d.conn.Exec(`
		INSERT INTO account (
			id, google_user_id, email,
			access_token_ciphertext, access_token_nonce,
			refresh_token_ciphertext, refresh_token_nonce,
			access_token_expiry, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			google_user_id = excluded.google_user_id,
			email = excluded.email,
			access_token_ciphertext = excluded.access_token_ciphertext,
			access_token_nonce = excluded.access_token_nonce,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			refresh_token_nonce = excluded.refresh_token_nonce,
			access_token_expiry = excluded.access_token_expiry
	`,
		singleAccountID, a.GoogleUserID, a.Email,
		a.AccessTokenCiphertext, a.AccessTokenNonce,
		a.RefreshTokenCiphertext, a.RefreshTokenNonce,
		formatTime(a.AccessTokenExpiry), formatTime(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("storage: upsert account: %w", err)
	}
	return nil
}

// GetAccount returns the single Account row, or ErrNoAccount if the app is
// signed out.
func (d *DB) GetAccount() (*Account, error) {
	row := d.conn.QueryRow(`
		SELECT google_user_id, email,
			access_token_ciphertext, access_token_nonce,
			refresh_token_ciphertext, refresh_token_nonce,
			access_token_expiry, created_at
		FROM account WHERE id = ?
	`, singleAccountID)

	var a Account
	var expiry, created string
	err := row.Scan(
		&a.GoogleUserID, &a.Email,
		&a.AccessTokenCiphertext, &a.AccessTokenNonce,
		&a.RefreshTokenCiphertext, &a.RefreshTokenNonce,
		&expiry, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoAccount
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get account: %w", err)
	}
	a.AccessTokenExpiry, err = parseTime(expiry)
	if err != nil {
		return nil, fmt.Errorf("storage: parse access_token_expiry: %w", err)
	}
	a.CreatedAt, err = parseTime(created)
	if err != nil {
		return nil, fmt.Errorf("storage: parse created_at: %w", err)
	}
	return &a, nil
}

// DeleteAccount removes the Account row outright (data-model.md: sign-out
// deletes the row, it does not flag it -- so a stale token can never be
// read after sign-out). Safe to call when no account exists.
func (d *DB) DeleteAccount() error {
	_, err := d.conn.Exec(`DELETE FROM account WHERE id = ?`, singleAccountID)
	if err != nil {
		return fmt.Errorf("storage: delete account: %w", err)
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
