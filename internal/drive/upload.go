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

// UploadResult is what a successful upload reports back: a reference to
// the uploaded file's location in Drive.
type UploadResult struct {
	FileID      string
	WebViewLink string
}

// UploadFile performs a basic (non-resumable) upload of localPath to the
// Drive folder driveFolderID. While the transfer is in flight it emits
// upload:progress events and calls onProgress (if non-nil) with the
// cumulative byte count; exactly one terminal event — upload:complete or
// upload:failed — always fires before this function returns.
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
