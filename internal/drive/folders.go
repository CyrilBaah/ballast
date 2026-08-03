// Package drive wraps the Google Drive API v3 client for the two
// operations Ballast needs: browsing existing folders and performing a
// basic, non-resumable file upload.
package drive

import (
	"context"
	"fmt"
	"strings"

	drivev3 "google.golang.org/api/drive/v3"
)

const folderMimeType = "application/vnd.google-apps.folder"

// Folder is a Drive folder as surfaced to the frontend.
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// HasChildren is always true rather than computed from an extra
	// per-folder API call, so the UI can show an expand affordance without
	// an extra round trip; worst case is an empty folder view on click.
	HasChildren bool `json:"hasChildren"`
}

// buildFolderQuery builds the Drive Files.List `q` filter: folders only,
// not trashed, scoped to parentID (defaulting to "root" when empty).
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

	folders := []Folder{}
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
