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
	created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')),
	display_name TEXT,
	picture_url TEXT
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
	ended_at DATETIME,
	drive_folder_name TEXT
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
	if err := d.migrateDriveFolderNameColumnIfNeeded(); err != nil {
		return err
	}
	if err := d.migrateAccountProfileColumnsIfNeeded(); err != nil {
		return err
	}
	return nil
}

// migrateDriveFolderNameColumnIfNeeded upgrades a pre-existing upload table
// missing this feature's drive_folder_name column (data-model.md). A plain
// ALTER TABLE ADD COLUMN suffices, same as migrateChunkSizeColumnsIfNeeded --
// no CHECK constraint involved. A no-op once the column already exists.
func (d *DB) migrateDriveFolderNameColumnIfNeeded() error {
	hasColumn, err := d.uploadTableHasColumn("drive_folder_name")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	if _, err := d.conn.Exec(`ALTER TABLE upload ADD COLUMN drive_folder_name TEXT`); err != nil {
		return fmt.Errorf("storage: migrate drive_folder_name column: %w", err)
	}
	return nil
}

// migrateAccountProfileColumnsIfNeeded upgrades a pre-existing account table
// missing this feature's display_name/picture_url columns (data-model.md).
// A no-op once display_name already exists.
func (d *DB) migrateAccountProfileColumnsIfNeeded() error {
	hasColumn, err := d.accountTableHasColumn("display_name")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	stmts := []string{
		`ALTER TABLE account ADD COLUMN display_name TEXT`,
		`ALTER TABLE account ADD COLUMN picture_url TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := d.conn.Exec(stmt); err != nil {
			return fmt.Errorf("storage: migrate account profile columns: %w", err)
		}
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
	return d.tableHasColumn("upload", name)
}

// accountTableHasColumn reports whether the account table (as it currently
// exists in the database) has a column with the given name.
func (d *DB) accountTableHasColumn(name string) (bool, error) {
	return d.tableHasColumn("account", name)
}

// tableHasColumn reports whether table (as it currently exists in the
// database) has a column with the given name. table is trusted, fixed
// call-site input, never user data.
func (d *DB) tableHasColumn(table, name string) (bool, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, fmt.Errorf("storage: inspect %s table: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var colName, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("storage: scan %s table_info: %w", table, err)
		}
		if colName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}
