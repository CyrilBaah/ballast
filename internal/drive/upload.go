package drive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"ballast/internal/events"

	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// UploadResult is what a successful upload reports back (FR-008: a
// reference to the uploaded file's location in Drive).
type UploadResult struct {
	FileID      string
	WebViewLink string
}

// UploadFile performs a basic (non-resumable) upload of localPath to the
// Drive folder driveFolderID, using Files.Create(...).Media(...,
// googleapi.ChunkSize(0)) to force the single-request simple-upload path
// rather than the client library's default resumable protocol
// (research.md §3).
//
// The local file is re-stat'd (via os.Open, which stats implicitly)
// immediately before the transfer starts, not just at selection time, to
// catch FR-011's "file moved, renamed, or deleted before the upload
// starts" -- this can be arbitrarily later than when the user originally
// picked the file.
//
// While the transfer is in flight, it emits upload:progress events
// (throttled to ~1/s via countingReader, T034) with id and totalBytes so
// the frontend can render a live progress indicator (FR-007), and calls
// onProgress (if non-nil) with the same cumulative byte count so the
// caller can persist bytes_sent (data-model.md). Exactly one terminal
// event -- upload:complete on success, upload:failed on any error -- is
// guaranteed to fire before this function returns, satisfying
// FR-009/SC-006's "never leave the user in an indefinite or ambiguous
// waiting state."
func UploadFile(ctx context.Context, svc *drivev3.Service, id int64, localPath, driveFolderID string, totalBytes int64, onProgress func(bytesSent int64)) (*UploadResult, error) {
	f, err := os.Open(localPath)
	if err != nil {
		reason := fmt.Sprintf("local file can no longer be found: %v", err)
		events.EmitUploadFailed(ctx, id, reason)
		return nil, fmt.Errorf("drive: %s", reason)
	}
	defer f.Close()

	meta := &drivev3.File{
		Name:    filepath.Base(localPath),
		Parents: []string{driveFolderID},
	}

	cr := newCountingReader(f, progressEmitThrottle, func(bytesRead int64) {
		events.EmitUploadProgress(ctx, id, bytesRead, totalBytes)
		if onProgress != nil {
			onProgress(bytesRead)
		}
	})

	file, err := svc.Files.Create(meta).
		Media(cr, googleapi.ChunkSize(0)).
		Fields("id, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		reason := fmt.Sprintf("upload failed: %v", err)
		events.EmitUploadFailed(ctx, id, reason)
		return nil, fmt.Errorf("drive: %s", reason)
	}

	events.EmitUploadComplete(ctx, id, file.WebViewLink)
	return &UploadResult{FileID: file.Id, WebViewLink: file.WebViewLink}, nil
}
