package storage

import "fmt"

// schema defines the Account and Upload tables.
const schema = `
-- Separate nonce columns per token, since a shared nonce column can't
-- satisfy AES-GCM's rule against reusing a nonce across two encrypted values.
CREATE TABLE IF NOT EXISTS account (
	id INTEGER PRIMARY KEY,
	google_user_id TEXT NOT NULL,
	email TEXT NOT NULL,
	access_token_ciphertext BLOB NOT NULL,
	access_token_nonce BLOB NOT NULL,
	refresh_token_ciphertext BLOB NOT NULL,
	refresh_token_nonce BLOB NOT NULL,
	access_token_expiry DATETIME NOT NULL,
	created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS upload (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	local_path TEXT NOT NULL,
	local_size_bytes INTEGER NOT NULL,
	drive_folder_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'succeeded', 'failed')),
	bytes_sent INTEGER NOT NULL DEFAULT 0,
	drive_file_id TEXT,
	drive_file_link TEXT,
	failure_reason TEXT,
	started_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')),
	ended_at DATETIME
);
`

// ensureSchema creates the account/upload tables if they don't already
// exist. Idempotent — safe to call on every app launch.
func (d *DB) ensureSchema() error {
	if _, err := d.conn.Exec(schema); err != nil {
		return fmt.Errorf("storage: create schema: %w", err)
	}
	return nil
}
