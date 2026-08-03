// Package storage provides SQLite-backed persistence for Ballast's local
// session/state store (Account, Upload) plus the AES-256-GCM helpers used
// to encrypt OAuth tokens at rest.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo
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
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return OpenAt(path)
}

// DefaultPath returns the path Open uses -- exposed so a caller that needs
// to remember it (e.g. to reopen the same file, simulating a process
// restart for crash-recovery E2E testing) doesn't have to duplicate it.
func DefaultPath() (string, error) {
	dir, err := AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFileName), nil
}

// OpenAt opens (creating if necessary) a SQLite database at an explicit
// path. Exposed separately from Open so tests can point at a temp file
// instead of the real OS app-data directory.
func OpenAt(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite db: %w", err)
	}
	// A single connection avoids SQLite's "database is locked" errors, since this app never writes concurrently.
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
