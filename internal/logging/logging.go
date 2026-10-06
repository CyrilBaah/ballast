// Package logging provides Ballast's structured logging, built on log/slog.
//
// Credentials must never appear in logs at any level: no OAuth tokens,
// ciphertext, nonces, encryption keys, redirect query strings, or PKCE
// verifiers. Non-secret identifiers (email, user ID, upload IDs, file
// paths, byte counts, status codes) are fine to log.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// logFileName is the log file kept in the app data directory, and
// maxLogFileBytes the size past which it is rotated to logFileName+".1"
// at startup, so the log never grows without bound.
const (
	logFileName     = "ballast.log"
	maxLogFileBytes = 10 << 20
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

// LogToFile adds a log file in dir alongside stderr, so a transfer's
// history survives the process -- stderr goes nowhere once the app is
// launched from Finder. The file is rotated first if it has grown past
// maxLogFileBytes. On failure logging carries on to stderr only.
func LogToFile(dir string) error {
	path := filepath.Join(dir, logFileName)
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogFileBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	logger = slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, f), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	return nil
}

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
