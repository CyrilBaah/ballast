package captions

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SaveLocalCopy copies the finished caption file next to the original
// video, named after it (Sermon.mp4 → Sermon.srt), as soon as it's ready
// -- without waiting for the upload (FR-022). An existing file is never
// overwritten: the copy becomes "Sermon (2).srt" and so on. If the video's
// folder can't be written to, the copy goes to downloadsDir instead. It
// returns where the copy was saved.
func SaveLocalCopy(videoPath, srtPath, downloadsDir string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	saved, err := copyToFreeName(srtPath, filepath.Dir(videoPath), base, ".srt")
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, os.ErrPermission) && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return copyToFreeName(srtPath, downloadsDir, base, ".srt")
}

// FreeName returns the first of "base.ext", "base (2).ext", "base (3).ext"
// … for which taken reports false.
func FreeName(base, ext string, taken func(name string) bool) string {
	name := base + ext
	for n := 2; taken(name); n++ {
		name = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
	return name
}

// copyToFreeName copies src into dir under the first free name, creating
// the destination exclusively so a file that appears in the meantime is
// never overwritten.
func copyToFreeName(src, dir, base, ext string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("captions: open transcript: %w", err)
	}
	defer in.Close()

	for attempt := 0; attempt < 100; attempt++ {
		name := FreeName(base, ext, func(n string) bool {
			_, err := os.Lstat(filepath.Join(dir, n))
			return err == nil
		})
		dst := filepath.Join(dir, name)
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue // lost a race for that name; pick the next one
		}
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			os.Remove(dst)
			return "", fmt.Errorf("captions: write local copy: %w", err)
		}
		if err := out.Close(); err != nil {
			os.Remove(dst)
			return "", fmt.Errorf("captions: write local copy: %w", err)
		}
		return dst, nil
	}
	return "", fmt.Errorf("captions: no free name for %s%s in %s", base, ext, dir)
}
