package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestInitiateSessionTagsFileWithUploadID checks that every session's
// metadata carries the upload's ID as an app property, which is what lets
// FindLandedUpload recognise the finished file later.
func TestInitiateSessionTagsFileWithUploadID(t *testing.T) {
	var meta struct {
		AppProperties map[string]string `json:"appProperties"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &meta); err != nil {
			t.Errorf("decode initiate body: %v", err)
		}
		w.Header().Set("Location", "http://"+r.Host+"/session")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, derr, terr := InitiateSession(context.Background(), srv.Client(), srv.URL, 266, "sermon.mp4", "folder-1", 10); derr != nil || terr != nil {
		t.Fatalf("InitiateSession: derr=%v terr=%v", derr, terr)
	}
	if got := meta.AppProperties[UploadIDAppProperty]; got != "266" {
		t.Fatalf("appProperties[%s] = %q, want %q", UploadIDAppProperty, got, "266")
	}
}

func TestFindLandedUpload(t *testing.T) {
	var gotQuery string
	files := `{"files":[
		{"id":"wrong-size","name":"sermon.mp4","size":"9","webViewLink":"https://drive/wrong-size"},
		{"id":"wrong-name","name":"other.mp4","size":"10","webViewLink":"https://drive/wrong-name"},
		{"id":"match","name":"sermon.mp4","size":"10","webViewLink":"https://drive/match"}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/files" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, files)
	}))
	defer srv.Close()

	res, err := FindLandedUpload(context.Background(), srv.Client(), srv.URL, 266, "/Users/me/sermon.mp4", "folder-1", 10)
	if err != nil {
		t.Fatalf("FindLandedUpload: %v", err)
	}
	if res == nil || res.FileID != "match" || res.WebViewLink != "https://drive/match" {
		t.Fatalf("FindLandedUpload = %+v, want the file with matching name and size", res)
	}
	for _, want := range []string{"key='ballastUploadId' and value='266'", "'folder-1' in parents", "trashed = false"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}

	files = `{"files":[]}`
	res, err = FindLandedUpload(context.Background(), srv.Client(), srv.URL, 266, "sermon.mp4", "folder-1", 10)
	if err != nil || res != nil {
		t.Fatalf("FindLandedUpload with no match = %+v, %v; want nil, nil", res, err)
	}
}

func TestFindLandedUploadReportsDriveErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDriveError(w, http.StatusInternalServerError, "backendError", "", "try again")
	}))
	defer srv.Close()

	if _, err := FindLandedUpload(context.Background(), srv.Client(), srv.URL, 1, "a.bin", "folder-1", 1); err == nil {
		t.Fatal("FindLandedUpload: want an error for a 500 response")
	}
}

func TestEscapeQueryValue(t *testing.T) {
	if got, want := escapeQueryValue(`it's\here`), `it\'s\\here`; got != want {
		t.Fatalf("escapeQueryValue = %q, want %q", got, want)
	}
}
