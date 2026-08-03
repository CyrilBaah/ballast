package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevokeSendsTokenToEndpoint(t *testing.T) {
	var gotToken string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotToken = r.Form.Get("token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := Revoke(context.Background(), srv.Client(), srv.URL, "test-refresh-token")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotToken != "test-refresh-token" {
		t.Errorf("token = %q, want %q", gotToken, "test-refresh-token")
	}
}

func TestRevokeReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := Revoke(context.Background(), srv.Client(), srv.URL, "test-refresh-token"); err == nil {
		t.Fatal("expected Revoke to return an error on a 500 response")
	}
}

// TestSignOutProceedsWithDeletionEvenWhenRevokeFails guards against leaving
// the user signed in locally after a failed revoke call.
func TestSignOutProceedsWithDeletionEvenWhenRevokeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	deleteCalled := false
	deleteAccount := func() error {
		deleteCalled = true
		return nil
	}

	err := SignOut(context.Background(), srv.Client(), srv.URL, "test-refresh-token", deleteAccount)
	if err != nil {
		t.Fatalf("SignOut returned an error even though deleteAccount succeeded: %v", err)
	}
	if !deleteCalled {
		t.Fatal("deleteAccount was not called after the mocked revoke call failed")
	}
}

func TestSignOutSurfacesDeleteAccountError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wantErr := errors.New("db is locked")
	deleteAccount := func() error { return wantErr }

	err := SignOut(context.Background(), srv.Client(), srv.URL, "test-refresh-token", deleteAccount)
	if !errors.Is(err, wantErr) {
		t.Fatalf("SignOut error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestSignOutSkipsRevokeWhenNoRefreshToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	deleteCalled := false
	deleteAccount := func() error {
		deleteCalled = true
		return nil
	}

	if err := SignOut(context.Background(), srv.Client(), srv.URL, "", deleteAccount); err != nil {
		t.Fatalf("SignOut: %v", err)
	}
	if called {
		t.Error("Revoke endpoint should not be called with an empty refresh token")
	}
	if !deleteCalled {
		t.Fatal("deleteAccount was not called")
	}
}
