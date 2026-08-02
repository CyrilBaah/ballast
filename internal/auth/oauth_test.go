package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewPKCEProducesValidVerifierAndChallenge(t *testing.T) {
	pkce, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if len(pkce.Verifier) < 43 || len(pkce.Verifier) > 128 {
		t.Fatalf("verifier length %d out of RFC 7636 range [43,128]", len(pkce.Verifier))
	}
	if pkce.Challenge == "" {
		t.Fatal("challenge must not be empty")
	}
	if strings.ContainsAny(pkce.Challenge, "+/=") {
		t.Fatalf("challenge must be base64url without padding, got %q", pkce.Challenge)
	}

	pkce2, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE #2: %v", err)
	}
	if pkce.Verifier == pkce2.Verifier {
		t.Fatal("two independent NewPKCE calls produced the same verifier")
	}
}

func testOAuthConfig(tokenURL, authURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Scopes:       []string{DriveFileScope, DriveMetadataReadonlyScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}
}

func TestBuildAuthURLIncludesPKCEAndScopes(t *testing.T) {
	cfg := testOAuthConfig("https://example.invalid/token", "https://example.invalid/auth")
	pkce, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}

	authURL := BuildAuthURL(cfg, "test-state", pkce, "http://127.0.0.1:12345/callback")

	for _, want := range []string{
		"client_id=test-client-id",
		"code_challenge=" + pkce.Challenge,
		"code_challenge_method=S256",
		"state=test-state",
		"redirect_uri=",
	} {
		if !strings.Contains(authURL, want) {
			t.Errorf("auth URL %q missing expected fragment %q", authURL, want)
		}
	}
	if !strings.Contains(authURL, "drive.file") {
		t.Errorf("auth URL %q missing drive.file scope", authURL)
	}
	if !strings.Contains(authURL, "drive.metadata.readonly") {
		t.Errorf("auth URL %q missing drive.metadata.readonly scope", authURL)
	}
}

func TestExchangeCodeSendsVerifierAndParsesToken(t *testing.T) {
	var gotVerifier string
	var gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotVerifier = r.Form.Get("code_verifier")
		gotCode = r.Form.Get("code")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	cfg := testOAuthConfig(srv.URL, "https://example.invalid/auth")
	pkce, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}

	tok, err := ExchangeCode(context.Background(), cfg, "test-auth-code", pkce, "http://127.0.0.1:12345/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if gotCode != "test-auth-code" {
		t.Errorf("server saw code %q, want %q", gotCode, "test-auth-code")
	}
	if gotVerifier != pkce.Verifier {
		t.Errorf("server saw code_verifier %q, want %q", gotVerifier, pkce.Verifier)
	}
	if tok.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "test-access-token")
	}
	if tok.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", tok.RefreshToken, "test-refresh-token")
	}
}

func TestLoopbackServerCapturesCallbackCode(t *testing.T) {
	srv, err := StartLoopbackServer()
	if err != nil {
		t.Fatalf("StartLoopbackServer: %v", err)
	}
	defer srv.Close()

	if !strings.HasPrefix(srv.RedirectURL(), "http://127.0.0.1:") {
		t.Fatalf("RedirectURL %q does not look like a loopback URL", srv.RedirectURL())
	}

	resultCh := make(chan CallbackResult, 1)
	go func() {
		result, err := srv.Wait(context.Background())
		resultCh <- CallbackResult{Code: result.Code, State: result.State, Err: err}
	}()

	resp, err := http.Get(srv.RedirectURL() + "?code=abc123&state=xyz")
	if err != nil {
		t.Fatalf("http.Get callback: %v", err)
	}
	resp.Body.Close()

	select {
	case result := <-resultCh:
		if result.Err != nil {
			t.Fatalf("Wait returned error: %v", result.Err)
		}
		if result.Code != "abc123" {
			t.Errorf("Code = %q, want %q", result.Code, "abc123")
		}
		if result.State != "xyz" {
			t.Errorf("State = %q, want %q", result.State, "xyz")
		}
	case <-timeoutCh():
		t.Fatal("timed out waiting for loopback callback result")
	}
}

func TestLoopbackServerReportsUserDenied(t *testing.T) {
	srv, err := StartLoopbackServer()
	if err != nil {
		t.Fatalf("StartLoopbackServer: %v", err)
	}
	defer srv.Close()

	resultCh := make(chan CallbackResult, 1)
	go func() {
		result, err := srv.Wait(context.Background())
		resultCh <- CallbackResult{Code: result.Code, State: result.State, Denied: result.Denied, Err: err}
	}()

	resp, err := http.Get(srv.RedirectURL() + "?error=access_denied&state=xyz")
	if err != nil {
		t.Fatalf("http.Get callback: %v", err)
	}
	resp.Body.Close()

	select {
	case result := <-resultCh:
		if result.Err != nil {
			t.Fatalf("Wait returned error for a denial (should be a non-error Denied result): %v", result.Err)
		}
		if !result.Denied {
			t.Error("expected Denied=true when Google redirects with error=access_denied")
		}
	case <-timeoutCh():
		t.Fatal("timed out waiting for loopback callback result")
	}
}
