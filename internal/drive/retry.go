package drive

import (
	"errors"
	"time"

	"golang.org/x/oauth2"
)

// ErrorBucket is the three-way classification research.md §4 assigns to
// every retryable/terminal condition hit while transferring a chunk.
type ErrorBucket int

const (
	Retryable ErrorBucket = iota
	TerminalRecoverable
	TerminalNotRecoverable
)

// Reason strings for the two TerminalRecoverable conditions, matching
// data-model.md's awaiting_confirmation_reason enum.
const (
	ReasonSessionExpired = "session_expired"
	ReasonFileChanged    = "file_changed"
)

// Classification is the outcome of classifying one failed chunk-send/
// offset-query/session-initiate attempt: which bucket it falls into, and
// the reason to surface -- one of the Reason* constants for
// TerminalRecoverable, or free-text user-facing copy for
// TerminalNotRecoverable/Retryable.
type Classification struct {
	Bucket ErrorBucket
	Reason string
}

// ClassifyTransportError maps a transport-level failure (no HTTP response
// from Drive itself) to its bucket. The oauth2-wrapped http.Client used
// for every resumable-protocol call refreshes an expiring access token
// transparently before each request; if that refresh itself fails (e.g.
// the refresh token was revoked -- permission revoked, research.md §4),
// the client surfaces an *oauth2.RetrieveError here rather than an HTTP
// response, so it must be classified as terminal, not endlessly retried
// like a genuine network drop (connection reset, timeout, EOF mid-chunk).
func ClassifyTransportError(err error) Classification {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return Classification{Bucket: TerminalNotRecoverable, Reason: "signed out — sign in again"}
	}
	return Classification{Bucket: Retryable, Reason: "network error: " + err.Error()}
}

// ClassifyDriveError maps a parsed Drive API error response to its bucket,
// per research.md §4's table. isSessionInitiation distinguishes a 404 on
// session *initiation* (or one whose error body names the parents field)
// -- meaning the destination folder itself is gone, not recoverable -- from
// a 404/410 on an already-initiated session's own URI, meaning just the
// session expired, which is recoverable by restarting the same logical
// upload with a fresh session.
func ClassifyDriveError(de *DriveError, isSessionInitiation bool) Classification {
	destinationGone := isSessionInitiation || de.Location == "parents"

	switch {
	case de.StatusCode == 403 && de.Reason == "storageQuotaExceeded":
		return Classification{Bucket: TerminalNotRecoverable, Reason: "Google Drive storage is full"}
	case de.StatusCode == 429,
		de.StatusCode == 403 && (de.Reason == "rateLimitExceeded" || de.Reason == "userRateLimitExceeded"),
		de.StatusCode >= 500 && de.StatusCode < 600:
		return Classification{Bucket: Retryable, Reason: de.Message}
	case de.StatusCode == 404 && destinationGone:
		return Classification{Bucket: TerminalNotRecoverable, Reason: "the destination folder no longer exists"}
	case de.StatusCode == 404 || de.StatusCode == 410:
		return Classification{Bucket: TerminalRecoverable, Reason: ReasonSessionExpired}
	default:
		return Classification{Bucket: TerminalNotRecoverable, Reason: de.Message}
	}
}

// Two-tier backoff policy for retryable errors (research.md §4): a fast,
// fixed interval for the first FastTierDuration of continuous failure,
// then an exponential tier (base EscalatingTierBase, x2 per attempt)
// capped at EscalatingTierCap. Per Constitution Principle III, both
// intervals are explicitly starting hypotheses pending harness
// validation, not settled defaults.
const (
	FastTierInterval   = 2 * time.Second
	FastTierDuration   = 30 * time.Second
	EscalatingTierBase = 2 * time.Second
	EscalatingTierCap  = 30 * time.Second
)

// BackoffPolicy tracks one upload's current unbroken run of retryable
// failures and computes how long to wait before the next attempt. Retries
// never stop on their own for a retryable error (FR-007) -- there is no
// attempt-count ceiling.
type BackoffPolicy struct {
	firstFailureAt     time.Time
	escalatingAttempts int
	now                func() time.Time
}

// NewBackoffPolicy returns a policy with no failures recorded yet.
func NewBackoffPolicy() *BackoffPolicy {
	return &BackoffPolicy{now: time.Now}
}

// NextDelay returns how long to wait before the next retry attempt,
// recording this call as another failure in the current streak. Call
// Reset once a chunk send succeeds, ending the streak.
func (b *BackoffPolicy) NextDelay() time.Duration {
	now := b.now()
	if b.firstFailureAt.IsZero() {
		b.firstFailureAt = now
	}

	if now.Sub(b.firstFailureAt) < FastTierDuration {
		return FastTierInterval
	}

	delay := EscalatingTierBase
	for i := 0; i < b.escalatingAttempts && delay < EscalatingTierCap; i++ {
		delay *= 2
	}
	if delay > EscalatingTierCap {
		delay = EscalatingTierCap
	}
	b.escalatingAttempts++
	return delay
}

// Reset clears the failure streak after a successful chunk send.
func (b *BackoffPolicy) Reset() {
	b.firstFailureAt = time.Time{}
	b.escalatingAttempts = 0
}
