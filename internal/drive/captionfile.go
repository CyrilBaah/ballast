package drive

import (
	"context"
	"fmt"
	"io"
	"strconv"

	drivev3 "google.golang.org/api/drive/v3"
)

// CaptionForAppProperty tags a caption file with the upload it belongs to,
// so a crash between creating it and recording it never leads to a second
// copy (Feature 005 research.md §6) -- the same check-before-re-sending
// rule FindLandedUpload applies to videos.
const CaptionForAppProperty = "ballastCaptionFor"

// srtMimeType is SubRip's media type; Drive's player accepts it as a
// caption track.
const srtMimeType = "application/x-subrip"

// FindCaptionFile returns the non-trashed file in folderID already tagged
// as uploadID's caption file, or nil, nil if there is none.
func FindCaptionFile(ctx context.Context, svc *drivev3.Service, uploadID int64, folderID string) (*UploadResult, error) {
	q := fmt.Sprintf("appProperties has { key='%s' and value='%d' } and '%s' in parents and trashed = false",
		CaptionForAppProperty, uploadID, escapeQueryValue(folderID))
	resp, err := svc.Files.List().Q(q).Fields("files(id, webViewLink)").PageSize(10).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("drive: look for an existing caption file: %w", err)
	}
	for _, f := range resp.Files {
		if f.Id != "" {
			return &UploadResult{FileID: f.Id, WebViewLink: f.WebViewLink}, nil
		}
	}
	return nil, nil
}

// ChooseCaptionName returns base+ext, or the first free "base (N)ext",
// given the names already in folderID -- an existing file is never
// overwritten (FR-014).
func ChooseCaptionName(ctx context.Context, svc *drivev3.Service, folderID, base, ext string) (string, error) {
	q := fmt.Sprintf("name contains '%s' and '%s' in parents and trashed = false",
		escapeQueryValue(base), escapeQueryValue(folderID))
	taken := map[string]bool{}
	pageToken := ""
	for {
		call := svc.Files.List().Q(q).Fields("nextPageToken, files(name)").PageSize(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return "", fmt.Errorf("drive: list names in the destination folder: %w", err)
		}
		for _, f := range resp.Files {
			taken[f.Name] = true
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	name := base + ext
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
	return name, nil
}

// UploadCaptionFile creates a caption file named name in folderID with a
// single small multipart upload -- caption files are well under a
// megabyte, so the resumable engine adds nothing -- tagged with uploadID.
func UploadCaptionFile(ctx context.Context, svc *drivev3.Service, uploadID int64, folderID, name string, content io.Reader) (*UploadResult, error) {
	f := &drivev3.File{
		Name:          name,
		Parents:       []string{folderID},
		MimeType:      srtMimeType,
		AppProperties: map[string]string{CaptionForAppProperty: strconv.FormatInt(uploadID, 10)},
	}
	created, err := svc.Files.Create(f).Media(content).Fields("id, webViewLink").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("drive: upload caption file: %w", err)
	}
	return &UploadResult{FileID: created.Id, WebViewLink: created.WebViewLink}, nil
}
