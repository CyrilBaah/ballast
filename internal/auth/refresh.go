package auth

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// refreshSkew is how much lead time before the recorded expiry we treat the
// access token as "needs refresh" -- avoids a request racing an
// about-to-expire token.
const refreshSkew = 2 * time.Minute

// NeedsRefresh reports whether the access token expiring at expiry should
// be silently refreshed before making a Drive API call (data-model.md:
// "Used to decide whether a silent refresh is needed before an API call").
func NeedsRefresh(expiry time.Time) bool {
	return time.Now().Add(refreshSkew).After(expiry)
}

// RefreshAccessToken exchanges a stored refresh token for a fresh access
// token, without any user interaction. Returns the new token (including a
// possibly-rotated refresh token -- Google does not always issue a new one,
// in which case callers should keep the existing refresh token).
func RefreshAccessToken(ctx context.Context, cfg *oauth2.Config, refreshToken string) (*oauth2.Token, error) {
	src := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: silent refresh failed: %w", err)
	}
	return tok, nil
}
