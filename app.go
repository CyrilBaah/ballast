package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ballast/internal/auth"
	"ballast/internal/captions"
	"ballast/internal/drive"
	"ballast/internal/events"
	"ballast/internal/keychain"
	"ballast/internal/logging"
	"ballast/internal/storage"
	"ballast/internal/summaries"

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
	// running holds the live transfer goroutine of every upload that has
	// one, keyed by upload ID, so an upload is only ever driven by one
	// goroutine at a time. Two goroutines on the same row each finish
	// their own Drive session and leave duplicate files behind. Guarded
	// by runningMu: it is touched from Wails-bound calls and from upload
	// goroutines alike.
	runningMu sync.Mutex
	running   map[int64]*uploadRun

	// captions turns uploaded videos into caption files (Feature 005);
	// captionsStop stops its worker loop.
	captions     *captions.Worker
	captionsStop context.CancelFunc
	// captionsAvailability reports whether captions can run on this
	// machine; swapped in tests.
	captionsAvailability func() (ok bool, reason, binPath string)

	// summaries writes a summary of every captioned video (Feature 006).
	summaries             *summaries.Worker
	summariesStop         context.CancelFunc
	summariesAvailability func() (ok bool, reason, binPath string)
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

	// autoRestarted remembers which uploads have already silently
	// restarted themselves after Drive dropped their resumable session,
	// so a session that keeps dying can't put the app in a loop that
	// re-sends the same file forever. One free restart per upload per app
	// run; the second dropped session is reported as a failure instead.
	// Guarded by autoRestartMu: it is written from the upload goroutine
	// (runUpload) and read from Wails RPC calls (UploadGetRecoverable).
	autoRestartMu sync.Mutex
	autoRestarted map[int64]bool
}

// claimAutoRestart reserves this upload's one automatic restart, reporting
// false if it has already been used in this app run.
func (a *App) claimAutoRestart(id int64) bool {
	a.autoRestartMu.Lock()
	defer a.autoRestartMu.Unlock()
	if a.autoRestarted[id] {
		return false
	}
	a.autoRestarted[id] = true
	return true
}

// resetAutoRestarts returns every upload's automatic restart, for a
// simulated process restart (DebugRestart) that a real relaunch would.
func (a *App) resetAutoRestarts() {
	a.autoRestartMu.Lock()
	defer a.autoRestartMu.Unlock()
	a.autoRestarted = make(map[int64]bool)
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{
		autoRestarted:         make(map[int64]bool),
		running:               make(map[int64]*uploadRun),
		captionsAvailability:  captions.Availability,
		summariesAvailability: summaries.Availability,
	}
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

	if dir, err := storage.AppDataDir(); err != nil {
		logging.Error("failed to resolve app data directory", "error", err)
	} else if err := logging.LogToFile(dir); err != nil {
		logging.Warn("could not open log file; logging to stderr only", "error", err)
	}

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
	a.startCaptions()
	a.startSummaries()

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
	if a.captionsStop != nil {
		a.captionsStop()
	}
	if a.summariesStop != nil {
		a.summariesStop()
	}
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

// errSessionUnusable marks a silent-refresh failure caused by the stored
// tokens themselves (they can't be decrypted), as opposed to the network
// or Google's token endpoint.
var errSessionUnusable = errors.New("stored session can't be used")

// refreshIsPermanent reports whether a failed silent refresh means the
// stored Google session can never work again -- the grant was revoked or
// expired, or the tokens can't be decrypted -- so the user has to sign in
// again. Anything else (no internet, Google's token endpoint briefly
// failing or rate-limiting) is temporary: signing the user out for it
// would throw away a perfectly good session. The upload engine draws the
// same line (drive.ClassifyTransportError).
func refreshIsPermanent(err error) bool {
	if errors.Is(err, errSessionUnusable) {
		return true
	}
	var retrieve *oauth2pkg.RetrieveError
	if errors.As(err, &retrieve) {
		return drive.ClassifyTransportError(err).Bucket == drive.TerminalNeedsSignIn
	}
	return false
}

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
			if !refreshIsPermanent(err) {
				// Keep the session: the next Drive call refreshes again.
				logging.Warn("silent token refresh failed for now; keeping the session", "error", err)
			} else {
				logging.Warn("silent token refresh failed; clearing local session", "error", err)
				_ = a.db.DeleteAccount()
				status := events.AuthStatus{SignedIn: false}
				events.EmitAuthChanged(a.ctx, status)
				return status
			}
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
		return fmt.Errorf("auth: decrypt refresh token: %w: %w", errSessionUnusable, err)
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

// FilesPickLocalMultiple opens the native OS file picker in multi-select
// mode. Returns an empty slice if the user cancels.
func (a *App) FilesPickLocalMultiple() ([]LocalFileRef, error) {
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select files to upload",
	})
	if err != nil {
		return nil, fmt.Errorf("files: open dialog: %w", err)
	}
	refs := make([]LocalFileRef, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("files: could not read the selected file: %w", err)
		}
		refs = append(refs, LocalFileRef{Path: path, Name: filepath.Base(path), SizeBytes: info.Size()})
	}
	return refs, nil
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
			if refreshIsPermanent(err) {
				logging.Warn("silent token refresh failed before Drive call; clearing local session", "error", err)
				_ = a.db.DeleteAccount()
				events.EmitAuthChanged(a.ctx, events.AuthStatus{SignedIn: false})
				return nil, errSignedOut
			}
			// Temporary: hand back a client with the stored tokens. Its
			// token source refreshes again on first use, and the upload
			// engine already retries a failed refresh as a network blip.
			logging.Warn("silent token refresh failed for now; continuing with the stored session", "error", err)
		} else if acct, err = a.db.GetAccount(); err != nil {
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
	// Caption is the upload's captioning state, absent when it has none
	// (Feature 005 contracts/wails-bindings.md).
	Caption *events.CaptionJob `json:"caption,omitempty"`
	// Summary is the upload's summary state, absent when it has none
	// (Feature 006 contracts/wails-bindings.md).
	Summary *events.SummaryJob `json:"summary,omitempty"`
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
		item.Caption = a.captionJobFor(u.ID)
		item.Summary = a.summaryJobFor(u.ID)
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
	return a.startNewUpload(localPath, driveFolderId, driveFolderName)
}

// UploadRetry starts a brand-new upload of a cancelled upload's same local
// file and destination, from byte 0 -- a distinct History row, not a
// resume of the old one's session (Drive already closed that when it was
// cancelled, and ResetUploadForRestart's byte-0 restart is reserved for
// awaiting_confirmation rows -- FR-010). Rejects anything not cancelled.
func (a *App) UploadRetry(id int64) (int64, error) {
	if err := a.requireSignedIn(); err != nil {
		return 0, err
	}
	if a.db == nil {
		return 0, fmt.Errorf("upload: local database is unavailable")
	}
	u, err := a.db.GetUpload(id)
	if err != nil {
		return 0, err
	}
	if u.Status != storage.UploadCancelled {
		return 0, fmt.Errorf("upload: cannot retry an upload that is not cancelled")
	}
	folderName := ""
	if u.DriveFolderName != nil {
		folderName = *u.DriveFolderName
	}
	return a.startNewUpload(u.LocalPath, u.DriveFolderID, folderName)
}

// startNewUpload is UploadStart/UploadRetry's shared core: create a fresh
// Upload row for localPath/driveFolderId and claim FR-013's single-active
// slot for it, then launch the transfer in the background.
func (a *App) startNewUpload(localPath, driveFolderId, driveFolderName string) (int64, error) {
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
		// The row above already exists as "pending" -- pending is not a
		// recoverable/non-terminal status (storage.nonTerminalStatuses), so
		// leaving it as-is would strand it in History forever with no way
		// to retry or dismiss it. Mark it failed so it's a clear, terminal
		// row instead of a silent ghost.
		if failErr := a.db.SetUploadFailed(u.ID, err.Error()); failErr != nil {
			logging.Warn("failed to mark stranded upload as failed", "uploadId", u.ID, "error", failErr)
		}
		return 0, fmt.Errorf("upload: %w", err)
	}

	baseline := drive.IdentityBaseline{Size: info.Size(), Mtime: info.ModTime()}
	a.startUpload(u.ID, client, localPath, driveFolderId, info.Size(), baseline, drive.ResumeState{})
	a.enqueueCaptions(u.ID)

	return u.ID, nil
}

// uploadRun is one live transfer goroutine for an upload.
type uploadRun struct {
	cancel context.CancelFunc
}

// startUpload launches runUpload in the background under a context
// derived from the app's lifetime context, unless the upload already has
// a live transfer goroutine -- in which case it does nothing and returns
// false. The cancel func is remembered so UploadCancel and DebugRestart
// (E2E-test-only) can stop it.
func (a *App) startUpload(id int64, client *http.Client, localPath, driveFolderID string, totalBytes int64, baseline drive.IdentityBaseline, resume drive.ResumeState) bool {
	return a.launchUpload(id, nil, client, localPath, driveFolderID, totalBytes, baseline, resume)
}

// launchUpload is startUpload, except that the upload's live run may be
// handed over from prev -- a goroutine that is restarting its own upload
// on its way out (restartExpiredSession) -- rather than refused.
func (a *App) launchUpload(id int64, prev *uploadRun, client *http.Client, localPath, driveFolderID string, totalBytes int64, baseline drive.IdentityBaseline, resume drive.ResumeState) bool {
	ctx, cancel := context.WithCancel(a.ctx)
	run := &uploadRun{cancel: cancel}

	a.runningMu.Lock()
	if cur := a.running[id]; cur != nil && cur != prev {
		a.runningMu.Unlock()
		cancel()
		logging.Warn("upload already has a live transfer; not starting a second one", "uploadId", id)
		return false
	}
	a.running[id] = run
	a.runningMu.Unlock()

	go a.runUpload(ctx, run, id, client, localPath, driveFolderID, totalBytes, baseline, resume)
	return true
}

// finishRun drops run from the live set once its goroutine is done,
// unless it has already handed the upload over to a newer run.
func (a *App) finishRun(id int64, run *uploadRun) {
	run.cancel()
	a.runningMu.Lock()
	defer a.runningMu.Unlock()
	if a.running[id] == run {
		delete(a.running, id)
	}
}

// uploadIsRunning reports whether some goroutine other than self is
// currently driving upload id (self may be nil).
func (a *App) uploadIsRunning(id int64, self *uploadRun) bool {
	a.runningMu.Lock()
	defer a.runningMu.Unlock()
	cur := a.running[id]
	return cur != nil && cur != self
}

// stopUpload cancels upload id's live transfer goroutine, if it has one.
func (a *App) stopUpload(id int64) {
	a.runningMu.Lock()
	run := a.running[id]
	delete(a.running, id)
	a.runningMu.Unlock()
	if run != nil {
		run.cancel()
	}
}

// stopAllUploads cancels every live transfer goroutine.
func (a *App) stopAllUploads() {
	a.runningMu.Lock()
	runs := a.running
	a.running = make(map[int64]*uploadRun)
	a.runningMu.Unlock()
	for _, run := range runs {
		run.cancel()
	}
}

// runUpload drives one upload's resumable session to a terminal outcome
// and records it. ctx governs only the resumable transfer itself (so it
// can be cancelled independently, e.g. by DebugRestart); events are still
// emitted against the app's lifetime context. It keeps going after the
// call that launched it has already returned; resume is the checkpoint to
// continue from (zero value for a brand-new upload).
func (a *App) runUpload(ctx context.Context, run *uploadRun, id int64, client *http.Client, localPath, driveFolderID string, totalBytes int64, baseline drive.IdentityBaseline, resume drive.ResumeState) {
	defer a.finishRun(id, run)

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
			switch outcome.Bucket {
			case drive.TerminalNeedsSignIn:
				a.holdForSignIn(id, outcome.Reason)
			case drive.TerminalRecoverable:
				if outcome.Reason == drive.ReasonSessionExpired && a.restartExpiredSession(id, run) {
					return
				}
				if setErr := a.db.SetUploadAwaitingConfirmation(id, outcome.Reason); setErr != nil {
					logging.Warn("failed to record upload awaiting_confirmation", "uploadId", id, "error", setErr)
				}
				events.EmitUploadAwaitingConfirmation(a.ctx, id, outcome.Reason)
			default:
				if setErr := a.db.SetUploadFailed(id, outcome.Reason); setErr != nil {
					logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
				}
				events.EmitUploadFailed(a.ctx, id, outcome.Reason)
				a.cancelCaptions(id)
			}
		}
		// A non-TerminalOutcome error means ctx was cancelled (the app is
		// shutting down): nothing to record -- the last checkpoint already
		// on disk is what a future launch resumes from.
		return
	}
	if setErr := a.db.SetUploadSucceeded(id, result.FileID, result.WebViewLink); setErr != nil {
		// The transfer genuinely finished on Drive's side, but couldn't be
		// persisted locally as such (e.g. the webViewLink fallback lookup
		// itself failed) -- don't tell the UI it's complete when the DB
		// disagrees. The row stays non-terminal, same as the ctx-cancelled
		// case above: a future launch's GetRecoverable picks it back up.
		logging.Warn("failed to record upload success", "uploadId", id, "error", setErr)
		return
	}
	events.EmitUploadComplete(a.ctx, id, result.WebViewLink)
	a.captionsVideoSucceeded(id)
}

// stillTheSameFile reports whether u's source file on disk is still the
// file whose bytes Drive already acknowledged: a cheap size/mtime
// comparison first (research.md §5), falling back to a bounded re-hash of
// exactly the acknowledged prefix when that fails, since a rewritten-then-
// restored mtime is not proof of a changed file. A non-nil error means the
// file is gone or unreadable, which no restart can fix.
func (a *App) stillTheSameFile(u *storage.Upload) (bool, error) {
	baseline := drive.IdentityBaseline{Size: u.LocalSizeBytes, Mtime: u.LocalMtime}
	ok, err := drive.CheapIdentityCheck(u.LocalPath, baseline)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	return drive.VerifyPrefix(u.LocalPath, u.BytesSent, u.ContentHashState)
}

// restartExpiredSession handles a Drive resumable session that Drive itself
// has dropped (404/410 on the session URI). Once that happens the bytes
// Drive had acknowledged are gone from its side for good -- a status query
// against the dead URI returns 404, not an offset -- so there is nothing
// left to continue from and a new session starting at byte 0 is the only
// way this file ever lands. Since the outcome is forced, the app just does
// it rather than stopping to ask: the user gets a transfer that carries on
// by itself, not a dialog whose only real answer is "yes".
//
// It returns true once it has taken responsibility for the upload (either
// restarted it or recorded a terminal outcome). Returning false leaves the
// upload for the caller's ordinary awaiting_confirmation path -- used when
// the source file has changed underneath it, which is a genuine question
// only the user can answer, and when the session is gone but the app is
// signed out, where the next sign-in retries this same path.
//
// The row is moved to awaiting_confirmation before the reset, both to
// reuse ResetUploadForRestart's guard and so that a crash mid-restart
// leaves a state the next launch recognises and picks up here again.
//
// self is the transfer goroutine calling this on its way out, or nil when
// called from outside one; any other live goroutine on this upload already
// owns it, so this leaves it alone. Before restarting from zero it asks
// Drive whether the "dropped" session in fact finished, and if so records
// that file instead of uploading a second copy.
func (a *App) restartExpiredSession(id int64, self *uploadRun) bool {
	if a.uploadIsRunning(id, self) {
		return true
	}
	u, err := a.db.GetUpload(id)
	if err != nil {
		logging.Warn("could not read upload while restarting an expired session", "uploadId", id, "error", err)
		return false
	}

	same, err := a.stillTheSameFile(u)
	if err != nil {
		reason := fmt.Sprintf("local file can no longer be found: %v", err)
		if setErr := a.db.SetUploadFailed(id, reason); setErr != nil {
			logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
		}
		events.EmitUploadFailed(a.ctx, id, reason)
		return true
	}
	if !same {
		// The file changed underneath the transfer as well. Which file to
		// send is a question only the user can answer, so this one does
		// stop and ask -- under file_changed, the reason that actually
		// needs them, rather than the expired session that doesn't.
		if setErr := a.db.SetUploadAwaitingConfirmation(id, storage.AwaitingConfirmationFileChanged); setErr != nil {
			logging.Warn("failed to record upload awaiting_confirmation", "uploadId", id, "error", setErr)
		}
		events.EmitUploadAwaitingConfirmation(a.ctx, id, storage.AwaitingConfirmationFileChanged)
		return true
	}

	client, err := a.driveHTTPClient(a.ctx)
	if err != nil {
		// Signed out: leave the row where the caller puts it, so the next
		// sign-in's recovery pass comes back through here with a client.
		return false
	}
	info, err := os.Stat(u.LocalPath)
	if err != nil {
		reason := fmt.Sprintf("local file can no longer be found: %v", err)
		if setErr := a.db.SetUploadFailed(id, reason); setErr != nil {
			logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
		}
		events.EmitUploadFailed(a.ctx, id, reason)
		return true
	}
	if a.adoptLandedUpload(client, u) {
		return true
	}
	// Claimed only once everything else is ready, so a restart that never
	// happened doesn't spend the allowance for one that could.
	if !a.claimAutoRestart(id) {
		reason := "Google Drive dropped this upload's session twice, so its progress could not be kept"
		if setErr := a.db.SetUploadFailed(id, reason); setErr != nil {
			logging.Warn("failed to record upload failure", "uploadId", id, "error", setErr)
		}
		events.EmitUploadFailed(a.ctx, id, reason)
		return true
	}

	if u.Status != storage.UploadAwaitingConfirmation {
		if setErr := a.db.SetUploadAwaitingConfirmation(id, storage.AwaitingConfirmationSessionExpired); setErr != nil {
			logging.Warn("failed to record expired session before restarting it", "uploadId", id, "error", setErr)
			return false
		}
	}
	if err := a.db.ResetUploadForRestart(id, info.Size(), info.ModTime()); err != nil {
		logging.Warn("failed to reset upload after its session expired", "uploadId", id, "error", err)
		return false
	}
	logging.Info("drive dropped this upload's session; starting a fresh one automatically", "uploadId", id, "discardedBytes", u.BytesSent)

	// Tell the UI the progress bar is going back to zero before any chunk
	// lands, so it shows a transfer that restarted rather than one frozen
	// at the offset Drive just threw away.
	events.EmitUploadProgress(a.ctx, id, 0, info.Size())

	baseline := drive.IdentityBaseline{Size: info.Size(), Mtime: info.ModTime()}
	resume := drive.ResumeState{ChunkSize: u.ChunkSizeBytes, ConsecutiveSuccesses: u.ConsecutiveChunkSuccesses}
	a.launchUpload(id, self, client, u.LocalPath, u.DriveFolderID, info.Size(), baseline, resume)
	return true
}

// adoptLandedUpload checks whether upload u's file already finished
// landing in Drive under a session Ballast lost track of, and if so
// records it as the upload's result and reports true, so the caller does
// not restart it from zero. A failed lookup reports false: the restart
// goes ahead, as it would have without the check.
func (a *App) adoptLandedUpload(client *http.Client, u *storage.Upload) bool {
	res, err := drive.FindLandedUpload(a.ctx, client, a.driveUploadAPIBase(), u.ID, u.LocalPath, u.DriveFolderID, u.LocalSizeBytes)
	if err != nil {
		logging.Warn("could not check Drive for an already-finished copy before restarting", "uploadId", u.ID, "error", err)
		return false
	}
	if res == nil {
		return false
	}
	if err := a.db.SetUploadSucceeded(u.ID, res.FileID, res.WebViewLink); err != nil {
		logging.Warn("failed to record an upload Drive had already finished", "uploadId", u.ID, "error", err)
		return false
	}
	logging.Info("upload had already finished on Drive; keeping that file instead of re-sending", "uploadId", u.ID, "driveFileId", res.FileID)
	events.EmitUploadComplete(a.ctx, u.ID, res.WebViewLink)
	a.captionsVideoSucceeded(u.ID)
	return true
}

// holdForSignIn parks an upload whose Google session died mid-transfer.
// The row stays paused -- non-terminal, with its session URI, bytes_sent
// and content-hash checkpoint intact -- rather than failed, because
// nothing about the transfer itself went wrong: every acknowledged byte is
// still sitting on Drive's side of the resumable session. The transfer
// goroutine still has to stop, since the client it was started with wraps
// a token source holding the now-dead refresh token and would never see a
// newly minted one.
//
// Clearing the local session mirrors what a failed pre-flight refresh
// already does (driveHTTPClient), and is what closes the loop: the UI
// drops to signed-out, and the user's next sign-in runs the ordinary
// recovery path (UploadGetRecoverable), which picks this row up and
// resumes it from its last acknowledged byte -- never from zero.
func (a *App) holdForSignIn(id int64, reason string) {
	logging.Warn("google session died mid-transfer; parking upload until the next sign-in", "uploadId", id, "reason", reason)
	if err := a.db.SetUploadPaused(id); err != nil {
		logging.Warn("failed to park upload for sign-in", "uploadId", id, "error", err)
	}
	events.EmitUploadPaused(a.ctx, id, time.Now())

	if err := a.db.DeleteAccount(); err != nil {
		logging.Warn("failed to clear local session after in-flight refresh failure", "uploadId", id, "error", err)
	}
	events.EmitAuthChanged(a.ctx, events.AuthStatus{SignedIn: false})
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
	// A row this process is already transferring is not a leftover: it can
	// read as paused while its goroutine waits out a network drop, and
	// starting it again here would race a second session against the first.
	live := a.uploadIsRunning(u.ID, nil)

	// An upload left waiting on a session Drive has already dropped has
	// nothing to wait for -- there is no offset to resume against and only
	// one way for the file to land. Restart it here rather than greeting
	// the user with a question whose only answer is yes.
	if !live &&
		u.Status == storage.UploadAwaitingConfirmation &&
		u.AwaitingConfirmationReason != nil &&
		*u.AwaitingConfirmationReason == storage.AwaitingConfirmationSessionExpired &&
		a.restartExpiredSession(u.ID, nil) {
		refreshed, rerr := a.db.GetUpload(u.ID)
		if rerr != nil {
			return nil, rerr
		}
		u = refreshed
	}

	if !live && u.Status == storage.UploadPaused {
		client, cerr := a.driveHTTPClient(a.ctx)
		if cerr != nil {
			return nil, cerr
		}
		baseline := drive.IdentityBaseline{Size: u.LocalSizeBytes, Mtime: u.LocalMtime}

		same, idErr := a.stillTheSameFile(u)
		if idErr != nil {
			reason := fmt.Sprintf("local file can no longer be found: %v", idErr)
			if setErr := a.db.SetUploadFailed(u.ID, reason); setErr != nil {
				logging.Warn("failed to record upload failure", "uploadId", u.ID, "error", setErr)
			}
			events.EmitUploadFailed(a.ctx, u.ID, reason)
			return nil, nil
		}
		if !same {
			if setErr := a.db.SetUploadAwaitingConfirmation(u.ID, storage.AwaitingConfirmationFileChanged); setErr != nil {
				logging.Warn("failed to record upload awaiting_confirmation", "uploadId", u.ID, "error", setErr)
			}
			events.EmitUploadAwaitingConfirmation(a.ctx, u.ID, storage.AwaitingConfirmationFileChanged)
			u.Status = storage.UploadAwaitingConfirmation
			reason := storage.AwaitingConfirmationFileChanged
			u.AwaitingConfirmationReason = &reason
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
	if a.uploadIsRunning(id, nil) {
		return fmt.Errorf("upload: this upload is already transferring")
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
	// A file that changed may still match the old copy's name and size, so
	// only an expired session is checked for having quietly finished.
	if u.AwaitingConfirmationReason != nil &&
		*u.AwaitingConfirmationReason == storage.AwaitingConfirmationSessionExpired &&
		a.adoptLandedUpload(client, u) {
		return nil
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
	// Checked before touching the session: releasing the session of an
	// upload that cannot be cancelled would leave its transfer reading the
	// released session as expired and restarting from zero.
	if u.Status != storage.UploadPaused && u.Status != storage.UploadAwaitingConfirmation {
		return storage.ErrUploadNotCancellable
	}
	// Stop the transfer goroutine before releasing its session, for the
	// same reason.
	a.stopUpload(id)
	if u.SessionURI != nil {
		if client, cerr := a.driveHTTPClient(a.ctx); cerr == nil {
			drive.ReleaseSession(a.ctx, client, *u.SessionURI)
		}
	}
	if err := a.db.SetUploadCancelled(id); err != nil {
		return err
	}
	a.cancelCaptions(id)
	return nil
}

// UploadDelete permanently removes a terminal (succeeded, failed, or
// cancelled) upload row from history (storage.DeleteUpload rejects
// anything still active).
func (a *App) UploadDelete(id int64) error {
	if a.db == nil {
		return fmt.Errorf("upload: local database is unavailable")
	}
	a.cancelCaptions(id)
	return a.db.DeleteUpload(id)
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
	a.stopAllUploads()
	if a.captionsStop != nil {
		a.captionsStop()
		a.captionsStop = nil
	}
	if a.summariesStop != nil {
		a.summariesStop()
		a.summariesStop = nil
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			logging.Warn("error closing database during DebugRestart", "error", err)
		}
	}
	a.db = nil
	a.encKey = nil
	// A simulated process restart starts over on the per-run auto-restart
	// allowance too, the same way a real relaunch would.
	a.resetAutoRestarts()

	db, err := storage.OpenAt(a.dbPath)
	if err != nil {
		return fmt.Errorf("debug: reopen database: %w", err)
	}
	a.db = db
	a.startCaptions()
	a.startSummaries()

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
