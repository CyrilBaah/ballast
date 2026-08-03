package drive

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"fmt"
	"hash"
	"io"
	"os"
	"time"
)

// IdentityBaseline is the size/mtime snapshot captured at upload creation
// (data-model.md's local_size_bytes/local_mtime) -- the cheap half of the
// source-file-identity check (research.md §5, FR-009).
type IdentityBaseline struct {
	Size  int64
	Mtime time.Time
}

// CheapIdentityCheck compares path's current os.Stat size/mtime against
// baseline. A match means resume immediately, with no hashing needed.
func CheapIdentityCheck(path string, baseline IdentityBaseline) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Size() == baseline.Size && info.ModTime().Equal(baseline.Mtime), nil
}

// NewChecksum starts a fresh incremental SHA-256 digest, updated as each
// chunk is read and checkpointed (via MarshalChecksum) after Drive
// acknowledges it.
func NewChecksum() hash.Hash {
	return sha256.New()
}

// MarshalChecksum serializes a digest's internal state for persistence
// (crypto/sha256's Hash implements encoding.BinaryMarshaler).
func MarshalChecksum(h hash.Hash) ([]byte, error) {
	m, ok := h.(encoding.BinaryMarshaler)
	if !ok {
		return nil, fmt.Errorf("drive: checksum type does not support binary marshaling")
	}
	return m.MarshalBinary()
}

// UnmarshalChecksum restores a previously-checkpointed digest state.
func UnmarshalChecksum(state []byte) (hash.Hash, error) {
	h := sha256.New()
	u, ok := h.(encoding.BinaryUnmarshaler)
	if !ok {
		return nil, fmt.Errorf("drive: checksum type does not support binary unmarshaling")
	}
	if err := u.UnmarshalBinary(state); err != nil {
		return nil, fmt.Errorf("drive: restore checksum state: %w", err)
	}
	return h, nil
}

// VerifyPrefix re-reads path's first prefixLen bytes and hashes them,
// reporting whether the result matches checkpointedState -- the bounded
// fallback used only when the cheap check fails (research.md §5). It never
// reads past prefixLen, regardless of the file's current total size, and
// never hashes the whole file.
func VerifyPrefix(path string, prefixLen int64, checkpointedState []byte) (bool, error) {
	if len(checkpointedState) == 0 {
		// No chunk has ever been acknowledged yet -- an empty prefix
		// trivially matches, since there is nothing yet to have changed.
		return prefixLen == 0, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() < prefixLen {
		return false, nil // file is now shorter than what was already acknowledged
	}

	h := sha256.New()
	if _, err := io.CopyN(h, f, prefixLen); err != nil {
		return false, err
	}
	current, err := MarshalChecksum(h)
	if err != nil {
		return false, err
	}
	return bytes.Equal(current, checkpointedState), nil
}
