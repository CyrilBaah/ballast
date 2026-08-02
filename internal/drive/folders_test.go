package drive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestBuildFolderQueryDefaultsToRoot(t *testing.T) {
	q := buildFolderQuery("")
	if !strings.Contains(q, "'root' in parents") {
		t.Fatalf("query %q does not scope to root when parentID is empty", q)
	}
}

func TestBuildFolderQueryScopesToParent(t *testing.T) {
	q := buildFolderQuery("some-folder-id")
	if !strings.Contains(q, "'some-folder-id' in parents") {
		t.Fatalf("query %q does not scope to the given parent", q)
	}
}

func TestBuildFolderQueryFiltersToFoldersAndExcludesTrashed(t *testing.T) {
	q := buildFolderQuery("root")
	if !strings.Contains(q, "mimeType='application/vnd.google-apps.folder'") {
		t.Fatalf("query %q missing folder mimeType filter", q)
	}
	if !strings.Contains(q, "trashed=false") {
		t.Fatalf("query %q missing trashed=false filter", q)
	}
}

func TestListFoldersCallsDriveAPIAndParsesResults(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]string{
				{"id": "folder-1", "name": "Reports"},
				{"id": "folder-2", "name": "Photos"},
			},
		})
	}))
	defer srv.Close()

	svc, err := drivev3.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("drivev3.NewService: %v", err)
	}

	folders, err := ListFolders(context.Background(), svc, "")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2", len(folders))
	}
	if folders[0].ID != "folder-1" || folders[0].Name != "Reports" {
		t.Errorf("folders[0] = %+v", folders[0])
	}
	if folders[1].ID != "folder-2" || folders[1].Name != "Photos" {
		t.Errorf("folders[1] = %+v", folders[1])
	}

	decodedQuery, err := url.QueryUnescape(gotQuery)
	if err != nil {
		t.Fatalf("unescape query: %v", err)
	}
	if !strings.Contains(decodedQuery, "'root' in parents") {
		t.Errorf("request query %q did not scope to root", decodedQuery)
	}
}

func TestListFoldersPaginates(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files":         []map[string]string{{"id": "folder-1", "name": "Page1"}},
				"nextPageToken": "page-2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]string{{"id": "folder-2", "name": "Page2"}},
		})
	}))
	defer srv.Close()

	svc, err := drivev3.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("drivev3.NewService: %v", err)
	}

	folders, err := ListFolders(context.Background(), svc, "root")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("got %d folders across pages, want 2", len(folders))
	}
	if callCount != 2 {
		t.Fatalf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
}
