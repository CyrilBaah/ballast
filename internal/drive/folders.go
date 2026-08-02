// Package drive wraps the Google Drive API v3 client for the two
// operations this feature needs: browsing existing folders (FR-005) and
// performing a basic, non-resumable file upload (FR-006).
package drive

import (
	"context"
	"fmt"
	"strings"

	drivev3 "google.golang.org/api/drive/v3"
)

const folderMimeType = "application/vnd.google-apps.folder"

// Folder is a Drive folder as surfaced to the frontend (contract:
// DriveFolder).
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// HasChildren is always true rather than computed from an extra
	// per-folder API call. Contracts/wails-bindings.md asks for this field
	// specifically so the UI can "show an expand affordance without an
	// extra round trip" -- an honest per-folder child count would require
	// exactly the extra round trip that requirement rules out. Reporting
	// true always costs nothing extra and only downside-cases into an
	// empty folder view on click, which is a fine, simple outcome
	// (Constitution Principle V: no speculative complexity for a cosmetic
	// affordance).
	HasChildren bool `json:"hasChildren"`
}

// buildFolderQuery builds the Drive Files.List `q` filter (research.md
// §2): folders only, not trashed, scoped to parentID -- defaulting to
// "root" ("My Drive") when parentID is empty, per Acceptance Scenario 2 of
// User Story 2.
func buildFolderQuery(parentID string) string {
	if parentID == "" {
		parentID = "root"
	}
	// Drive folder IDs don't contain single quotes in practice, but escape
	// defensively since this value is interpolated into the query string.
	safeParent := strings.ReplaceAll(parentID, "'", "\\'")
	return fmt.Sprintf("mimeType='%s' and trashed=false and '%s' in parents", folderMimeType, safeParent)
}

// ListFolders lists the child folders of parentID ("" / "root" means the
// Drive root, "My Drive"), paginating through all result pages.
func ListFolders(ctx context.Context, svc *drivev3.Service, parentID string) ([]Folder, error) {
	query := buildFolderQuery(parentID)

	var folders []Folder
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name)").
			PageSize(100).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("drive: list folders: %w", err)
		}
		for _, f := range resp.Files {
			folders = append(folders, Folder{ID: f.Id, Name: f.Name, HasChildren: true})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return folders, nil
}
