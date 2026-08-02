// Package storage provides SQLite-backed persistence for Ballast's local
// session/state store (Account, Upload) plus the AES-256-GCM helpers used to
// encrypt OAuth tokens at rest, per Constitution Principle IV.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo (Constitution Principle VII)
)

const appDirName = "ballast"
const dbFileName = "ballast.db"

// DB wraps the underlying *sql.DB handle for Ballast's local store.
type DB struct {
	conn *sql.DB
}

// AppDataDir returns the OS-appropriate application data directory for
// Ballast, creating it if it does not already exist.
func AppDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("storage: resolve OS config dir: %w", err)
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("storage: create app data dir: %w", err)
	}
	return dir, nil
}

// Open creates (if necessary) and opens the SQLite database file in the OS
// app-data directory, then ensures the schema exists.
func Open() (*DB, error) {
	dir, err := AppDataDir()
	if err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(dir, dbFileName))
}

// OpenAt opens (creating if necessary) a SQLite database at an explicit
// path. Exposed separately from Open so tests can point at a temp file
// instead of the real OS app-data directory.
func OpenAt(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite db: %w", err)
	}
	// This feature has no concurrent writers by design (Scale/Scope in
	// plan.md: single user, one upload at a time), so a single connection
	// avoids SQLite's well-known "database is locked" surprises under
	// modernc.org/sqlite's driver.
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("storage: enable foreign keys: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.ensureSchema(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}
