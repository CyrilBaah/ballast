package main

import (
	"context"
	"encoding/json"
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
	// dbPath is remembered so DebugRestart (E2E-test-only) can reopen the
	// same SQLite file, simulating a process restart without actually
	// killing the OS process.
	dbPath string
	// uploadCancel stops the currently-running upload goroutine's
	// resumable-session loop, if any. DebugRestart (E2E-test-only) calls
	// it to simulate the current process's in-flight work dying, rather
	// than leaving it running against a freshly-reopened DB handle.
	uploadCancel context.CancelFunc
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

	if path, err := storage.DefaultPath(); err != nil {
		logging.Error("failed to resolve local database path", "error", err)
	} else {
		a.dbPath = path
	}
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
		Scopes:       []string{auth.OpenIDScope, auth.UserInfoEmailScope, auth.UserInfoProfileScope, auth.DriveFileScope, auth.DriveMetadataReadonlyScope},
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

	status := events.AuthStatus{SignedIn: true, Email: acct.Email}
	if acct.DisplayName != nil {
		status.Name = *acct.DisplayName
	}
	if acct.PictureURL != nil {
		status.PictureURL = *acct.PictureURL
	}
	return status
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

	status := events.AuthStatus{SignedIn: true, Email: session.Email, Name: session.Name, PictureURL: session.Picture}
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

	acct := storage.Account{
		GoogleUserID:           session.GoogleUserID,
		Email:                  session.Email,
		AccessTokenCiphertext:  accessCiphertext,
		AccessTokenNonce:       accessNonce,
		RefreshTokenCiphertext: refreshCiphertext,
		RefreshTokenNonce:      refreshNonce,
		AccessTokenExpiry:      session.Expiry,
		CreatedAt:              time.Now(),
	}
	if session.Name != "" {
		acct.DisplayName = &session.Name
	}
	if session.Picture != "" {
		acct.PictureURL = &session.Picture
	}
	return a.db.UpsertAccount(acct)
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
	// A silent refresh doesn't re-fetch userinfo -- carry the
	// already-stored name/picture forward so persistSession doesn't wipe
	// them (it only writes a field when the Session value is non-empty).
	if acct.DisplayName != nil {
		session.Name = *acct.DisplayName
	}
	if acct.PictureURL != nil {
		session.Picture = *acct.PictureURL
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

// DriveGetStorageQuota returns the signed-in account's Drive storage usage
// (contracts/wails-bindings.md), fetched fresh via Drive's about.get and
// held in memory for the session only -- never persisted (research.md §8).
func (a *App) DriveGetStorageQuota() (drive.StorageQuota, error) {
	svc, err := a.driveService(a.ctx)
	if err != nil {
		return drive.StorageQuota{}, err
	}
	return drive.GetStorageQuota(a.ctx, svc)
}

// driveService builds an authenticated Drive API client for the current
// session, refreshing the access token first if it's near expiry.
func (a *App) driveService(ctx context.Context) (*drivev3.Service, error) {
	client, err := a.driveHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
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

// driveHTTPClient builds an authenticated HTTP client for the current
// session's Drive access, refreshing the access token first if it's near
// expiry. Shared by driveService (JSON API calls via the SDK) and the raw
// resumable-upload primitives in internal/drive, which talk to Drive
// directly over net/http (research.md §1).
func (a *App) driveHTTPClient(ctx context.Context) (*http.Client, error) {
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
	return a.oauthConfig().Client(ctx, tok), nil
}

// driveUploadAPIBase returns the host used for the raw resumable-upload
// HTTP calls -- production Drive, or the E2E mock server if overridden.
func (a *App) driveUploadAPIBase() string {
	if a.driveAPIEndpointOverride != "" {
		return a.driveAPIEndpointOverride
	}
	return drive.ProdAPIBase
}

// --- Upload.* -----------------------------------------------------------

// UploadStatusDTO is the upload status sent to the frontend.
type UploadStatusDTO struct {
	Status                     string `json:"status"`
	BytesSent                  int64  `json:"bytesSent"`
	TotalBytes                 int64  `json:"totalBytes"`
	DriveFileLink              string `json:"driveFileLink,omitempty"`
	FailureReason              string `json:"failureReason,omitempty"`
	AwaitingConfirmationReason string `json:"awaitingConfirmationReason,omitempty"`
}

// RecoverableUploadDTO describes the single non-terminal upload left over
// from a previous run, if any (contracts/wails-bindings.md).
type RecoverableUploadDTO struct {
	ID                         int64  `json:"id"`
	LocalPath                  string `json:"localPath"`
	FileName                   string `json:"fileName"`
	Status                     string `json:"status"`
	BytesSent                  int64  `json:"bytesSent"`
	TotalBytes                 int64  `json:"totalBytes"`
	AwaitingConfirmationReason string `json:"awaitingConfirmationReason,omitempty"`
}

// UploadListItemDTO is one row of Upload.ListRecent's result
// (contracts/wails-bindings.md), the new upload-history screen's data source.
type UploadListItemDTO struct {
	ID              int64  `json:"id"`
	FileName        string `json:"fileName"`
	DriveFolderName string `json:"driveFolderName"`
	Status          string `json:"status"`
	BytesSent       int64  `json:"bytesSent"`
	TotalBytes      int64  `json:"totalBytes"`
	DriveFileLink   string `json:"driveFileLink,omitempty"`
	FailureReason   string `json:"failureReason,omitempty"`
	StartedAt       string `json:"startedAt"`
}

// UploadListRecent returns up to 50 uploads, most recent first, for the
// history screen's initial snapshot (data-model.md's ListRecentUploads,
// contracts/wails-bindings.md). Read-only; the frontend keeps entries
// current afterward by subscribing to the same upload:* events
// progress.ts already consumes, not by re-calling this method.
func (a *App) UploadListRecent() ([]UploadListItemDTO, error) {
	if a.db == nil {
		return nil, fmt.Errorf("upload: local database is unavailable")
	}
	uploads, err := a.db.ListRecentUploads()
	if err != nil {
		return nil, err
	}
	items := make([]UploadListItemDTO, 0, len(uploads))
	for _, u := range uploads {
		folderName := "My Drive"
		if u.DriveFolderName != nil && *u.DriveFolderName != "" {
			folderName = *u.DriveFolderName
		}
		item := UploadListItemDTO{
			ID:              u.ID,
			FileName:        filepath.Base(u.LocalPath),
			DriveFolderName: folderName,
			Status:          string(u.Status),
			BytesSent:       u.BytesSent,
			TotalBytes:      u.LocalSizeBytes,
			StartedAt:       u.StartedAt.UTC().Format(time.RFC3339),
		}
		if u.DriveFileLink != nil {
			item.DriveFileLink = *u.DriveFileLink
		}
		if u.FailureReason != nil {
			item.FailureReason = *u.FailureReason
		}
		items = append(items, item)
	}
	return items, nil
}

// UploadStart verifies the local file still exists, creates an Upload row,
// and kicks off the transfer in the background. Rejects up front (FR-013)
// if another upload is already in_progress, paused, or awaiting_confirmation.
func (a *App) UploadStart(localPath, driveFolderId, driveFolderName string) (int64, error) {
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

	client, err := a.driveHTTPClient(a.ctx)
	if err != nil {
		return 0, err
	}

	u, err := a.db.CreateUpload(localPath, info.Size(), info.ModTime(), driveFolderId, driveFolderName)
	if err != nil {
		return 0, fmt.Errorf("upload: create upload record: %w", err)
	}
	if err := a.db.SetUploadInProgress(u.ID); err != nil {
		return 0, fmt.Errorf("upload: %w", err)
	}

	baseline := drive.IdentityBaseline{Size: info.Size(), Mtime: info.ModTime()}
	a.startUpload(u.ID, client, localPath, driveFolderId, info.Size(), baseline, drive.ResumeState{})

	return u.ID, nil
}

// startUpload launches runUpload in the background under a context
// derived from the app's lifetime context, remembering its cancel func so
// DebugRestart (E2E-test-only) can stop it -- simulating the current
// process's in-flight work dying on a "restart" without needing to
// actually kill the OS process.
func (a *App) startUpload(id int64, client *http.Client, localPath, driveFolderID string, totalBytes int64, baseline drive.IdentityBaseline, resume drive.ResumeState) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.uploadCancel = cancel
	go a.runUpload(ctx, id, client, localPath, driveFolderID, totalBytes, baseline, resume)
}

// runUpload drives one upload's resumable session to a terminal outcome
// and records it. ctx governs only the resumable transfer itself (so it
// can be cancelled independently, e.g. by DebugRestart); events are still
// emitted against the app's lifetime context. It keeps going after the
// call that launched it has already returned; resume is the checkpoint to
// continue from (zero value for a brand-new upload).
func (a *App) runUpload(ctx context.Context, id int64, client *http.Client, localPath, driveFolderID string, totalBytes int64, baseline drive.IdentityBaseline, resume drive.ResumeState) {
	cb := drive.UploadCallbacks{
		OnChunkAcked: func(bytesSent int64, sessionURI string, hashState []byte, chunkSize int64, consecutiveSuccesses int) {
			if err := a.db.UpdateUploadProgress(id, bytesSent, sessionURI, hashState, chunkSize, consecutiveSuccesses); err != nil {
				logging.Warn("failed to record upload progress", "uploadId", id, "error", err)
			}
			events.EmitUploadProgress(a.ctx, id, bytesSent, totalBytes)
		},
		OnPaused: func() {
			if err := a.db.SetUploadPaused(id); err != nil {
				logging.Warn("failed to record upload paused", "uploadId", id, "error", err)
			}
			events.EmitUploadPaused(a.ctx, id, time.Now())
		},
		OnResumed: func() {
			if err := a.db.SetUploadResumed(id); err != nil {
				logging.Warn("failed to record upload resumed", "uploadId", id, "error", err)
			}
		},
	}

	result, err := drive.UploadFile(a.ctx, client, a.driveUploadAPIBase(), id, localPath, driveFolderID, totalBytes, baseline, resume, cb)
	if err != nil {
		var outcome *drive.TerminalOutcome
		if errors.As(err, &outcome) {
			if outcome.Bucket == drive.TerminalRecoverable {
				if setErr := a.db.SetUploadAwaitingConfirmation(id, outcome.Reason); setErr != nil {
					logging.Warn("failed to record upload awaiting_confirmation", "uploadId", id, "error", setErr)
				}
				events.EmitUploadAwaitingConfirmation(a.ctx, id, outcome.Reason)
			} else {
				if setErr := a.db.SetUploadFailed(id, outcome.Reason); setErr != nil {
					logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
				}
				events.EmitUploadFailed(a.ctx, id, outcome.Reason)
			}
		}
		// A non-TerminalOutcome error means ctx was cancelled (the app is
		// shutting down): nothing to record -- the last checkpoint already
		// on disk is what a future launch resumes from.
		return
	}
	if setErr := a.db.SetUploadSucceeded(id, result.FileID, result.WebViewLink); setErr != nil {
		logging.Warn("failed to record upload success", "uploadId", id, "error", setErr)
	}
	events.EmitUploadComplete(a.ctx, id, result.WebViewLink)
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
	if u.AwaitingConfirmationReason != nil {
		dto.AwaitingConfirmationReason = *u.AwaitingConfirmationReason
	}
	return dto, nil
}

// UploadGetRecoverable returns the single non-terminal upload left over
// from a previous run, if any (research.md §7). If it's still paused, its
// source-file-identity check (research.md §5) runs here: passing it means
// the backend has already begun resuming it in the background by the
// time this call returns; failing it routes the upload to
// awaiting_confirmation instead, with no auto-resume.
func (a *App) UploadGetRecoverable() (*RecoverableUploadDTO, error) {
	if err := a.requireSignedIn(); err != nil {
		return nil, err
	}
	u, err := a.db.GetRecoverableUpload()
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	if u.Status == storage.UploadPaused {
		client, cerr := a.driveHTTPClient(a.ctx)
		if cerr != nil {
			return nil, cerr
		}
		baseline := drive.IdentityBaseline{Size: u.LocalSizeBytes, Mtime: u.LocalMtime}

		identityOK, statErr := drive.CheapIdentityCheck(u.LocalPath, baseline)
		if statErr != nil {
			reason := fmt.Sprintf("local file can no longer be found: %v", statErr)
			if setErr := a.db.SetUploadFailed(u.ID, reason); setErr != nil {
				logging.Warn("failed to record upload failure", "uploadId", u.ID, "error", setErr)
			}
			events.EmitUploadFailed(a.ctx, u.ID, reason)
			return nil, nil
		}
		if !identityOK {
			verified, verifyErr := drive.VerifyPrefix(u.LocalPath, u.BytesSent, u.ContentHashState)
			if verifyErr != nil {
				reason := fmt.Sprintf("local file can no longer be found: %v", verifyErr)
				if setErr := a.db.SetUploadFailed(u.ID, reason); setErr != nil {
					logging.Warn("failed to record upload failure", "uploadId", u.ID, "error", setErr)
				}
				events.EmitUploadFailed(a.ctx, u.ID, reason)
				return nil, nil
			}
			if !verified {
				if setErr := a.db.SetUploadAwaitingConfirmation(u.ID, storage.AwaitingConfirmationFileChanged); setErr != nil {
					logging.Warn("failed to record upload awaiting_confirmation", "uploadId", u.ID, "error", setErr)
				}
				events.EmitUploadAwaitingConfirmation(a.ctx, u.ID, storage.AwaitingConfirmationFileChanged)
				u.Status = storage.UploadAwaitingConfirmation
				reason := storage.AwaitingConfirmationFileChanged
				u.AwaitingConfirmationReason = &reason
			}
		}

		if u.Status == storage.UploadPaused {
			sessionURI := ""
			if u.SessionURI != nil {
				sessionURI = *u.SessionURI
			}
			resume := drive.ResumeState{SessionURI: sessionURI, BytesSent: u.BytesSent, ContentHashState: u.ContentHashState, ChunkSize: u.ChunkSizeBytes, ConsecutiveSuccesses: u.ConsecutiveChunkSuccesses}
			a.startUpload(u.ID, client, u.LocalPath, u.DriveFolderID, u.LocalSizeBytes, baseline, resume)
		}
	}

	dto := &RecoverableUploadDTO{
		ID:         u.ID,
		LocalPath:  u.LocalPath,
		FileName:   filepath.Base(u.LocalPath),
		Status:     string(u.Status),
		BytesSent:  u.BytesSent,
		TotalBytes: u.LocalSizeBytes,
	}
	if u.AwaitingConfirmationReason != nil {
		dto.AwaitingConfirmationReason = *u.AwaitingConfirmationReason
	}
	return dto, nil
}

// UploadConfirmRestart restarts an awaiting_confirmation upload from byte
// 0 against a brand-new Drive session (FR-010). Only valid when the
// upload's status is currently awaiting_confirmation. The restarted
// transfer's first chunk uses the size the upload had earned before the
// interruption, not the baseline -- ResetUploadForRestart deliberately
// leaves chunk_size_bytes/consecutive_chunk_successes untouched, and this
// applies the same way regardless of which awaiting_confirmation_reason
// triggered the restart (spec Clarifications, research.md §5).
func (a *App) UploadConfirmRestart(id int64) error {
	if err := a.requireSignedIn(); err != nil {
		return err
	}
	u, err := a.db.GetUpload(id)
	if err != nil {
		return err
	}
	if u.Status != storage.UploadAwaitingConfirmation {
		return fmt.Errorf("upload: cannot restart an upload that is not awaiting confirmation")
	}

	info, err := os.Stat(u.LocalPath)
	if err != nil {
		reason := fmt.Sprintf("local file can no longer be found: %v", err)
		if setErr := a.db.SetUploadFailed(id, reason); setErr != nil {
			logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
		}
		events.EmitUploadFailed(a.ctx, id, reason)
		return fmt.Errorf("upload: %s", reason)
	}

	client, err := a.driveHTTPClient(a.ctx)
	if err != nil {
		return err
	}
	// Release the old session before abandoning it locally -- otherwise it
	// can still complete independently on Drive's side later, leaving an
	// orphaned duplicate file alongside the one the restarted upload creates.
	if u.SessionURI != nil {
		drive.ReleaseSession(a.ctx, client, *u.SessionURI)
	}

	if err := a.db.ResetUploadForRestart(id, info.Size(), info.ModTime()); err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	baseline := drive.IdentityBaseline{Size: info.Size(), Mtime: info.ModTime()}
	resume := drive.ResumeState{ChunkSize: u.ChunkSizeBytes, ConsecutiveSuccesses: u.ConsecutiveChunkSuccesses}
	a.startUpload(id, client, u.LocalPath, u.DriveFolderID, info.Size(), baseline, resume)
	return nil
}

// UploadCancel transitions a paused or awaiting_confirmation upload
// directly to cancelled (FR-014), freeing the single-active-upload slot.
// Makes a best-effort attempt to release the Drive session first,
// ignoring its result -- local cancellation must succeed regardless.
func (a *App) UploadCancel(id int64) error {
	if a.db == nil {
		return fmt.Errorf("upload: local database is unavailable")
	}
	u, err := a.db.GetUpload(id)
	if err != nil {
		return err
	}
	if u.SessionURI != nil {
		if client, cerr := a.driveHTTPClient(a.ctx); cerr == nil {
			drive.ReleaseSession(a.ctx, client, *u.SessionURI)
		}
	}
	return a.db.SetUploadCancelled(id)
}

// --- Debug.* (E2E-test-only) --------------------------------------------

// DebugRestart discards this App's in-memory state (DB connection, auth
// token cache) and reopens a fresh storage.DB against the same on-disk
// SQLite file, then re-runs the same keychain/E2E-mock wiring startup
// does. It exists solely so Playwright can exercise User Story 2's
// crash-recovery path (quickstart.md Scenario 2) -- SQLite's own
// durability, not any OS crash-reporting mechanism, is what this feature
// actually relies on (Constitution Principle VII), so reopening a fresh
// handle against the same file is a faithful simulation of a process
// restart without needing to kill and relaunch the OS process itself.
// Only available in E2E mock mode; rejects otherwise.
func (a *App) DebugRestart() error {
	if a.driveAPIEndpointOverride == "" {
		return fmt.Errorf("debug: DebugRestart is only available in E2E mock mode")
	}
	if a.uploadCancel != nil {
		a.uploadCancel()
		a.uploadCancel = nil
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			logging.Warn("error closing database during DebugRestart", "error", err)
		}
	}
	a.db = nil
	a.encKey = nil

	db, err := storage.OpenAt(a.dbPath)
	if err != nil {
		return fmt.Errorf("debug: reopen database: %w", err)
	}
	a.db = db

	key, err := keychain.GetOrCreateKey()
	if err != nil {
		logging.Warn("OS keychain unavailable after DebugRestart", "error", err)
		return nil
	}
	a.encKey = key
	return nil
}

// DebugSessionReleaseCount reports how many DELETEs the E2E mock Drive
// server has received against a resumable-upload session. It exists so
// Playwright can assert that UploadConfirmRestart (and UploadCancel)
// actually release a stale session with Drive rather than only
// discarding it locally -- silently dropping that call lets the old
// session keep completing in the background and produces a duplicate
// file. Only available in E2E mock mode; rejects otherwise.
func (a *App) DebugSessionReleaseCount() (int, error) {
	if a.driveAPIEndpointOverride == "" {
		return 0, fmt.Errorf("debug: DebugSessionReleaseCount is only available in E2E mock mode")
	}
	resp, err := http.Get(a.driveAPIEndpointOverride + "/debug/session-release-count")
	if err != nil {
		return 0, fmt.Errorf("debug: query session release count: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("debug: decode session release count: %w", err)
	}
	return body.Count, nil
}
