package modelfetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeModelServer struct {
	*httptest.Server
	mu     sync.Mutex
	ranges []string
}

func newFakeModelServer(t *testing.T, content []byte) *fakeModelServer {
	s := &fakeModelServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.ranges = append(s.ranges, r.Header.Get("Range"))
		s.mu.Unlock()
		http.ServeContent(w, r, "model.bin", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(s.Close)
	return s
}

func specFor(url string, content []byte) Spec {
	sum := sha256.Sum256(content)
	return Spec{URL: url, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), FileName: "model.bin"}
}

func modelBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func TestFetchDownloadsVerifiesAndRenames(t *testing.T) {
	content := modelBytes(300_000)
	srv := newFakeModelServer(t, content)
	dir := t.TempDir()
	spec := specFor(srv.URL, content)

	var last int64
	if err := Fetch(context.Background(), srv.Client(), dir, spec, func(done, total int64) { last = done }); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !Present(dir, spec) {
		t.Fatal("Present() = false after a successful Fetch")
	}
	if _, err := os.Stat(Path(dir, spec) + ".part"); !os.IsNotExist(err) {
		t.Error("the .part file was left behind")
	}
	if last != spec.Size {
		t.Errorf("last progress = %d, want %d", last, spec.Size)
	}
}

func TestFetchResumesFromPartFile(t *testing.T) {
	content := modelBytes(300_000)
	srv := newFakeModelServer(t, content)
	dir := t.TempDir()
	spec := specFor(srv.URL, content)

	if err := os.WriteFile(Path(dir, spec)+".part", content[:120_000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Fetch(context.Background(), srv.Client(), dir, spec, nil); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(srv.ranges) != 1 || srv.ranges[0] != "bytes=120000-" {
		t.Fatalf("Range headers = %q, want one request for bytes=120000-", srv.ranges)
	}
	got, _ := os.ReadFile(Path(dir, spec))
	if !bytes.Equal(got, content) {
		t.Fatal("resumed file differs from the original")
	}
}

func TestFetchRejectsWrongChecksum(t *testing.T) {
	content := modelBytes(50_000)
	srv := newFakeModelServer(t, content)
	dir := t.TempDir()
	spec := specFor(srv.URL, content)
	spec.SHA256 = "00" + spec.SHA256[2:]

	err := Fetch(context.Background(), srv.Client(), dir, spec, nil)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Fetch err = %v, want ErrChecksumMismatch", err)
	}
	if Present(dir, spec) {
		t.Fatal("a file failing its checksum was installed")
	}
	if _, err := os.Stat(Path(dir, spec) + ".part"); !os.IsNotExist(err) {
		t.Fatal("a .part failing its checksum was kept for resuming")
	}
}

func TestPresentChecksSize(t *testing.T) {
	dir := t.TempDir()
	spec := Spec{FileName: "m.bin", Size: 10}
	if Present(dir, spec) {
		t.Fatal("Present() with no file = true")
	}
	os.WriteFile(filepath.Join(dir, "m.bin"), []byte("short"), 0o644)
	if Present(dir, spec) {
		t.Fatal("Present() with a truncated file = true")
	}
	os.WriteFile(filepath.Join(dir, "m.bin"), []byte("0123456789"), 0o644)
	if !Present(dir, spec) {
		t.Fatal("Present() with the right size = false")
	}
}

func TestFetchChecksFreeSpaceFirst(t *testing.T) {
	content := modelBytes(10_000)
	srv := newFakeModelServer(t, content)
	spec := specFor(srv.URL, content)

	orig := freeSpace
	freeSpace = func(string) (uint64, error) { return 100, nil }
	t.Cleanup(func() { freeSpace = orig })

	err := Fetch(context.Background(), srv.Client(), t.TempDir(), spec, nil)
	if !errors.Is(err, ErrNotEnoughSpace) {
		t.Fatalf("Fetch err = %v, want ErrNotEnoughSpace", err)
	}
	if len(srv.ranges) != 0 {
		t.Fatal("Fetch started downloading without enough space")
	}
}
