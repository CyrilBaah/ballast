package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ballast/internal/auth"
	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/keychain"
	"ballast/internal/logging"
	"ballast/internal/storage"

	"github.com/pkg/browser"
	oauth2pkg "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

	// userInfoURL and revokeEndpoint default to Google's real endpoints;
	// the E2E-mocked dev build (mock_e2e.go, active only when
	// BALLAST_E2E_MOCK is set) points them at an in-process fake server so
	// Playwright can exercise the full sign-in contract without a real
	// Google account or a real browser window (research.md §5).
	userInfoURL           string
	revokeEndpoint        string
	openBrowser           auth.BrowserOpener
	oauthEndpointOverride *oauth2pkg.Endpoint
	// driveAPIEndpointOverride points the Drive API client at an
	// in-process mock server in the E2E-mocked dev build (mock_e2e.go);
	// empty in production, which uses the real Drive API.
	driveAPIEndpointOverride string
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
	a.userInfoURL = auth.DefaultUserInfoURL
	a.revokeEndpoint = auth.GoogleRevokeEndpoint
	a.openBrowser = browser.OpenURL
	maybeInstallE2EMock(a)

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

// --- OAuth configuration -------------------------------------------------

// oauthConfig builds the Google OAuth 2.0 desktop-app client configuration.
// Client ID/secret are read from the environment rather than hardcoded:
// this repo does not embed real Google Cloud OAuth client credentials (none
// were available to provision in this environment -- see the feature's
// final implementation report). A real distribution build sets
// BALLAST_GOOGLE_CLIENT_ID / BALLAST_GOOGLE_CLIENT_SECRET at build or
// install time.
func (a *App) oauthConfig() *oauth2pkg.Config {
	endpoint := google.Endpoint
	if a.oauthEndpointOverride != nil {
		endpoint = *a.oauthEndpointOverride
	}
	return &oauth2pkg.Config{
		ClientID:     os.Getenv("BALLAST_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("BALLAST_GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{auth.DriveFileScope, auth.DriveMetadataReadonlyScope},
		Endpoint:     endpoint,
	}
}

// errSignedOut is returned by Files/Drive/Upload methods when called while
// signed out (FR-001) -- a distinct, recognizable error rather than a
// generic failure, so the frontend can surface "sign in first" messaging
// (contracts/wails-bindings.md's Upload.Start note).
var errSignedOut = errors.New("not signed in")

// --- Auth.* (contracts/wails-bindings.md) ---------------------------------

// AuthGetStatus returns the current session state (contract: Auth.GetStatus).
// If the stored access token is near expiry, it is silently refreshed
// before returning signedIn:true (data-model.md: access_token_expiry
// "used to decide whether a silent refresh is needed"). If refresh fails
// (revoked/expired grant -- Edge Case in spec.md), the session is cleared
// and signedIn:false is returned.
func (a *App) AuthGetStatus() events.AuthStatus {
	if a.db == nil {
		return events.AuthStatus{SignedIn: false}
	}
	acct, err := a.db.GetAccount()
	if err != nil {
		return events.AuthStatus{SignedIn: false}
	}

	if auth.NeedsRefresh(acct.AccessTokenExpiry) {
		if err := a.silentlyRefresh(acct); err != nil {
			logging.Warn("silent token refresh failed; clearing local session", "error", err)
			_ = a.db.DeleteAccount()
			status := events.AuthStatus{SignedIn: false}
			events.EmitAuthChanged(a.ctx, status)
			return status
		}
	}

	return events.AuthStatus{SignedIn: true, Email: acct.Email}
}

// AuthSignIn starts the OAuth loopback flow (contract: Auth.SignIn).
func (a *App) AuthSignIn() (events.AuthStatus, error) {
	if a.db == nil {
		return events.AuthStatus{}, fmt.Errorf("auth: local database is unavailable")
	}
	if a.encKey == nil {
		return events.AuthStatus{}, keychain.ErrUnavailable
	}

	session, err := auth.SignIn(a.ctx, a.oauthConfig(), a.openBrowser, a.userInfoURL)
	if err != nil {
		return events.AuthStatus{}, err
	}
	if session.Cancelled {
		// No partial session (Edge Case) -- not an error/rejection, a
		// valid, expected outcome per contracts/wails-bindings.md.
		return events.AuthStatus{SignedIn: false}, nil
	}

	if err := a.persistSession(session); err != nil {
		return events.AuthStatus{}, err
	}

	status := events.AuthStatus{SignedIn: true, Email: session.Email}
	events.EmitAuthChanged(a.ctx, status)
	return status, nil
}

// AuthSignOut revokes the OAuth grant server-side and clears the local
// session (contract: Auth.SignOut; FR-003).
func (a *App) AuthSignOut() error {
	if a.db == nil {
		return fmt.Errorf("auth: local database is unavailable")
	}
	acct, err := a.db.GetAccount()
	if errors.Is(err, storage.ErrNoAccount) {
		events.EmitAuthChanged(a.ctx, events.AuthStatus{SignedIn: false})
		return nil
	}
	if err != nil {
		return err
	}

	var refreshToken string
	if a.encKey != nil {
		if plain, decErr := storage.Decrypt(a.encKey, acct.RefreshTokenCiphertext, acct.RefreshTokenNonce); decErr == nil {
			refreshToken = string(plain)
		} else {
			logging.Warn("could not decrypt stored refresh token for revocation; proceeding with local sign-out anyway", "error", decErr)
		}
	}

	err = auth.SignOut(a.ctx, http.DefaultClient, a.revokeEndpoint, refreshToken, a.db.DeleteAccount)
	events.EmitAuthChanged(a.ctx, events.AuthStatus{SignedIn: false})
	return err
}

// requireSignedIn is the FR-001 access-control gate for Files/Drive/Upload
// methods: file selection, folder browsing, and upload capabilities are
// unavailable until sign-in completes.
func (a *App) requireSignedIn() error {
	if a.db == nil {
		return fmt.Errorf("auth: local database is unavailable")
	}
	if _, err := a.db.GetAccount(); err != nil {
		return errSignedOut
	}
	return nil
}

// persistSession encrypts and stores a completed sign-in's tokens.
func (a *App) persistSession(session *auth.Session) error {
	accessCiphertext, accessNonce, err := storage.Encrypt(a.encKey, []byte(session.AccessToken))
	if err != nil {
		return fmt.Errorf("auth: encrypt access token: %w", err)
	}
	refreshCiphertext, refreshNonce, err := storage.Encrypt(a.encKey, []byte(session.RefreshToken))
	if err != nil {
		return fmt.Errorf("auth: encrypt refresh token: %w", err)
	}

	return a.db.UpsertAccount(storage.Account{
		GoogleUserID:           session.GoogleUserID,
		Email:                  session.Email,
		AccessTokenCiphertext:  accessCiphertext,
		AccessTokenNonce:       accessNonce,
		RefreshTokenCiphertext: refreshCiphertext,
		RefreshTokenNonce:      refreshNonce,
		AccessTokenExpiry:      session.Expiry,
		CreatedAt:              time.Now(),
	})
}

// silentlyRefresh refreshes an about-to-expire access token in place,
// re-encrypting and persisting the result.
func (a *App) silentlyRefresh(acct *storage.Account) error {
	if a.encKey == nil {
		return keychain.ErrUnavailable
	}
	refreshPlain, err := storage.Decrypt(a.encKey, acct.RefreshTokenCiphertext, acct.RefreshTokenNonce)
	if err != nil {
		return fmt.Errorf("auth: decrypt refresh token: %w", err)
	}

	tok, err := auth.RefreshAccessToken(a.ctx, a.oauthConfig(), string(refreshPlain))
	if err != nil {
		return err
	}

	newRefreshToken := tok.RefreshToken
	if newRefreshToken == "" {
		// Google does not always rotate the refresh token; keep the
		// existing one if none was issued.
		newRefreshToken = string(refreshPlain)
	}

	session := &auth.Session{
		Email:        acct.Email,
		GoogleUserID: acct.GoogleUserID,
		AccessToken:  tok.AccessToken,
		RefreshToken: newRefreshToken,
		Expiry:       tok.Expiry,
	}
	return a.persistSession(session)
}

// --- Files.* (contracts/wails-bindings.md) --------------------------------

// LocalFileRef mirrors contracts/wails-bindings.md's LocalFileRef type.
type LocalFileRef struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

// FilesPickLocal opens the native OS file-picker dialog in single-select
// mode (contract: Files.PickLocal; FR-004's "exactly one file"). Returns
// nil if the user cancels the dialog.
func (a *App) FilesPickLocal() (*LocalFileRef, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a file to upload",
	})
	if err != nil {
		return nil, fmt.Errorf("files: open dialog: %w", err)
	}
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("files: could not read the selected file: %w", err)
	}
	return &LocalFileRef{Path: path, Name: filepath.Base(path), SizeBytes: info.Size()}, nil
}

// --- Drive.* (contracts/wails-bindings.md) ---------------------------------

// DriveListFolders lists child folders of parentId ("" means the Drive
// root, "My Drive" -- Acceptance Scenario 2 of User Story 2). Requires a
// signed-in session (FR-001).
func (a *App) DriveListFolders(parentId string) ([]drive.Folder, error) {
	svc, err := a.driveService(a.ctx)
	if err != nil {
		return nil, err
	}
	return drive.ListFolders(a.ctx, svc, parentId)
}

// driveService builds an authenticated Drive API client for the current
// session, refreshing the access token first if it's near expiry. Returns
// errSignedOut if there is no valid session -- the single FR-001 gate all
// Drive/Upload methods share.
func (a *App) driveService(ctx context.Context) (*drivev3.Service, error) {
	if err := a.requireSignedIn(); err != nil {
		return nil, err
	}
	acct, err := a.db.GetAccount()
	if err != nil {
		return nil, errSignedOut
	}

	if auth.NeedsRefresh(acct.AccessTokenExpiry) {
		if err := a.silentlyRefresh(acct); err != nil {
			logging.Warn("silent token refresh failed before Drive call; clearing local session", "error", err)
			_ = a.db.DeleteAccount()
			events.EmitAuthChanged(a.ctx, events.AuthStatus{SignedIn: false})
			return nil, errSignedOut
		}
		acct, err = a.db.GetAccount()
		if err != nil {
			return nil, errSignedOut
		}
	}

	accessPlain, err := storage.Decrypt(a.encKey, acct.AccessTokenCiphertext, acct.AccessTokenNonce)
	if err != nil {
		return nil, fmt.Errorf("drive: decrypt access token: %w", err)
	}
	tok := &oauth2pkg.Token{AccessToken: string(accessPlain), Expiry: acct.AccessTokenExpiry}
	client := a.oauthConfig().Client(ctx, tok)

	opts := []option.ClientOption{option.WithHTTPClient(client)}
	if a.driveAPIEndpointOverride != "" {
		opts = append(opts, option.WithEndpoint(a.driveAPIEndpointOverride))
	}
	svc, err := drivev3.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("drive: build client: %w", err)
	}
	return svc, nil
}

// --- Upload.* (contracts/wails-bindings.md) ---------------------------------

// UploadStatusDTO mirrors contracts/wails-bindings.md's UploadStatus type.
type UploadStatusDTO struct {
	Status        string `json:"status"`
	BytesSent     int64  `json:"bytesSent"`
	TotalBytes    int64  `json:"totalBytes"`
	DriveFileLink string `json:"driveFileLink,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
}

// UploadStart validates the local file still exists (FR-011), creates an
// Upload row, and begins the non-resumable transfer in the background.
// Requires a signed-in session; rejects with errSignedOut (a distinct auth
// error, not a generic upload failure) if called while signed out
// (FR-001) -- checked, like the file-existence check, before any Upload
// row is created.
func (a *App) UploadStart(localPath, driveFolderId string) (int64, error) {
	if err := a.requireSignedIn(); err != nil {
		return 0, err
	}
	if a.db == nil {
		return 0, fmt.Errorf("upload: local database is unavailable")
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return 0, fmt.Errorf("upload: local file can no longer be found: %w", err)
	}

	svc, err := a.driveService(a.ctx)
	if err != nil {
		return 0, err
	}

	u, err := a.db.CreateUpload(localPath, info.Size(), driveFolderId)
	if err != nil {
		return 0, fmt.Errorf("upload: create upload record: %w", err)
	}
	if err := a.db.SetUploadInProgress(u.ID); err != nil {
		return 0, fmt.Errorf("upload: %w", err)
	}

	go a.runUpload(u.ID, svc, localPath, driveFolderId, info.Size())

	return u.ID, nil
}

// runUpload performs the actual transfer and records its outcome. Runs on
// the app's lifetime context (not a per-request context, since it
// continues after UploadStart has already returned the UploadId to the
// frontend). drive.UploadFile emits the upload:progress/upload:complete/
// upload:failed events itself (T035); this method's job is solely to keep
// the persisted Upload row's bytes_sent/status in sync with those outcomes.
func (a *App) runUpload(id int64, svc *drivev3.Service, localPath, driveFolderID string, totalBytes int64) {
	onProgress := func(bytesSent int64) {
		if err := a.db.UpdateUploadProgress(id, bytesSent); err != nil {
			logging.Warn("failed to record upload progress", "uploadId", id, "error", err)
		}
	}
	result, err := drive.UploadFile(a.ctx, svc, id, localPath, driveFolderID, totalBytes, onProgress)
	if err != nil {
		if setErr := a.db.SetUploadFailed(id, err.Error()); setErr != nil {
			logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
		}
		return
	}
	if setErr := a.db.SetUploadSucceeded(id, result.FileID, result.WebViewLink); setErr != nil {
		logging.Warn("failed to record upload success", "uploadId", id, "error", setErr)
	}
}

// UploadGetStatus is a point-in-time read of an Upload's state, for
// reconnecting the UI after a reload (contract: Upload.GetStatus).
func (a *App) UploadGetStatus(id int64) (UploadStatusDTO, error) {
	if a.db == nil {
		return UploadStatusDTO{}, fmt.Errorf("upload: local database is unavailable")
	}
	u, err := a.db.GetUpload(id)
	if err != nil {
		return UploadStatusDTO{}, err
	}
	dto := UploadStatusDTO{
		Status:     string(u.Status),
		BytesSent:  u.BytesSent,
		TotalBytes: u.LocalSizeBytes,
	}
	if u.DriveFileLink != nil {
		dto.DriveFileLink = *u.DriveFileLink
	}
	if u.FailureReason != nil {
		dto.FailureReason = *u.FailureReason
	}
	return dto, nil
}
