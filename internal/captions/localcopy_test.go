package captions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLocalCopyNextToVideo(t *testing.T) {
	videoDir, work := t.TempDir(), t.TempDir()
	video := filepath.Join(videoDir, "Sermon.mp4")
	srt := filepath.Join(work, "transcript.srt")
	writeFile(t, srt, "1\n00:00:00,000 --> 00:00:01,000\nHello.\n\n")

	got, err := SaveLocalCopy(video, srt, t.TempDir())
	if err != nil {
		t.Fatalf("SaveLocalCopy: %v", err)
	}
	if want := filepath.Join(videoDir, "Sermon.srt"); got != want {
		t.Fatalf("saved to %s, want %s", got, want)
	}
	b, _ := os.ReadFile(got)
	if string(b) != "1\n00:00:00,000 --> 00:00:01,000\nHello.\n\n" {
		t.Fatalf("local copy content = %q", b)
	}
}

func TestSaveLocalCopyNeverOverwrites(t *testing.T) {
	videoDir := t.TempDir()
	video := filepath.Join(videoDir, "Sermon.mp4")
	writeFile(t, filepath.Join(videoDir, "Sermon.srt"), "mine")
	writeFile(t, filepath.Join(videoDir, "Sermon (2).srt"), "also mine")
	srt := filepath.Join(t.TempDir(), "t.srt")
	writeFile(t, srt, "new")

	got, err := SaveLocalCopy(video, srt, t.TempDir())
	if err != nil {
		t.Fatalf("SaveLocalCopy: %v", err)
	}
	if want := filepath.Join(videoDir, "Sermon (3).srt"); got != want {
		t.Fatalf("saved to %s, want %s", got, want)
	}
	if b, _ := os.ReadFile(filepath.Join(videoDir, "Sermon.srt")); string(b) != "mine" {
		t.Fatal("an existing caption file was overwritten")
	}
}

func TestSaveLocalCopyFallsBackToDownloads(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits don't stop writes here")
	}
	videoDir := t.TempDir()
	if err := os.Chmod(videoDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(videoDir, 0o755) })
	downloads := t.TempDir()
	srt := filepath.Join(t.TempDir(), "t.srt")
	writeFile(t, srt, "x")

	got, err := SaveLocalCopy(filepath.Join(videoDir, "Talk.mov"), srt, downloads)
	if err != nil {
		t.Fatalf("SaveLocalCopy: %v", err)
	}
	if want := filepath.Join(downloads, "Talk.srt"); got != want {
		t.Fatalf("saved to %s, want the Downloads fallback %s", got, want)
	}
}
