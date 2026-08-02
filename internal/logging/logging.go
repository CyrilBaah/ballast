// Package logging provides Ballast's structured logging, built on the
// standard library's log/slog.
//
// Constitution Principle IV, verbatim: "Credentials MUST NOT appear in logs
// at any log level, including debug." This is a hard rule, not a style
// preference:
//
//   - Never log an OAuth access token, refresh token, encrypted ciphertext
//     blob, GCM nonce, or the AES data-encryption key itself — not even at
//     Debug level, not even truncated/partially redacted.
//   - Never log the raw Google OAuth consent redirect URL's query string
//     (it carries the authorization code) or the PKCE code verifier.
//   - It is fine to log non-secret identifiers: email address (already
//     shown in the UI per data-model.md), google_user_id, upload IDs, file
//     paths, byte counts, HTTP status codes, and error messages that don't
//     themselves embed a secret.
//   - When logging an error returned from the oauth2/keyring/Drive
//     packages, check that the error's message doesn't interpolate a
//     token — Go's oauth2 package errors do not include the raw token in
//     their Error() string, but callers constructing their own wrapped
//     errors MUST NOT do so either (see internal/logging's audit note and
//     T038).
package logging

import (
	"log/slog"
	"os"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

// Redacted wraps a value that must never be written to a log record. Its
// LogValue implementation always renders as "[REDACTED]" regardless of the
// underlying value, so even an accidental attempt to log a secret (e.g.
// `logging.Info("token", "value", logging.Redacted{V: token})`) cannot leak
// it. Prefer simply not logging the value at all; this exists as a
// defense-in-depth backstop, not an invitation to log secrets "safely".
type Redacted struct {
	V any
}

// LogValue implements slog.LogValuer.
func (Redacted) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// Debug logs at debug level. See package doc for what MUST NOT be passed.
func Debug(msg string, args ...any) { logger.Debug(msg, args...) }

// Info logs at info level. See package doc for what MUST NOT be passed.
func Info(msg string, args ...any) { logger.Info(msg, args...) }

// Warn logs at warn level. See package doc for what MUST NOT be passed.
func Warn(msg string, args ...any) { logger.Warn(msg, args...) }

// Error logs at error level. See package doc for what MUST NOT be passed.
func Error(msg string, args ...any) { logger.Error(msg, args...) }
