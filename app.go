package main

import (
	"context"

	"ballast/internal/keychain"
	"ballast/internal/logging"
	"ballast/internal/storage"
)

// App is the single struct Wails binds to the frontend. Its methods are the
// entire Go<->TS contract for this feature (contracts/wails-bindings.md).
//
// Naming note: the contract documents methods under conceptual namespaces
// ("Auth.GetStatus", "Files.PickLocal", "Drive.ListFolders",
// "Upload.Start"/"Upload.GetStatus") for readability, but Go methods on one
// struct can't collide on a bare name (Auth.GetStatus vs Upload.GetStatus).
// Each method below is prefixed with its contract namespace
// (AuthGetStatus, FilesPickLocal, DriveListFolders, UploadStart,
// UploadGetStatus, ...) instead; the frontend's screen modules re-export
// them under the contract's shorter names so call sites read like the
// contract (see frontend/src/screens/*.ts).
type App struct {
	ctx context.Context
	db  *storage.DB
	// encKey is the AES-256 data-encryption key for token ciphertext
	// columns, fetched from the OS keychain at startup. It is held only in
	// memory for the process lifetime and is never logged (Constitution
	// Principle IV) or written to any file.
	encKey []byte
}

// NewApp creates a new App application struct. Construction does not touch
// the database or keychain — that happens in startup, so NewApp itself
// can't fail and stays trivially testable.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so we can
// call Wails runtime methods (event emission, dialogs) from bound methods.
//
// Per Constitution Principle VII / research.md §4: if the OS keychain is
// unavailable, we do NOT fall back to an unencrypted key file. The app
// still starts (so a signed-out user can at least see a clear error rather
// than a crash), but any method that needs the encryption key will surface
// keychain.ErrUnavailable until the underlying OS issue is resolved.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	db, err := storage.Open()
	if err != nil {
		// No credentials in this log line — just a path/OS-level error.
		logging.Error("failed to open local database", "error", err)
	} else {
		a.db = db
	}

	key, err := keychain.GetOrCreateKey()
	if err != nil {
		// Expected/handled case on a subset of Linux systems (research.md
		// §4) — logged at Warn, not Error, and never includes key
		// material (there is none to include: GetOrCreateKey failed
		// before any key existed in this process).
		logging.Warn("OS keychain unavailable; sign-in will fail closed until this is resolved", "error", err)
		return
	}
	a.encKey = key
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			logging.Warn("error closing database", "error", err)
		}
	}
}
