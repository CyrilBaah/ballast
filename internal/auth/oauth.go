// Package auth implements the Google OAuth 2.0 desktop (installed-app)
// loopback-redirect flow with PKCE (research.md §1): a local HTTP listener
// on an OS-assigned port captures the redirect after the user approves
// access in their system browser, closing the main weakness of loopback
// redirects (a second local process racing to claim the redirect) by making
// a stolen auth code useless without the original code verifier.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Least-privilege scope pair (research.md §1): drive.file for upload
// capability, drive.metadata.readonly to browse pre-existing folders
// (FR-005) without requesting full Drive read/write access.
const (
	DriveFileScope             = "https://www.googleapis.com/auth/drive.file"
	DriveMetadataReadonlyScope = "https://www.googleapis.com/auth/drive.metadata.readonly"
)

// FetchUserInfo needs an identity scope to call Google's userinfo endpoint
// at all -- the Drive scopes above don't grant access to it on their own.
const (
	OpenIDScope        = "openid"
	UserInfoEmailScope = "https://www.googleapis.com/auth/userinfo.email"
)

// DefaultUserInfoURL is Google's real userinfo endpoint. Callers that need
// to point at a mock (tests; the E2E-mocked dev build, research.md §5) pass
// a different URL explicitly to FetchUserInfo/SignIn instead of relying on
// package-level mutable state.
const DefaultUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"

// PKCE holds an S256 PKCE verifier/challenge pair (RFC 7636).
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a new random code verifier (43-128 chars per RFC 7636,
// here 96 bytes of randomness base64url-encoded) and its S256 challenge.
func NewPKCE() (PKCE, error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return PKCE{}, fmt.Errorf("auth: generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)

	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	return PKCE{Verifier: verifier, Challenge: challenge}, nil
}

// BuildAuthURL builds the Google consent-screen URL for this sign-in
// attempt, including the PKCE challenge and the loopback redirect URI.
func BuildAuthURL(cfg *oauth2.Config, state string, pkce PKCE, redirectURL string) string {
	c := *cfg
	c.RedirectURL = redirectURL
	return c.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", pkce.Challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
}

// ExchangeCode exchanges an authorization code for tokens, presenting the
// PKCE verifier so Google can validate it against the challenge sent in
// BuildAuthURL.
func ExchangeCode(ctx context.Context, cfg *oauth2.Config, code string, pkce PKCE, redirectURL string) (*oauth2.Token, error) {
	c := *cfg
	c.RedirectURL = redirectURL
	tok, err := c.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pkce.Verifier))
	if err != nil {
		return nil, fmt.Errorf("auth: exchange code: %w", err)
	}
	return tok, nil
}

// UserInfo is the subset of Google's userinfo response this feature needs.
type UserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

// FetchUserInfo retrieves the signed-in user's stable account identifier
// and email using an authenticated client (an *http.Client produced by
// cfg.Client(ctx, tok), so the access token is attached automatically and
// never appears in this function's own code or logs).
func FetchUserInfo(ctx context.Context, client *http.Client, userInfoURL string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: build userinfo request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: userinfo returned status %d", resp.StatusCode)
	}
	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("auth: decode userinfo: %w", err)
	}
	return &info, nil
}

// CallbackResult is what the loopback listener captured from Google's
// redirect.
type CallbackResult struct {
	Code   string
	State  string
	Denied bool // true when Google redirected with error=access_denied (or any error param) -- Edge Case: user cancels/denies mid-flow
	Err    error
}

// LoopbackServer is a short-lived local HTTP listener that captures exactly
// one OAuth redirect, per research.md §1.
type LoopbackServer struct {
	listener net.Listener
	server   *http.Server
	resultCh chan CallbackResult
}

// StartLoopbackServer starts listening on an OS-assigned loopback port.
func StartLoopbackServer() (*LoopbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("auth: start loopback listener: %w", err)
	}

	s := &LoopbackServer{
		listener: ln,
		resultCh: make(chan CallbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)
	s.server = &http.Server{Handler: mux}

	go func() {
		_ = s.server.Serve(ln)
	}()

	return s, nil
}

func (s *LoopbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := CallbackResult{
		Code:  q.Get("code"),
		State: q.Get("state"),
	}
	if errParam := q.Get("error"); errParam != "" {
		result.Denied = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if result.Denied {
		_, _ = w.Write([]byte("<html><body>Sign-in was cancelled. You can close this window and return to Ballast.</body></html>"))
	} else {
		_, _ = w.Write([]byte("<html><body>Signed in. You can close this window and return to Ballast.</body></html>"))
	}

	select {
	case s.resultCh <- result:
	default:
		// Already delivered a result (shouldn't happen for a single
		// sign-in attempt); drop any further hits to /callback.
	}
}

// RedirectURL returns the loopback callback URL to register as this
// attempt's redirect_uri.
func (s *LoopbackServer) RedirectURL() string {
	return fmt.Sprintf("http://%s/callback", s.listener.Addr().String())
}

// Wait blocks until the redirect is captured or ctx is cancelled.
func (s *LoopbackServer) Wait(ctx context.Context) (CallbackResult, error) {
	select {
	case result := <-s.resultCh:
		return result, nil
	case <-ctx.Done():
		return CallbackResult{}, ctx.Err()
	}
}

// Close shuts down the loopback listener. Safe to call once the callback
// has been received (or the attempt is abandoned).
func (s *LoopbackServer) Close() error {
	return s.server.Close()
}

// randomState generates an opaque, unguessable OAuth state parameter (CSRF
// protection for the redirect).
func randomState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Session is the outcome of a completed sign-in: either a real session, or
// Cancelled=true if the user denied/cancelled consent (Edge Case in
// spec.md) -- a valid, expected outcome, not an error.
type Session struct {
	Cancelled    bool
	Email        string
	GoogleUserID string
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

// BrowserOpener opens a URL in the user's default system browser. Swappable
// for tests / the mocked-OAuth Playwright path (research.md §5) so SignIn
// doesn't require launching a real browser window.
type BrowserOpener func(url string) error

// signInTimeout bounds how long SignIn waits for the user to complete (or
// cancel) the consent flow in their browser before giving up.
const signInTimeout = 5 * time.Minute

// SignIn runs one full loopback+PKCE sign-in attempt: starts the local
// listener, opens the consent screen via openBrowser, waits for the
// redirect, exchanges the code, and fetches the account's email/sub.
// userInfoURL is normally DefaultUserInfoURL; tests / the E2E-mocked dev
// build pass a mock server URL instead.
func SignIn(ctx context.Context, cfg *oauth2.Config, openBrowser BrowserOpener, userInfoURL string) (*Session, error) {
	pkce, err := NewPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	loopback, err := StartLoopbackServer()
	if err != nil {
		return nil, err
	}
	defer loopback.Close()

	authURL := BuildAuthURL(cfg, state, pkce, loopback.RedirectURL())
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("auth: open system browser: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, signInTimeout)
	defer cancel()

	result, err := loopback.Wait(waitCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("auth: timed out waiting for Google sign-in to complete")
		}
		return nil, err
	}
	if result.Denied {
		return &Session{Cancelled: true}, nil
	}
	if result.State != state {
		return nil, fmt.Errorf("auth: redirect state mismatch (possible CSRF)")
	}

	tok, err := ExchangeCode(ctx, cfg, result.Code, pkce, loopback.RedirectURL())
	if err != nil {
		return nil, err
	}

	client := cfg.Client(ctx, tok)
	info, err := FetchUserInfo(ctx, client, userInfoURL)
	if err != nil {
		return nil, err
	}

	return &Session{
		Email:        info.Email,
		GoogleUserID: info.Sub,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
	}, nil
}
