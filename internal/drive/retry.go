package drive

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// ErrorBucket is the classification research.md §4 assigns to every
// retryable/terminal condition hit while transferring a chunk.
type ErrorBucket int

const (
	Retryable ErrorBucket = iota
	TerminalRecoverable
	TerminalNotRecoverable
	// TerminalNeedsSignIn stops this transfer without failing it: the
	// stored Google session is gone, so the oauth2-wrapped client this
	// upload was started with can never recover on its own (its token
	// source holds the dead refresh token, and a later sign-in mints a
	// new one it will never see). The checkpoint stays intact and the
	// upload stays non-terminal, so signing back in resumes it from the
	// last acknowledged byte rather than restarting it from zero.
	TerminalNeedsSignIn
)

// Reason strings for the two TerminalRecoverable conditions, matching
// data-model.md's awaiting_confirmation_reason enum.
const (
	ReasonSessionExpired = "session_expired"
	ReasonFileChanged    = "file_changed"
)

// ReasonNeedsSignIn is the user-facing copy for TerminalNeedsSignIn. It
// describes a transfer that is holding its place, not one that failed --
// the upload picks up from its last acknowledged byte once the user signs
// back in.
const ReasonNeedsSignIn = "signed out — sign in again to continue"

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
// transparently before each request; if that refresh itself fails, the
// client surfaces an *oauth2.RetrieveError here rather than an HTTP
// response. That covers two very different situations, so it is split
// rather than lumped together: a grant that is genuinely gone (revoked --
// research.md §4) can only be resolved by signing in again, while the
// token endpoint merely being unhappy right now (503, 429) is as
// transient as any network drop and must not cost the user a
// part-transferred file. Everything else -- connection reset, timeout,
// EOF mid-chunk -- is a plain retryable network error.
func ClassifyTransportError(err error) Classification {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if refreshTokenIsDead(retrieveErr) {
			return Classification{Bucket: TerminalNeedsSignIn, Reason: ReasonNeedsSignIn}
		}
		return Classification{Bucket: Retryable, Reason: "could not refresh the Google session: " + refreshFailureDetail(retrieveErr)}
	}
	return Classification{Bucket: Retryable, Reason: "network error: " + err.Error()}
}

// refreshFailureDetail describes a failed token refresh for the reason
// string. It reads RetrieveError's fields directly rather than calling its
// Error method, which dereferences Response unconditionally once ErrorCode
// is empty.
func refreshFailureDetail(err *oauth2.RetrieveError) string {
	switch {
	case err.ErrorCode != "":
		return err.ErrorCode
	case err.Response != nil:
		return fmt.Sprintf("HTTP %d", err.Response.StatusCode)
	default:
		return "unknown error"
	}
}

// deadGrantErrorCodes are the RFC 6749 'error' values that mean the stored
// refresh token will never work again no matter how long we wait, so the
// only way forward is a fresh sign-in. Google returns invalid_grant for a
// revoked, expired, or password-change-invalidated refresh token.
var deadGrantErrorCodes = map[string]bool{
	"invalid_grant":       true,
	"invalid_client":      true,
	"unauthorized_client": true,
	"invalid_scope":       true,
}

// refreshTokenIsDead reports whether a failed token refresh is permanent
// (needs a new sign-in) rather than something a retry could clear. It
// prefers the RFC 6749 error code, falling back to the status code for a
// response whose body wasn't a parseable OAuth error -- 4xx other than 429
// is the client's own credentials being rejected; 5xx and 429 are the
// token endpoint having a bad moment, which is retryable.
func refreshTokenIsDead(err *oauth2.RetrieveError) bool {
	if err.ErrorCode != "" {
		return deadGrantErrorCodes[err.ErrorCode]
	}
	if err.Response == nil {
		return false
	}
	code := err.Response.StatusCode
	return code >= 400 && code < 500 && code != http.StatusTooManyRequests
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
