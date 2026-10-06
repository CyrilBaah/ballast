package captions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAudioReportsNoAudio(t *testing.T) {
	if _, err := os.Stat(afconvertPath); err != nil {
		t.Skip("afconvert is macOS-only")
	}
	// afconvert fails the same way on a video with no audio track as on a
	// file with no audio at all (measured, research.md §3).
	notAudio := filepath.Join(t.TempDir(), "no-audio.mp4")
	if err := os.WriteFile(notAudio, []byte("this has no audio track"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ExtractAudio(context.Background(), notAudio, filepath.Join(t.TempDir(), "out.wav"))
	if !errors.Is(err, ErrNoAudio) {
		t.Fatalf("ExtractAudio err = %v, want ErrNoAudio", err)
	}
}

func TestExtractAudioMissingConverter(t *testing.T) {
	orig := afconvertPath
	afconvertPath = filepath.Join(t.TempDir(), "missing-afconvert")
	t.Cleanup(func() { afconvertPath = orig })
	if err := ExtractAudio(context.Background(), "in.mp4", "out.wav"); err == nil || errors.Is(err, ErrNoAudio) {
		t.Fatalf("ExtractAudio with no converter err = %v, want a start error", err)
	}
}
