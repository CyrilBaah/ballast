package storage

import (
	"database/sql"
	"fmt"
)

const schemaAccountTable = `
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
`

// schemaUploadTable is the full, current shape of the upload table
// (data-model.md), used both to create it fresh and, via
// migrateUploadTableIfNeeded, to rebuild it in place when upgrading a
// pre-existing Feature-001-shape table that lacks this feature's columns
// and widened status CHECK constraint (SQLite has no ALTER ... DROP
// CONSTRAINT, so widening a CHECK requires recreating the table).
const schemaUploadTable = `
CREATE TABLE IF NOT EXISTS upload (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	local_path TEXT NOT NULL,
	local_size_bytes INTEGER NOT NULL,
	local_mtime DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')),
	drive_folder_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'paused', 'awaiting_confirmation', 'cancelled', 'succeeded', 'failed')),
	bytes_sent INTEGER NOT NULL DEFAULT 0,
	session_uri TEXT,
	content_hash_state BLOB,
	awaiting_confirmation_reason TEXT CHECK (awaiting_confirmation_reason IN ('session_expired', 'file_changed') OR awaiting_confirmation_reason IS NULL),
	chunk_size_bytes INTEGER NOT NULL DEFAULT 8388608,
	consecutive_chunk_successes INTEGER NOT NULL DEFAULT 0,
	drive_file_id TEXT,
	drive_file_link TEXT,
	failure_reason TEXT,
	started_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')),
	ended_at DATETIME
);
`

const schema = schemaAccountTable + schemaUploadTable

// ensureSchema creates the account/upload tables if they don't already
// exist, then upgrades an existing pre-Feature-002 or pre-Feature-003
// upload table in place. Idempotent -- safe to call on every app launch.
func (d *DB) ensureSchema() error {
	if _, err := d.conn.Exec(schema); err != nil {
		return fmt.Errorf("storage: create schema: %w", err)
	}
	if err := d.migrateUploadTableIfNeeded(); err != nil {
		return err
	}
	if err := d.migrateChunkSizeColumnsIfNeeded(); err != nil {
		return err
	}
	return nil
}

// migrateChunkSizeColumnsIfNeeded upgrades a pre-existing upload table
// (already at Feature 002's shape) that's missing this feature's
// chunk_size_bytes/consecutive_chunk_successes columns, detected via
// chunk_size_bytes's absence. Unlike migrateUploadTableIfNeeded, no CHECK
// constraint needs widening here, so a plain ALTER TABLE ADD COLUMN
// suffices -- no rename-recreate-copy dance required. A no-op once
// chunk_size_bytes already exists (including immediately after
// migrateUploadTableIfNeeded has just created it fresh via schemaUploadTable).
func (d *DB) migrateChunkSizeColumnsIfNeeded() error {
	hasColumn, err := d.uploadTableHasColumn("chunk_size_bytes")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	stmts := []string{
		`ALTER TABLE upload ADD COLUMN chunk_size_bytes INTEGER NOT NULL DEFAULT 8388608`,
		`ALTER TABLE upload ADD COLUMN consecutive_chunk_successes INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range stmts {
		if _, err := d.conn.Exec(stmt); err != nil {
			return fmt.Errorf("storage: migrate chunk-size columns: %w", err)
		}
	}
	return nil
}

// migrateUploadTableIfNeeded upgrades a pre-existing upload table missing
// this feature's columns (detected via the local_mtime column's absence)
// by rebuilding it with schemaUploadTable's shape and copying every
// existing row across, defaulting the new columns. A no-op once
// local_mtime already exists.
func (d *DB) migrateUploadTableIfNeeded() error {
	hasColumn, err := d.uploadTableHasColumn("local_mtime")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("storage: begin upload table migration: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		`ALTER TABLE upload RENAME TO upload_pre_002`,
		schemaUploadTable,
		`INSERT INTO upload (
			id, local_path, local_size_bytes, drive_folder_id, status, bytes_sent,
			drive_file_id, drive_file_link, failure_reason, started_at, ended_at
		)
		SELECT
			id, local_path, local_size_bytes, drive_folder_id, status, bytes_sent,
			drive_file_id, drive_file_link, failure_reason, started_at, ended_at
		FROM upload_pre_002`,
		`DROP TABLE upload_pre_002`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("storage: migrate upload table: %w", err)
		}
	}
	return tx.Commit()
}

// uploadTableHasColumn reports whether the upload table (as it currently
// exists in the database) has a column with the given name.
func (d *DB) uploadTableHasColumn(name string) (bool, error) {
	rows, err := d.conn.Query(`PRAGMA table_info(upload)`)
	if err != nil {
		return false, fmt.Errorf("storage: inspect upload table: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var colName, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("storage: scan upload table_info: %w", err)
		}
		if colName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}
