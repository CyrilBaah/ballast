package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ballast/internal/auth"
	"ballast/internal/storage"

	oauth2pkg "golang.org/x/oauth2"
)

func TestRefreshIsPermanent(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"revoked grant", &oauth2pkg.RetrieveError{ErrorCode: "invalid_grant"}, true},
		{"token endpoint down", &oauth2pkg.RetrieveError{Response: &http.Response{StatusCode: 503}}, false},
		{"rate limited", &oauth2pkg.RetrieveError{Response: &http.Response{StatusCode: 429}}, false},
		{"no internet", fmt.Errorf("Post https://oauth2.googleapis.com/token: dial tcp: lookup failed"), false},
		{"tokens can't be decrypted", fmt.Errorf("x: %w", errSessionUnusable), true},
	}
	for _, c := range cases {
		if got := refreshIsPermanent(c.err); got != c.want {
			t.Errorf("%s: refreshIsPermanent = %v, want %v", c.name, got, c.want)
		}
	}
}

// newAuthTestApp signs an account in with an access token that has
// already expired, against a token endpoint that answers with status.
func newAuthTestApp(t *testing.T, status int, body string) *App {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	db, err := storage.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := NewApp()
	a.ctx = context.Background()
	a.db = db
	a.encKey = make([]byte, 32)
	a.oauthEndpointOverride = &oauth2pkg.Endpoint{TokenURL: srv.URL}
	if err := a.persistSession(&auth.Session{
		Email: "me@example.com", GoogleUserID: "u1",
		AccessToken: "old-access", RefreshToken: "refresh",
		Expiry: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAuthGetStatusKeepsSessionThroughTemporaryRefreshFailure(t *testing.T) {
	a := newAuthTestApp(t, http.StatusServiceUnavailable, `{"error":"backend_error"}`)

	status := a.AuthGetStatus()

	if !status.SignedIn || status.Email != "me@example.com" {
		t.Fatalf("AuthGetStatus = %+v; a brief token-endpoint outage must not sign the user out", status)
	}
	if _, err := a.db.GetAccount(); err != nil {
		t.Fatalf("the stored session was deleted: %v", err)
	}
}

func TestDriveClientKeepsSessionThroughTemporaryRefreshFailure(t *testing.T) {
	a := newAuthTestApp(t, http.StatusServiceUnavailable, `{"error":"backend_error"}`)

	client, err := a.driveHTTPClient(context.Background())

	if err != nil || client == nil {
		t.Fatalf("driveHTTPClient = %v, %v; want a client that retries the refresh itself", client, err)
	}
	if _, err := a.db.GetAccount(); err != nil {
		t.Fatalf("the stored session was deleted: %v", err)
	}
	if errors.Is(err, errSignedOut) {
		t.Fatal("reported signed out")
	}
}
