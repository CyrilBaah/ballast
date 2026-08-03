package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"ballast/internal/logging"
)

// GoogleRevokeEndpoint is Google's OAuth token-revocation endpoint.
const GoogleRevokeEndpoint = "https://oauth2.googleapis.com/revoke"

// Revoke POSTs the given token to Google's revocation endpoint. Works for
// either an access or a refresh token; revoking the refresh token kills the
// entire grant, which is what we want on sign-out.
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

// SignOut revokes the OAuth grant server-side and always calls
// deleteAccount afterward, so a failed revoke never leaves the user signed
// in locally. Only deleteAccount's own error, if any, is returned.
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
