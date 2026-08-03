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

// This file implements Ballast's E2E-mock mode: when BALLAST_E2E_MOCK is
// set, Google's OAuth/userinfo/revoke endpoints and the browser launch are
// replaced with fakes so Playwright can run the full sign-in flow with no
// real Google account or network access. It has no effect unless the env var is set.
const (
	e2eMockEnvVar        = "BALLAST_E2E_MOCK"
	e2eOutcomeFileEnvVar = "BALLAST_E2E_OUTCOME_FILE"
)

// maybeInstallE2EMock points the app's OAuth/userinfo/revoke/Drive
// endpoints at an in-process mock server if BALLAST_E2E_MOCK is set.
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
	a.driveAPIEndpointOverride = mock.URL
}

// newE2EMockServer starts an in-process HTTP server standing in for
// Google's token exchange, userinfo, and revoke endpoints, always succeeding with fixed test data.
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

	// Files.List always reports no existing folders, so tests always land on "My Drive".
	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]string{}})
	})

	// outcome "network-fail" simulates a dropped connection mid-upload instead of a clean HTTP error.
	mux.HandleFunc("/upload/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		if readE2EOutcome() == "network-fail" {
			hj, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "e2e-mock-file-id",
			"webViewLink": "https://drive.google.com/file/d/e2e-mock-file-id/view",
		})
	})

	return httptest.NewServer(mux)
}

// e2eBrowserOpener replaces launching a real system browser: it parses the
// authorization URL's redirect_uri/state and immediately redirects itself,
// either with a fake auth code (approve) or error=access_denied (deny). The
// outcome is read from the file at BALLAST_E2E_OUTCOME_FILE ("deny" or anything else for approve).
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
