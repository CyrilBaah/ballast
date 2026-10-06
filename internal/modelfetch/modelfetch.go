// Package modelfetch downloads a large model file once and installs it
// only after it is proven intact, for both the speech model (Feature 005)
// and the summary model (Feature 006). A download resumes from its .part
// file with an HTTP Range request, and the file is renamed into place only
// once its size and SHA-256 match the values compiled into Ballast, so a
// half-downloaded or tampered model is never run.
package modelfetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Spec pins one model file: where to get it and exactly what it must be.
type Spec struct {
	URL      string
	Size     int64
	SHA256   string
	FileName string
}

var (
	// ErrChecksumMismatch means the downloaded bytes weren't the pinned file;
	// the partial download is discarded.
	ErrChecksumMismatch = errors.New("modelfetch: downloaded file does not match its pinned checksum")
	// ErrNotEnoughSpace means the disk can't hold the rest of the download.
	ErrNotEnoughSpace = errors.New("modelfetch: not enough free disk space")
)

// spaceMargin is kept free on top of the download itself.
const spaceMargin = 500 << 20

// freeSpace reports bytes available to the user at dir; swapped in tests.
var freeSpace = availableBytes

// Path is where spec's file lives once installed in dir.
func Path(dir string, spec Spec) string {
	return filepath.Join(dir, spec.FileName)
}

// Present reports whether spec's file is installed in dir. It checks the
// size only: hashing a multi-gigabyte file on every launch is too slow,
// and a file only reaches this name after passing the full check.
func Present(dir string, spec Spec) bool {
	info, err := os.Stat(Path(dir, spec))
	return err == nil && info.Size() == spec.Size
}

// Fetch downloads spec into dir, resuming any earlier .part file, verifies
// it, and renames it into place. progress, if non-nil, is called with the
// bytes held so far. Network errors are returned as-is for the caller to
// retry; the .part file is kept so the retry resumes.
func Fetch(ctx context.Context, client *http.Client, dir string, spec Spec, progress func(done, total int64)) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("modelfetch: create %s: %w", dir, err)
	}
	part := Path(dir, spec) + ".part"

	h := sha256.New()
	have, err := hashExisting(part, h)
	if err != nil {
		return err
	}
	if have > spec.Size {
		os.Remove(part)
		h.Reset()
		have = 0
	}

	free, err := freeSpace(dir)
	if err != nil {
		return fmt.Errorf("modelfetch: check free space: %w", err)
	}
	if need := uint64(spec.Size-have) + spaceMargin; free < need {
		return ErrNotEnoughSpace
	}

	if have < spec.Size {
		if have, err = download(ctx, client, spec, part, have, h, progress); err != nil {
			return err
		}
	}

	if have != spec.Size || hex.EncodeToString(h.Sum(nil)) != spec.SHA256 {
		os.Remove(part)
		return ErrChecksumMismatch
	}
	if err := os.Rename(part, Path(dir, spec)); err != nil {
		return fmt.Errorf("modelfetch: install %s: %w", spec.FileName, err)
	}
	return nil
}

// hashExisting feeds an existing .part file into h and returns its length.
func hashExisting(part string, h hash.Hash) (int64, error) {
	f, err := os.Open(part)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("modelfetch: open partial download: %w", err)
	}
	defer f.Close()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, fmt.Errorf("modelfetch: read partial download: %w", err)
	}
	return n, nil
}

// download appends the rest of the file to part, starting at have, and
// returns the new length.
func download(ctx context.Context, client *http.Client, spec Spec, part string, have int64, h hash.Hash, progress func(done, total int64)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return have, fmt.Errorf("modelfetch: build request: %w", err)
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := client.Do(req)
	if err != nil {
		return have, err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
	case resp.StatusCode == http.StatusOK:
		// The server ignored the Range request: start over.
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		h.Reset()
		have = 0
	default:
		return have, fmt.Errorf("modelfetch: download %s: HTTP %d", spec.FileName, resp.StatusCode)
	}

	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return have, fmt.Errorf("modelfetch: open partial download: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 1<<20)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return have, fmt.Errorf("modelfetch: write partial download: %w", err)
			}
			h.Write(buf[:n])
			have += int64(n)
			if progress != nil {
				progress(have, spec.Size)
			}
		}
		if rerr == io.EOF {
			return have, nil
		}
		if rerr != nil {
			return have, rerr
		}
	}
}
