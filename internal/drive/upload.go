package drive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
func UploadFile(ctx context.Context, svc *drivev3.Service, localPath, driveFolderID string) (*UploadResult, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("drive: local file can no longer be found: %w", err)
	}
	defer f.Close()

	meta := &drivev3.File{
		Name:    filepath.Base(localPath),
		Parents: []string{driveFolderID},
	}

	file, err := svc.Files.Create(meta).
		Media(f, googleapi.ChunkSize(0)).
		Fields("id, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("drive: upload failed: %w", err)
	}

	return &UploadResult{FileID: file.Id, WebViewLink: file.WebViewLink}, nil
}
