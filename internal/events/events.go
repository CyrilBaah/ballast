// Package events defines the Wails events Ballast emits from Go to the
// frontend, and small helpers to emit them consistently.
package events

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Event names emitted to the frontend.
const (
	AuthChanged           = "auth:changed"
	UploadProgress        = "upload:progress"
	UploadPaused          = "upload:paused"
	UploadAwaitingConfirm = "upload:awaiting-confirmation"
	UploadComplete        = "upload:complete"
	UploadFailed          = "upload:failed"
)

// AuthStatus is the payload of AuthChanged, also returned by Auth.GetStatus/Auth.SignIn.
type AuthStatus struct {
	SignedIn bool   `json:"signedIn"`
	Email    string `json:"email,omitempty"`
	// Name and PictureURL are Google's display name/profile photo (FR-011,
	// research.md §7); either can be absent (older accounts, or Google
	// simply not returning one) -- the sidebar falls back to Email and a
	// generated-initials avatar in that case.
	Name       string `json:"name,omitempty"`
	PictureURL string `json:"pictureUrl,omitempty"`
}

// UploadProgressPayload mirrors the upload:progress event payload.
type UploadProgressPayload struct {
	ID         int64 `json:"id"`
	BytesSent  int64 `json:"bytesSent"`
	TotalBytes int64 `json:"totalBytes"`
}

// UploadCompletePayload mirrors the upload:complete event payload.
type UploadCompletePayload struct {
	ID            int64  `json:"id"`
	DriveFileLink string `json:"driveFileLink"`
}

// UploadFailedPayload mirrors the upload:failed event payload.
type UploadFailedPayload struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// UploadPausedPayload mirrors the upload:paused event payload.
type UploadPausedPayload struct {
	ID            int64  `json:"id"`
	RetryingSince string `json:"retryingSince"`
}

// UploadAwaitingConfirmationPayload mirrors the upload:awaiting-confirmation event payload.
type UploadAwaitingConfirmationPayload struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// EmitAuthChanged notifies the frontend of a sign-in, sign-out, or a
// force-cleared session (e.g. an unrenewable revoked/expired refresh token).
func EmitAuthChanged(ctx context.Context, status AuthStatus) {
	runtime.EventsEmit(ctx, AuthChanged, status)
}

// EmitUploadProgress notifies the frontend of upload progress. Callers are
// responsible for throttling call frequency — this helper does not throttle itself.
func EmitUploadProgress(ctx context.Context, id, bytesSent, totalBytes int64) {
	runtime.EventsEmit(ctx, UploadProgress, UploadProgressPayload{
		ID:         id,
		BytesSent:  bytesSent,
		TotalBytes: totalBytes,
	})
}

// EmitUploadComplete notifies the frontend that an upload succeeded.
func EmitUploadComplete(ctx context.Context, id int64, driveFileLink string) {
	runtime.EventsEmit(ctx, UploadComplete, UploadCompletePayload{
		ID:            id,
		DriveFileLink: driveFileLink,
	})
}

// EmitUploadFailed notifies the frontend that an upload failed. Every
// in_progress upload must eventually emit either this or EmitUploadComplete.
func EmitUploadFailed(ctx context.Context, id int64, reason string) {
	runtime.EventsEmit(ctx, UploadFailed, UploadFailedPayload{
		ID:     id,
		Reason: reason,
	})
}

// EmitUploadPaused notifies the frontend that a retryable error was hit --
// the upload keeps auto-retrying and is never shown as a failure (FR-003, FR-007).
func EmitUploadPaused(ctx context.Context, id int64, retryingSince time.Time) {
	runtime.EventsEmit(ctx, UploadPaused, UploadPausedPayload{
		ID:            id,
		RetryingSince: retryingSince.UTC().Format(time.RFC3339),
	})
}

// EmitUploadAwaitingConfirmation notifies the frontend that a
// terminal-but-recoverable condition was detected (research.md §4) --
// auto-retrying stops and the frontend must prompt for confirmation before
// restarting the transfer from byte 0 (FR-010).
func EmitUploadAwaitingConfirmation(ctx context.Context, id int64, reason string) {
	runtime.EventsEmit(ctx, UploadAwaitingConfirm, UploadAwaitingConfirmationPayload{
		ID:     id,
		Reason: reason,
	})
}
