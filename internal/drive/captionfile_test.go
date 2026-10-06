package drive

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type fakeDriveFiles struct {
	*httptest.Server
	listed      []string // q parameters seen
	files       string   // JSON body for list calls
	createdMD   map[string]any
	createdBody string
}

func newFakeDriveFiles(t *testing.T) (*fakeDriveFiles, *drivev3.Service) {
	t.Helper()
	f := &fakeDriveFiles{files: `{"files":[]}`}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files"):
			f.listed = append(f.listed, r.URL.Query().Get("q"))
			io.WriteString(w, f.files)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload/drive/v3/files"):
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				t.Errorf("create content type: %v", err)
			}
			mr := multipart.NewReader(r.Body, params["boundary"])
			meta, _ := mr.NextPart()
			json.NewDecoder(meta).Decode(&f.createdMD)
			media, _ := mr.NextPart()
			b, _ := io.ReadAll(media)
			f.createdBody = string(b)
			io.WriteString(w, `{"id":"new-id","webViewLink":"https://drive/new"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	svc, err := drivev3.NewService(context.Background(),
		option.WithEndpoint(f.URL+"/"), option.WithHTTPClient(f.Client()), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	return f, svc
}

func TestFindCaptionFile(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	got, err := FindCaptionFile(context.Background(), svc, 266, "folder-1")
	if err != nil || got != nil {
		t.Fatalf("FindCaptionFile with none = %+v, %v; want nil, nil", got, err)
	}
	if q := f.listed[0]; !strings.Contains(q, "key='ballastCaptionFor' and value='266'") || !strings.Contains(q, "'folder-1' in parents") || !strings.Contains(q, "trashed = false") {
		t.Fatalf("query = %q", q)
	}

	f.files = `{"files":[{"id":"cap-1","webViewLink":"https://drive/cap-1"}]}`
	got, err = FindCaptionFile(context.Background(), svc, 266, "folder-1")
	if err != nil || got == nil || got.FileID != "cap-1" || got.WebViewLink != "https://drive/cap-1" {
		t.Fatalf("FindCaptionFile = %+v, %v; want cap-1", got, err)
	}
}

func TestChooseCaptionName(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	name, err := ChooseCaptionName(context.Background(), svc, "folder-1", "Sermon", ".srt")
	if err != nil || name != "Sermon.srt" {
		t.Fatalf("ChooseCaptionName in an empty folder = %q, %v", name, err)
	}
	f.files = `{"files":[{"name":"Sermon.srt"},{"name":"Sermon (2).srt"},{"name":"Sermon.mp4"}]}`
	name, err = ChooseCaptionName(context.Background(), svc, "folder-1", "Sermon", ".srt")
	if err != nil || name != "Sermon (3).srt" {
		t.Fatalf("ChooseCaptionName = %q, %v; want Sermon (3).srt", name, err)
	}
}

func TestUploadCaptionFile(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	res, err := UploadCaptionFile(context.Background(), svc, 266, "folder-1", "Sermon.srt", strings.NewReader("1\n00:00:00,000 --> 00:00:01,000\nHi.\n\n"))
	if err != nil {
		t.Fatalf("UploadCaptionFile: %v", err)
	}
	if res.FileID != "new-id" || res.WebViewLink != "https://drive/new" {
		t.Fatalf("result = %+v", res)
	}
	md := f.createdMD
	if md["name"] != "Sermon.srt" || md["mimeType"] != "application/x-subrip" {
		t.Fatalf("metadata = %v", md)
	}
	if parents, _ := md["parents"].([]any); len(parents) != 1 || parents[0] != "folder-1" {
		t.Fatalf("parents = %v", md["parents"])
	}
	if props, _ := md["appProperties"].(map[string]any); props["ballastCaptionFor"] != "266" {
		t.Fatalf("appProperties = %v", md["appProperties"])
	}
	if !strings.Contains(f.createdBody, "Hi.") {
		t.Fatalf("uploaded body = %q", f.createdBody)
	}
}
