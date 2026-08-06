package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	oauth2pkg "golang.org/x/oauth2"
)

// This file implements Ballast's E2E-mock mode: when BALLAST_E2E_MOCK is
// set, Google's OAuth/userinfo/revoke/Drive endpoints and the browser
// launch are replaced with fakes so Playwright can run the full sign-in +
// resumable-upload flow with no real Google account or network access. It
// has no effect unless the env var is set.
const (
	e2eMockEnvVar        = "BALLAST_E2E_MOCK"
	e2eOutcomeFileEnvVar = "BALLAST_E2E_OUTCOME_FILE"
)

// resumeIncomplete is Drive's resumable-upload status code for "chunk
// accepted, more bytes expected" (matches internal/drive's constant of
// the same numeric value -- duplicated here since this file builds only
// under `go run`/`wails dev`, not as part of the internal/drive package).
const resumeIncomplete = 308

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

// e2eUploadSession tracks the single in-flight resumable-upload session
// the mock server supports at a time (matching the app's own
// one-upload-at-a-time constraint, FR-013).
type e2eUploadSession struct {
	total    int64
	received int64
}

// newE2EMockServer starts an in-process HTTP server standing in for
// Google's token exchange, userinfo, revoke, folder-listing, and
// resumable-upload endpoints. Most outcomes always succeed with fixed
// test data; the resumable-upload endpoints additionally honor
// BALLAST_E2E_OUTCOME_FILE to simulate the failure modes research.md §4
// and §6 describe (dropped connections, retryable errors, terminal
// conditions), so Playwright can exercise the resumable engine's
// pause/resume/terminal-condition handling without a real Drive account.
func newE2EMockServer() *httptest.Server {
	mux := http.NewServeMux()

	var mu sync.Mutex
	var session *e2eUploadSession
	// sessionReleaseCount counts DELETEs against the resumable-session URL
	// (UploadCancel and UploadConfirmRestart's best-effort release of a
	// stale session), exposed to Playwright via /debug/session-release-count
	// so a regression that drops that release call is actually caught,
	// rather than being invisible because this mock always returns 204
	// regardless of whether DELETE was ever sent.
	var sessionReleaseCount int
	// srv is assigned once httptest.NewServer starts the listener below;
	// handlers registered before that point (e.g. /userinfo, for the
	// avatar URL it returns) only read it once a real request arrives,
	// by which point srv is always already set.
	var srv *httptest.Server

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" && readE2EOutcome() == "token-refresh-fail" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "invalid_grant",
				"error_description": "token has been revoked",
			})
			return
		}
		// "signin-fail" simulates the code exchange itself failing (as
		// opposed to "deny", which the user triggers by cancelling
		// consent before any exchange happens) -- needed so Playwright
		// can exercise a genuine sign-in error's state treatment (User
		// Story 2) distinct from the no-error cancellation path.
		if r.Form.Get("grant_type") != "refresh_token" && readE2EOutcome() == "signin-fail" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "invalid_grant",
				"error_description": "malformed authorization code",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "e2e-mock-access-token",
			"refresh_token": "e2e-mock-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	// userinfo's name/picture fields vary by BALLAST_E2E_OUTCOME_FILE so
	// Playwright can exercise the sidebar's account-identity fallbacks
	// (User Story 1): "userinfo-name-only" omits the photo, "userinfo-neither"
	// omits both, and anything else (including the default "approve")
	// returns the full name+picture case.
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]any{
			"sub":   "e2e-mock-user-id",
			"email": "e2e-mock-user@example.com",
		}
		switch readE2EOutcome() {
		case "userinfo-neither":
			// no name, no picture
		case "userinfo-name-only":
			info["name"] = "E2E Mock User"
		default:
			info["name"] = "E2E Mock User"
			info["picture"] = srv.URL + "/avatar.png"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})

	mux.HandleFunc("/avatar.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(e2eOnePixelPNG)
	})

	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Files.List always reports no existing folders, so tests always land
	// on "My Drive". "slow-list" and "500-list" (BALLAST_E2E_OUTCOME_FILE)
	// let Playwright reliably trigger the picker's loading/error states
	// (User Story 2) instead of racing real network timing.
	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		switch readE2EOutcome() {
		case "500-list":
			writeE2EDriveError(w, http.StatusInternalServerError, "backendError", "", "internal error listing folders")
			return
		case "slow-list":
			time.Sleep(1500 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]string{}})
	})

	// about.get's storageQuota varies by BALLAST_E2E_OUTCOME_FILE so
	// Playwright can exercise the sidebar's storage-usage fallbacks (User
	// Story 1, spec.md FR-012): "quota-unlimited" omits `limit`, "quota-fail"
	// simulates the call failing outright, and anything else (including the
	// default "approve") returns a normal limited-storage account.
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		switch readE2EOutcome() {
		case "quota-fail":
			writeE2EDriveError(w, http.StatusInternalServerError, "backendError", "", "failed to fetch storage quota")
			return
		case "quota-unlimited":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"storageQuota": map[string]any{"usage": "5368709120"},
			})
			return
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"storageQuota": map[string]any{"usage": "5368709120", "limit": "17179869184"},
			})
		}
	})

	mux.HandleFunc("/upload/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		outcome := readE2EOutcome()

		if outcome == "404-parent" {
			writeE2EDriveError(w, http.StatusNotFound, "notFound", "parents", "the specified parent was not found")
			return
		}

		totalStr := r.Header.Get("X-Upload-Content-Length")
		total, _ := strconv.ParseInt(totalStr, 10, 64)

		mu.Lock()
		session = &e2eUploadSession{total: total}
		mu.Unlock()

		w.Header().Set("Location", "http://"+r.Host+"/upload/drive/v3/files/session")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/upload/drive/v3/files/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			// Upload.Cancel/UploadConfirmRestart's best-effort session
			// release -- always succeeds.
			mu.Lock()
			sessionReleaseCount++
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// E2E test files are only a few KB -- far smaller than a real chunk
		// -- so a whole upload is exactly one request. Without a deliberate
		// pause here, that single request can complete (under whatever
		// outcome was already active) before Playwright's own setOutcome()
		// write lands, racing the test rather than exercising it. This delay
		// gives that fs write a reliable window on every request, regardless
		// of file size or CI runner speed.
		time.Sleep(150 * time.Millisecond)

		outcome := readE2EOutcome()

		mu.Lock()
		hasProgress := session != nil && session.received > 0
		mu.Unlock()

		switch outcome {
		case "network-fail":
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
		case "network-fail-after-progress":
			// Deterministic (not timing-based) version of "network-fail":
			// only fails a request once at least one prior chunk has
			// already been accepted, letting a test reliably pause an
			// upload with a nonzero checkpoint -- e.g. to prove recovery
			// resumes from that checkpoint rather than from 0 -- without
			// racing setOutcome() against how fast the single request
			// completes. Requests before any progress succeed normally.
			if hasProgress {
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
		case "429":
			writeE2EDriveError(w, http.StatusTooManyRequests, "rateLimitExceeded", "", "rate limit exceeded")
			return
		case "503":
			writeE2EDriveError(w, http.StatusServiceUnavailable, "backendError", "", "service unavailable")
			return
		case "403-quota":
			writeE2EDriveError(w, http.StatusForbidden, "storageQuotaExceeded", "", "storage quota exceeded")
			return
		case "404-session":
			writeE2EDriveError(w, http.StatusNotFound, "notFound", "", "session not found")
			return
		case "404-session-after-progress":
			// Deterministic version of "404-session": only expires the
			// session once at least one prior chunk has already been
			// accepted, so a test can reliably reach awaiting_confirmation
			// for a session whose URI is already persisted to the DB
			// (session_uri is only recorded there after a chunk succeeds),
			// without racing setOutcome() against how fast a single-chunk
			// upload completes.
			if hasProgress {
				writeE2EDriveError(w, http.StatusNotFound, "notFound", "", "session not found")
				return
			}
		case "410-session":
			writeE2EDriveError(w, http.StatusGone, "gone", "", "session gone")
			return
		case "404-parent":
			// A destination folder can also be detected gone mid-session,
			// not only at initiation (research.md §4) -- the error body
			// naming "parents" is what distinguishes this from a merely
			// expired session, not the bare status code.
			writeE2EDriveError(w, http.StatusNotFound, "notFound", "parents", "the specified parent was not found")
			return
		}

		contentRange := r.Header.Get("Content-Range")
		isOffsetQuery := strings.HasPrefix(contentRange, "bytes */")

		mu.Lock()
		defer mu.Unlock()

		if session == nil {
			writeE2EDriveError(w, http.StatusNotFound, "notFound", "", "no such session")
			return
		}

		if isOffsetQuery {
			writeE2ESessionStatus(w, session)
			return
		}

		spec := strings.TrimPrefix(contentRange, "bytes ")
		rangePart, totalPart, _ := strings.Cut(spec, "/")
		startPart, endPart, _ := strings.Cut(rangePart, "-")
		start, _ := strconv.ParseInt(startPart, 10, 64)
		end, _ := strconv.ParseInt(endPart, 10, 64)
		total, _ := strconv.ParseInt(totalPart, 10, 64)

		if start != session.received {
			writeE2ESessionStatus(w, session)
			return
		}

		n := end - start + 1
		_, _ = io.CopyN(io.Discard, r.Body, n)
		session.received = end + 1
		session.total = total

		if session.received >= total {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "e2e-mock-file-id",
				"webViewLink": "https://drive.google.com/file/d/e2e-mock-file-id/view",
			})
			return
		}

		w.Header().Set("Range", "bytes=0-"+strconv.FormatInt(session.received-1, 10))
		w.WriteHeader(resumeIncomplete)
	})

	// /debug/session-release-count backs Debug.SessionReleaseCount
	// (app.go), E2E-test-only plumbing that lets Playwright assert
	// UploadConfirmRestart/UploadCancel actually told Drive to release a
	// stale session, not just abandoned it locally.
	mux.HandleFunc("/debug/session-release-count", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count := sessionReleaseCount
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": count})
	})

	srv = httptest.NewServer(mux)
	return srv
}

// e2eOnePixelPNG is a minimal valid 1x1 transparent PNG, served at
// /avatar.png so the sidebar's <img> for a mocked profile photo actually
// loads successfully rather than firing its error/fallback path.
var e2eOnePixelPNG = func() []byte {
	data, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		panic(err)
	}
	return data
}()

// writeE2ESessionStatus must be called with the session's guarding mutex held.
func writeE2ESessionStatus(w http.ResponseWriter, session *e2eUploadSession) {
	if session.total > 0 && session.received >= session.total {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "e2e-mock-file-id",
			"webViewLink": "https://drive.google.com/file/d/e2e-mock-file-id/view",
		})
		return
	}
	if session.received > 0 {
		w.Header().Set("Range", "bytes=0-"+strconv.FormatInt(session.received-1, 10))
	}
	w.WriteHeader(resumeIncomplete)
}

func writeE2EDriveError(w http.ResponseWriter, status int, reason, location, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
			"errors": []map[string]string{
				{"reason": reason, "location": location, "message": message},
			},
		},
	})
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
