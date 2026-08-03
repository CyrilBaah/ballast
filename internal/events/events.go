// Package events defines the Wails events this feature emits from Go to the
// frontend (contracts/wails-bindings.md's "Emitted events" section) and
// small helpers to emit them consistently.
package events

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Event names, exactly as specified in contracts/wails-bindings.md.
const (
	AuthChanged    = "auth:changed"
	UploadProgress = "upload:progress"
	UploadComplete = "upload:complete"
	UploadFailed   = "upload:failed"
)

// AuthStatus mirrors contracts/wails-bindings.md's AuthStatus type. Emitted
// as the payload of AuthChanged, and returned by Auth.GetStatus/Auth.SignIn.
type AuthStatus struct {
	SignedIn bool   `json:"signedIn"`
	Email    string `json:"email,omitempty"`
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

// EmitAuthChanged notifies the frontend of a sign-in, sign-out, or a
// force-cleared session (e.g. an unrenewable revoked/expired refresh
// token — Edge Case in spec.md).
func EmitAuthChanged(ctx context.Context, status AuthStatus) {
	runtime.EventsEmit(ctx, AuthChanged, status)
}

// EmitUploadProgress notifies the frontend of upload progress. Callers
// (internal/drive's counting-reader wrapper, T034) are responsible for
// throttling call frequency — this helper does not throttle itself.
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
// in_progress upload MUST eventually emit either EmitUploadComplete or
// EmitUploadFailed, never neither (FR-009, SC-006).
func EmitUploadFailed(ctx context.Context, id int64, reason string) {
	runtime.EventsEmit(ctx, UploadFailed, UploadFailedPayload{
		ID:     id,
		Reason: reason,
	})
}
