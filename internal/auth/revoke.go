package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"ballast/internal/logging"
)

// GoogleRevokeEndpoint is Google's OAuth token-revocation endpoint
// (FR-003).
const GoogleRevokeEndpoint = "https://oauth2.googleapis.com/revoke"

// Revoke POSTs the given token to Google's revocation endpoint. Works for
// either an access or a refresh token; FR-003 revokes using the stored
// refresh token, which revokes the entire grant.
func Revoke(ctx context.Context, client *http.Client, endpoint, token string) error {
	body := strings.NewReader(url.Values{"token": {token}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("auth: build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("auth: revoke request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("auth: revoke endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// SignOut revokes the OAuth grant server-side (FR-003) and then always
// calls deleteAccount, regardless of whether the revoke call succeeded --
// the user must never be left "signed in" locally against a dead or
// unreachable grant (data-model.md's Account validation rules). Only
// deleteAccount's own error (if any) is returned; a revoke failure is
// logged (never including the token itself, per Constitution Principle IV)
// and otherwise swallowed.
func SignOut(ctx context.Context, client *http.Client, revokeEndpoint, refreshToken string, deleteAccount func() error) error {
	if refreshToken != "" {
		if err := Revoke(ctx, client, revokeEndpoint, refreshToken); err != nil {
			logging.Warn("failed to revoke OAuth grant at Google; proceeding with local sign-out anyway", "error", err)
		}
	}
	if err := deleteAccount(); err != nil {
		return fmt.Errorf("auth: sign-out local cleanup failed: %w", err)
	}
	return nil
}
