package auth

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// refreshSkew is the lead time before expiry at which a token is treated as
// "needs refresh", avoiding a request racing an about-to-expire token.
const refreshSkew = 2 * time.Minute

// NeedsRefresh reports whether the access token expiring at expiry should
// be silently refreshed before making a Drive API call.
func NeedsRefresh(expiry time.Time) bool {
	return time.Now().Add(refreshSkew).After(expiry)
}

// RefreshAccessToken exchanges a stored refresh token for a fresh access
// token, without any user interaction. Google does not always rotate the
// refresh token, so callers should keep the existing one if none is returned.
func RefreshAccessToken(ctx context.Context, cfg *oauth2.Config, refreshToken string) (*oauth2.Token, error) {
	src := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: silent refresh failed: %w", err)
	}
	return tok, nil
}
