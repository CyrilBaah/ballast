package storage

import "fmt"

// schema defines the Account and Upload tables per data-model.md. Both
// tables belong to this feature only — no session-offset/chunk-state
// columns exist here, since resumable-engine persistence is a later
// feature's data model (see data-model.md's preamble).
const schema = `
-- Deviation from data-model.md: that doc lists a single shared
-- token_nonce column, but its own validation rule requires a nonce
-- "never reused across access/refresh token or across re-encryption" --
-- which a single shared column can't satisfy for two independently
-- encrypted values. Using two per-token nonce columns instead is the
-- literal, safe reading of that rule and avoids a real AES-GCM nonce-reuse
-- bug (Constitution Principle IV).
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
