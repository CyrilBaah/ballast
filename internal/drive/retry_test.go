package drive

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestClassifyTransportErrorIsRetryableForGenericNetworkFailure(t *testing.T) {
	c := ClassifyTransportError(errors.New("connection reset by peer"))
	if c.Bucket != Retryable {
		t.Fatalf("bucket = %v, want Retryable", c.Bucket)
	}
}

// TestClassifyTransportErrorTreatsRevokedRefreshTokenAsTerminal covers
// research.md §4's "permission revoked" row: the oauth2-wrapped HTTP
// client refreshes the access token transparently before each request, so
// a revoked refresh token surfaces as an *oauth2.RetrieveError from
// client.Do, not an HTTP response -- this must stop retrying, not loop
// forever like a genuine dropped connection.
func TestClassifyTransportErrorTreatsRevokedRefreshTokenAsTerminal(t *testing.T) {
	retrieveErr := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "token has been revoked"}
	wrapped := fmt.Errorf("Put \"https://example.test\": %w", retrieveErr)

	c := ClassifyTransportError(wrapped)
	if c.Bucket != TerminalNotRecoverable {
		t.Fatalf("bucket = %v, want TerminalNotRecoverable", c.Bucket)
	}
}

func TestClassifyDriveErrorTable(t *testing.T) {
	cases := []struct {
		name                string
		de                  *DriveError
		isSessionInitiation bool
		wantBucket          ErrorBucket
		wantReason          string
	}{
		{
			name:       "429 rate limit",
			de:         &DriveError{StatusCode: 429},
			wantBucket: Retryable,
		},
		{
			name:       "403 rateLimitExceeded",
			de:         &DriveError{StatusCode: 403, Reason: "rateLimitExceeded"},
			wantBucket: Retryable,
		},
		{
			name:       "403 userRateLimitExceeded",
			de:         &DriveError{StatusCode: 403, Reason: "userRateLimitExceeded"},
			wantBucket: Retryable,
		},
		{
			name:       "500",
			de:         &DriveError{StatusCode: 500},
			wantBucket: Retryable,
		},
		{
			name:       "503",
			de:         &DriveError{StatusCode: 503},
			wantBucket: Retryable,
		},
		{
			name:       "403 storageQuotaExceeded",
			de:         &DriveError{StatusCode: 403, Reason: "storageQuotaExceeded"},
			wantBucket: TerminalNotRecoverable,
			wantReason: "Google Drive storage is full",
		},
		{
			name:       "404 on session URI -- expired session",
			de:         &DriveError{StatusCode: 404, Reason: "notFound"},
			wantBucket: TerminalRecoverable,
			wantReason: ReasonSessionExpired,
		},
		{
			name:       "410 on session URI -- expired session",
			de:         &DriveError{StatusCode: 410},
			wantBucket: TerminalRecoverable,
			wantReason: ReasonSessionExpired,
		},
		{
			name:                "404 on session initiation -- missing parent folder",
			de:                  &DriveError{StatusCode: 404, Reason: "notFound", Location: "parents"},
			isSessionInitiation: true,
			wantBucket:          TerminalNotRecoverable,
			wantReason:          "the destination folder no longer exists",
		},
		{
			name:       "404 naming parents even mid-session -- missing destination, not expired session",
			de:         &DriveError{StatusCode: 404, Reason: "notFound", Location: "parents"},
			wantBucket: TerminalNotRecoverable,
			wantReason: "the destination folder no longer exists",
		},
		{
			name:       "401 permission revoked",
			de:         &DriveError{StatusCode: 401},
			wantBucket: TerminalNotRecoverable,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyDriveError(c.de, c.isSessionInitiation)
			if got.Bucket != c.wantBucket {
				t.Fatalf("bucket = %v, want %v", got.Bucket, c.wantBucket)
			}
			if c.wantReason != "" && got.Reason != c.wantReason {
				t.Fatalf("reason = %q, want %q", got.Reason, c.wantReason)
			}
		})
	}
}

func TestBackoffPolicyFastTierIsFixedInterval(t *testing.T) {
	b := NewBackoffPolicy()
	fakeNow := time.Unix(0, 0)
	b.now = func() time.Time { return fakeNow }

	for i := 0; i < 5; i++ {
		got := b.NextDelay()
		if got != FastTierInterval {
			t.Fatalf("attempt %d: delay = %v, want fast-tier %v", i, got, FastTierInterval)
		}
		fakeNow = fakeNow.Add(FastTierInterval)
	}
}

func TestBackoffPolicyEscalatesAfterFastTierDuration(t *testing.T) {
	b := NewBackoffPolicy()
	fakeNow := time.Unix(0, 0)
	b.now = func() time.Time { return fakeNow }

	// Exhaust the fast tier.
	b.NextDelay()
	fakeNow = fakeNow.Add(FastTierDuration)

	want := []time.Duration{
		EscalatingTierBase,     // 2s
		EscalatingTierBase * 2, // 4s
		EscalatingTierBase * 4, // 8s
		EscalatingTierBase * 8, // 16s
		EscalatingTierCap,      // 30s (32s would exceed the cap)
		EscalatingTierCap,      // stays capped
	}
	for i, w := range want {
		got := b.NextDelay()
		if got != w {
			t.Fatalf("escalating attempt %d: delay = %v, want %v", i, got, w)
		}
	}
}

func TestBackoffPolicyResetReturnsToFastTier(t *testing.T) {
	b := NewBackoffPolicy()
	fakeNow := time.Unix(0, 0)
	b.now = func() time.Time { return fakeNow }

	b.NextDelay()
	fakeNow = fakeNow.Add(FastTierDuration)
	b.NextDelay() // now in escalating tier

	b.Reset()
	fakeNow = fakeNow.Add(time.Second)
	got := b.NextDelay()
	if got != FastTierInterval {
		t.Fatalf("delay after Reset = %v, want fast-tier %v", got, FastTierInterval)
	}
}

func TestBackoffPolicyNeverStopsRetrying(t *testing.T) {
	b := NewBackoffPolicy()
	fakeNow := time.Unix(0, 0)
	b.now = func() time.Time { return fakeNow }

	// FR-007: no ceiling on retry attempts for a retryable error -- run
	// far more attempts than any reasonable outage and confirm it keeps
	// returning a valid, capped delay rather than erroring or zeroing out.
	for i := 0; i < 1000; i++ {
		fakeNow = fakeNow.Add(EscalatingTierCap)
		got := b.NextDelay()
		if got <= 0 || got > EscalatingTierCap {
			t.Fatalf("attempt %d: delay = %v, out of bounds (0, %v]", i, got, EscalatingTierCap)
		}
	}
}
