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

// App is the struct Wails binds to the frontend. Every exported method is
// part of the Go<->TS contract, prefixed by namespace (AuthGetStatus,
// DriveListFolders, ...) since Go doesn't allow dotted method names.
type App struct {
	ctx context.Context
	db  *storage.DB
	// encKey encrypts token columns at rest and is loaded from the OS
	// keychain at startup. It stays in memory only, never on disk.
	encKey []byte

	// userInfoURL and revokeEndpoint point at Google by default; the
	// E2E-mocked dev build (mock_e2e.go) swaps in an in-process fake server.
	userInfoURL           string
	revokeEndpoint        string
	openBrowser           auth.BrowserOpener
	oauthEndpointOverride *oauth2pkg.Endpoint
	// driveAPIEndpointOverride points at the same mock server; empty in production.
	driveAPIEndpointOverride string
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup wires up runtime dependencies once Wails hands us a context. If
// the OS keychain is unavailable, the app still starts but fails closed on
// anything needing the encryption key, rather than using an unencrypted fallback.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.userInfoURL = auth.DefaultUserInfoURL
	a.revokeEndpoint = auth.GoogleRevokeEndpoint
	a.openBrowser = browser.OpenURL
	maybeInstallE2EMock(a)

	db, err := storage.Open()
	if err != nil {
		logging.Error("failed to open local database", "error", err)
	} else {
		a.db = db
	}

	key, err := keychain.GetOrCreateKey()
	if err != nil {
		// Some Linux setups have no keyring daemon running; warn instead of erroring.
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
// Client ID/secret come from the environment (BALLAST_GOOGLE_CLIENT_ID/SECRET) rather than being hardcoded.
func (a *App) oauthConfig() *oauth2pkg.Config {
	endpoint := google.Endpoint
	if a.oauthEndpointOverride != nil {
		endpoint = *a.oauthEndpointOverride
	}
	return &oauth2pkg.Config{
		ClientID:     os.Getenv("BALLAST_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("BALLAST_GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{auth.OpenIDScope, auth.UserInfoEmailScope, auth.DriveFileScope, auth.DriveMetadataReadonlyScope},
		Endpoint:     endpoint,
	}
}

// errSignedOut is returned by Files/Drive/Upload methods when called without an active session.
var errSignedOut = errors.New("not signed in")

// --- Auth.* ----------------------------------------------------------------

// AuthGetStatus returns the current session state, silently refreshing a
// near-expiry access token first. If that refresh fails, the local session is cleared.
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

// AuthSignIn starts the OAuth loopback flow.
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
		// A denied/cancelled consent isn't an error — just leave no session behind.
		return events.AuthStatus{SignedIn: false}, nil
	}

	if err := a.persistSession(session); err != nil {
		return events.AuthStatus{}, err
	}

	status := events.AuthStatus{SignedIn: true, Email: session.Email}
	events.EmitAuthChanged(a.ctx, status)
	return status, nil
}

// AuthSignOut revokes the OAuth grant server-side and clears the local session.
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

// requireSignedIn is the single access-control gate shared by every
// Files/Drive/Upload method.
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
		// Google doesn't always rotate the refresh token; keep the
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

// --- Files.* -----------------------------------------------------------

// LocalFileRef is the file metadata sent to the frontend after a pick.
type LocalFileRef struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

// FilesPickLocal opens the native OS file picker in single-select mode.
// Returns nil if the user cancels.
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

// --- Drive.* -----------------------------------------------------------

// DriveListFolders lists the child folders of parentId ("" means the
// Drive root, "My Drive"). Requires an active session.
func (a *App) DriveListFolders(parentId string) ([]drive.Folder, error) {
	svc, err := a.driveService(a.ctx)
	if err != nil {
		return nil, err
	}
	return drive.ListFolders(a.ctx, svc, parentId)
}

// driveService builds an authenticated Drive API client for the current
// session, refreshing the access token first if it's near expiry.
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
	refreshPlain, err := storage.Decrypt(a.encKey, acct.RefreshTokenCiphertext, acct.RefreshTokenNonce)
	if err != nil {
		return nil, fmt.Errorf("drive: decrypt refresh token: %w", err)
	}
	tok := &oauth2pkg.Token{
		AccessToken:  string(accessPlain),
		RefreshToken: string(refreshPlain),
		Expiry:       acct.AccessTokenExpiry,
	}
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

// --- Upload.* -----------------------------------------------------------

// UploadStatusDTO is the upload status sent to the frontend.
type UploadStatusDTO struct {
	Status        string `json:"status"`
	BytesSent     int64  `json:"bytesSent"`
	TotalBytes    int64  `json:"totalBytes"`
	DriveFileLink string `json:"driveFileLink,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
}

// UploadStart verifies the local file still exists, creates an Upload row,
// and kicks off the transfer in the background.
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

// runUpload performs the transfer and records its outcome. It runs on the
// app's lifetime context since it keeps going after UploadStart has already returned.
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

// UploadGetStatus is a point-in-time read of an upload's state, used to
// reconnect the UI after a reload.
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
