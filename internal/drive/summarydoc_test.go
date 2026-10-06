package drive

import (
	"context"
	"strings"
	"testing"
)

func TestFindSummaryDoc(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	f.files = `{"files":[{"id":"doc-1","webViewLink":"https://docs/doc-1"}]}`
	got, err := FindSummaryDoc(context.Background(), svc, 7, "folder-1")
	if err != nil || got == nil || got.FileID != "doc-1" {
		t.Fatalf("FindSummaryDoc = %+v, %v", got, err)
	}
	if !strings.Contains(f.listed[0], "key='ballastSummaryFor' and value='7'") {
		t.Fatalf("query = %q", f.listed[0])
	}
}

func TestChooseSummaryName(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	if name, _ := ChooseSummaryName(context.Background(), svc, "folder-1", "Sermon"); name != "Sermon — Summary" {
		t.Fatalf("name = %q", name)
	}
	f.files = `{"files":[{"name":"Sermon — Summary"}]}`
	if name, _ := ChooseSummaryName(context.Background(), svc, "folder-1", "Sermon"); name != "Sermon — Summary (2)" {
		t.Fatalf("name = %q, want Sermon — Summary (2)", name)
	}
}

func TestCreateSummaryDoc(t *testing.T) {
	f, svc := newFakeDriveFiles(t)
	res, err := CreateSummaryDoc(context.Background(), svc, 7, "folder-1", "Sermon — Summary", strings.NewReader("<h1>Hi</h1>"))
	if err != nil || res.FileID != "new-id" {
		t.Fatalf("CreateSummaryDoc = %+v, %v", res, err)
	}
	if f.createdMD["mimeType"] != "application/vnd.google-apps.document" || f.createdMD["name"] != "Sermon — Summary" {
		t.Fatalf("metadata = %v", f.createdMD)
	}
	if props, _ := f.createdMD["appProperties"].(map[string]any); props["ballastSummaryFor"] != "7" {
		t.Fatalf("appProperties = %v", f.createdMD["appProperties"])
	}
	if !strings.Contains(f.createdBody, "<h1>Hi</h1>") {
		t.Fatalf("body = %q", f.createdBody)
	}
}
