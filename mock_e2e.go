package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"

	oauth2pkg "golang.org/x/oauth2"
)

// This file implements Ballast's E2E-mock mode: when the BALLAST_E2E_MOCK
// environment variable is set, Google's real OAuth/userinfo/revoke
// endpoints are replaced with an in-process fake server, and the
// system-browser launch that would normally pop up a real browser window
// is replaced with an automatic, immediate "redirect" -- so the full
// Auth.SignIn/Auth.SignOut contract can be exercised end-to-end by
// Playwright (research.md §5's "mock the Google OAuth consent screen ...
// at the network boundary") without a real Google account, a real browser
// window, or any network access.
//
// This exists purely to make quickstart.md Scenario 1 automatable in CI;
// it has zero effect unless BALLAST_E2E_MOCK is set, so it never changes
// production sign-in behavior.
const (
	e2eMockEnvVar        = "BALLAST_E2E_MOCK"
	e2eOutcomeFileEnvVar = "BALLAST_E2E_OUTCOME_FILE"
)

// maybeInstallE2EMock wires a.oauthEndpointOverride/a.userInfoURL/
// a.revokeEndpoint/a.openBrowser to an in-process mock server if
// BALLAST_E2E_MOCK is set; otherwise it's a no-op and the real Google
// endpoints (set just before this call in startup) are left untouched.
func maybeInstallE2EMock(a *App) {
	if os.Getenv(e2eMockEnvVar) == "" {
		return
	}

	mock := newE2EMockServer()
	a.oauthEndpointOverride = &oauth2pkg.Endpoint{
		AuthURL:  mock.URL + "/o/oauth2/v2/auth",
		TokenURL: mock.URL + "/token",
	}
	a.userInfoURL = mock.URL + "/userinfo"
	a.revokeEndpoint = mock.URL + "/revoke"
	a.openBrowser = e2eBrowserOpener
}

// newE2EMockServer starts an in-process HTTP server standing in for
// Google's token exchange, userinfo, and revoke endpoints. It always
// succeeds with fixed test data -- the interesting behavior under test
// (cancel/deny, persistence, sign-out) is driven by e2eBrowserOpener and by
// the frontend/backend contract itself, not by varying the mock's
// responses.
func newE2EMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "e2e-mock-access-token",
			"refresh_token": "e2e-mock-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":   "e2e-mock-user-id",
			"email": "e2e-mock-user@example.com",
		})
	})

	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

// e2eBrowserOpener replaces launching a real system browser: it parses the
// authorization URL's redirect_uri/state (exactly as a real browser
// following Google's redirect would) and immediately performs that
// redirect itself, either with a fake authorization code (approve) or
// error=access_denied (deny), so no visible browser window is ever needed.
//
// The outcome is controlled by the file at BALLAST_E2E_OUTCOME_FILE:
// content "deny" simulates the user cancelling consent; anything else
// (including a missing/empty file) simulates approval. Playwright toggles
// this between test cases without restarting the wails dev process.
func e2eBrowserOpener(authURL string) error {
	u, err := url.Parse(authURL)
	if err != nil {
		return fmt.Errorf("mock_e2e: parse auth URL: %w", err)
	}
	redirectURI := u.Query().Get("redirect_uri")
	state := u.Query().Get("state")

	cb, err := url.Parse(redirectURI)
	if err != nil {
		return fmt.Errorf("mock_e2e: parse redirect_uri: %w", err)
	}
	q := cb.Query()
	q.Set("state", state)
	if readE2EOutcome() == "deny" {
		q.Set("error", "access_denied")
	} else {
		q.Set("code", "e2e-mock-auth-code")
	}
	cb.RawQuery = q.Encode()

	resp, err := http.Get(cb.String())
	if err != nil {
		return fmt.Errorf("mock_e2e: deliver mock callback: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func readE2EOutcome() string {
	path := os.Getenv(e2eOutcomeFileEnvVar)
	if path == "" {
		return "approve"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "approve"
	}
	return strings.TrimSpace(string(data))
}
