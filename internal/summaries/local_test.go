package summaries

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeSummary = `{"overview":"An overview.","main_points":[{"title":"One","detail":"First point."}],"actions":[],"references":[],"quotes":[],"title":"A title","description":"A description.","share_message":"*A title*\nFirst point."}`
const fakeNotes = `{"points":[{"title":"Part point","detail":"x"}],"actions":[],"references":[],"quotes":[]}`

func startFake(t *testing.T, mode string) (*Server, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "requests.log")
	t.Setenv("FAKELLAMA_MODE", mode)
	t.Setenv("FAKELLAMA_SUMMARY", fakeSummary)
	t.Setenv("FAKELLAMA_NOTES", fakeNotes)
	t.Setenv("FAKELLAMA_LOG", log)
	s, err := StartServer(context.Background(), fakeLlamaBin, "model.gguf")
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	t.Cleanup(s.Stop)
	return s, log
}

func requests(t *testing.T, log string) []map[string]any {
	t.Helper()
	b, _ := os.ReadFile(log)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		json.Unmarshal([]byte(line), &m)
		out = append(out, m)
	}
	return out
}

func TestLocalSingleSendsSchemaAndParses(t *testing.T) {
	s, log := startFake(t, "ok")
	sum, err := Local{Server: s}.Single(context.Background(), "[00:00] Hello.\n")
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	if sum.Title != "A title" || len(sum.MainPoints) != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	reqs := requests(t, log)
	if len(reqs) != 1 {
		t.Fatalf("%d requests, want 1", len(reqs))
	}
	rf, _ := reqs[0]["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format = %v", rf)
	}
	js, _ := rf["json_schema"].(map[string]any)
	schema, _ := js["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["share_message"]; !ok {
		t.Fatal("the request didn't carry the summary schema")
	}
}

func TestLocalNotesAndCombine(t *testing.T) {
	s, log := startFake(t, "ok")
	l := Local{Server: s}
	n, err := l.Notes(context.Background(), "[20:00] Part two.", 1, 3)
	if err != nil || len(n.Points) != 1 {
		t.Fatalf("Notes = %+v, %v", n, err)
	}
	sum, err := l.Combine(context.Background(), []PartNotes{*n, *n}, "40 minutes")
	if err != nil || sum.Title != "A title" {
		t.Fatalf("Combine = %+v, %v", sum, err)
	}
	if len(requests(t, log)) != 2 {
		t.Fatal("want one notes request and one combine request")
	}
}

func TestLocalBadJSONIsUnusable(t *testing.T) {
	s, _ := startFake(t, "badjson")
	if _, err := (Local{Server: s}).Single(context.Background(), "x"); !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
}

func TestLocalEmptyAnswerIsUnusable(t *testing.T) {
	// A child process keeps the environment it started with, so set the
	// reply before starting the fake server.
	t.Setenv("FAKELLAMA_MODE", "ok")
	t.Setenv("FAKELLAMA_SUMMARY", `{"overview":"","main_points":[],"actions":[],"references":[],"quotes":[],"title":"","description":"","share_message":""}`)
	s, err := StartServer(context.Background(), fakeLlamaBin, "m")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if _, err := (Local{Server: s}).Single(context.Background(), "x"); !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable for an empty summary", err)
	}
}

func TestLocalTimeoutIsUnusable(t *testing.T) {
	s, _ := startFake(t, "hang")
	orig := RequestTimeout
	RequestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { RequestTimeout = orig })
	if _, err := (Local{Server: s}).Single(context.Background(), "x"); !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable after the timeout", err)
	}
}

func TestStartServerFailures(t *testing.T) {
	t.Setenv("FAKELLAMA_MODE", "oom")
	if _, err := StartServer(context.Background(), fakeLlamaBin, "m"); !errors.Is(err, ErrOutOfMemory) {
		t.Fatalf("oom err = %v", err)
	}
	t.Setenv("FAKELLAMA_MODE", "loadfail")
	if _, err := StartServer(context.Background(), fakeLlamaBin, "m"); !errors.Is(err, ErrLoadFailed) {
		t.Fatalf("loadfail err = %v", err)
	}
}

func TestStopKillsServer(t *testing.T) {
	s, _ := startFake(t, "ok")
	s.Stop()
	if !s.Exited() {
		t.Fatal("server still running after Stop")
	}
}
