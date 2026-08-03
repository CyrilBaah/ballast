// Package logging provides Ballast's structured logging, built on log/slog.
//
// Credentials must never appear in logs at any level: no OAuth tokens,
// ciphertext, nonces, encryption keys, redirect query strings, or PKCE
// verifiers. Non-secret identifiers (email, user ID, upload IDs, file
// paths, byte counts, status codes) are fine to log.
package logging

import (
	"log/slog"
	"os"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

// Redacted wraps a value that must never be written to a log record; it
// always renders as "[REDACTED]" as a defense-in-depth backstop. Prefer not logging the value at all.
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
