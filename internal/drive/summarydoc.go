package drive

import (
	"context"
	"fmt"
	"io"
	"strconv"

	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// SummaryForAppProperty tags a summary doc with its upload, so a crash
// between creating and recording it never leads to a second copy
// (Feature 006 research.md §11).
const SummaryForAppProperty = "ballastSummaryFor"

const googleDocMimeType = "application/vnd.google-apps.document"

// FindSummaryDoc returns the non-trashed doc in folderID already tagged as
// uploadID's summary, or nil, nil.
func FindSummaryDoc(ctx context.Context, svc *drivev3.Service, uploadID int64, folderID string) (*UploadResult, error) {
	q := fmt.Sprintf("appProperties has { key='%s' and value='%d' } and '%s' in parents and trashed = false",
		SummaryForAppProperty, uploadID, escapeQueryValue(folderID))
	resp, err := svc.Files.List().Q(q).Fields("files(id, webViewLink)").PageSize(10).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("drive: look for an existing summary: %w", err)
	}
	for _, f := range resp.Files {
		if f.Id != "" {
			return &UploadResult{FileID: f.Id, WebViewLink: f.WebViewLink}, nil
		}
	}
	return nil, nil
}

// ChooseSummaryName returns "base — Summary", or the first free
// "base — Summary (N)" in folderID (FR-006).
func ChooseSummaryName(ctx context.Context, svc *drivev3.Service, folderID, base string) (string, error) {
	return ChooseCaptionName(ctx, svc, folderID, base+" — Summary", "")
}

// CreateSummaryDoc uploads html into folderID as a Google Doc named name,
// tagged with uploadID; Drive converts the HTML into the doc.
func CreateSummaryDoc(ctx context.Context, svc *drivev3.Service, uploadID int64, folderID, name string, html io.Reader) (*UploadResult, error) {
	f := &drivev3.File{
		Name:          name,
		Parents:       []string{folderID},
		MimeType:      googleDocMimeType,
		AppProperties: map[string]string{SummaryForAppProperty: strconv.FormatInt(uploadID, 10)},
	}
	created, err := svc.Files.Create(f).Media(html, googleapi.ContentType("text/html")).Fields("id, webViewLink").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("drive: create summary doc: %w", err)
	}
	return &UploadResult{FileID: created.Id, WebViewLink: created.WebViewLink}, nil
}
