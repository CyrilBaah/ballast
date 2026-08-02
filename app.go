package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"ballast/internal/auth"
	"ballast/internal/events"
	"ballast/internal/keychain"
	"ballast/internal/logging"
	"ballast/internal/storage"

	"github.com/pkg/browser"
	oauth2pkg "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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
