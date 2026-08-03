package drive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, content []byte, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestCheapIdentityCheckMatchesUnchangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeFile(t, path, []byte("hello world"), mtime)

	ok, err := CheapIdentityCheck(path, IdentityBaseline{Size: 11, Mtime: mtime})
	if err != nil {
		t.Fatalf("CheapIdentityCheck: %v", err)
	}
	if !ok {
		t.Fatal("expected the cheap check to pass for an untouched file")
	}
}

// TestCheapIdentityCheckFailsOnMtimeTouchWithoutContentChange covers the
// false-positive case: a tool touches the file's mtime without altering
// its bytes (quickstart.md Scenario 3, case 7) -- the cheap check alone
// must report a mismatch here; VerifyPrefix is what rescues it.
func TestCheapIdentityCheckFailsOnMtimeTouchWithoutContentChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	original := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeFile(t, path, []byte("hello world"), original)

	touched := original.Add(time.Minute)
	if err := os.Chtimes(path, touched, touched); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	ok, err := CheapIdentityCheck(path, IdentityBaseline{Size: 11, Mtime: original})
	if err != nil {
		t.Fatalf("CheapIdentityCheck: %v", err)
	}
	if ok {
		t.Fatal("expected the cheap check to detect the mtime mismatch")
	}
}

func TestVerifyPrefixPassesWhenAcknowledgedBytesAreIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	content := []byte("the quick brown fox jumps over the lazy dog")
	writeFile(t, path, content, time.Now())

	acknowledged := int64(20)
	checksum := NewChecksum()
	checksum.Write(content[:acknowledged])
	state, err := MarshalChecksum(checksum)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	ok, err := VerifyPrefix(path, acknowledged, state)
	if err != nil {
		t.Fatalf("VerifyPrefix: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyPrefix to pass when the acknowledged prefix is unchanged")
	}
}

// TestVerifyPrefixFailsWhenAcknowledgedContentChanged covers the real
// file-changed case (quickstart.md Scenario 3, case 5): a mismatch in the
// already-acknowledged prefix must be reported, not silently accepted.
func TestVerifyPrefixFailsWhenAcknowledgedContentChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	original := []byte("the quick brown fox jumps over the lazy dog")
	writeFile(t, path, original, time.Now())

	acknowledged := int64(20)
	checksum := NewChecksum()
	checksum.Write(original[:acknowledged])
	state, err := MarshalChecksum(checksum)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	// Rewrite the file with different content of the same total length.
	changed := []byte("A DIFFERENT quick brown fox jumps over dog!")
	if len(changed) != len(original) {
		t.Fatalf("test setup: changed length %d != original length %d", len(changed), len(original))
	}
	writeFile(t, path, changed, time.Now())

	ok, err := VerifyPrefix(path, acknowledged, state)
	if err != nil {
		t.Fatalf("VerifyPrefix: %v", err)
	}
	if ok {
		t.Fatal("expected VerifyPrefix to detect changed content within the acknowledged prefix")
	}
}

func TestVerifyPrefixFailsWhenFileNowShorterThanAcknowledged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	content := []byte("0123456789")
	writeFile(t, path, content, time.Now())

	checksum := NewChecksum()
	checksum.Write(content)
	state, err := MarshalChecksum(checksum)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	// Truncate the file below the previously-acknowledged length.
	writeFile(t, path, content[:5], time.Now())

	ok, err := VerifyPrefix(path, int64(len(content)), state)
	if err != nil {
		t.Fatalf("VerifyPrefix: %v", err)
	}
	if ok {
		t.Fatal("expected VerifyPrefix to fail when the file is now shorter than the acknowledged prefix")
	}
}

func TestVerifyPrefixNeverReadsPastAcknowledgedLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	content := []byte("0123456789----extra-unacknowledged-bytes-that-must-not-be-read")
	writeFile(t, path, content, time.Now())

	acknowledged := int64(10)
	checksum := NewChecksum()
	checksum.Write(content[:acknowledged])
	state, err := MarshalChecksum(checksum)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	ok, err := VerifyPrefix(path, acknowledged, state)
	if err != nil {
		t.Fatalf("VerifyPrefix: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyPrefix to pass -- it must only ever compare the acknowledged prefix, not the whole file")
	}
}

func TestVerifyPrefixTriviallyPassesWhenNothingAcknowledgedYet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	writeFile(t, path, []byte("anything"), time.Now())

	ok, err := VerifyPrefix(path, 0, nil)
	if err != nil {
		t.Fatalf("VerifyPrefix: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyPrefix to trivially pass when no chunk has ever been acknowledged")
	}
}

func TestMarshalUnmarshalChecksumRoundTrips(t *testing.T) {
	h := NewChecksum()
	h.Write([]byte("part one "))
	state, err := MarshalChecksum(h)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	restored, err := UnmarshalChecksum(state)
	if err != nil {
		t.Fatalf("UnmarshalChecksum: %v", err)
	}
	restored.Write([]byte("part two"))

	want := NewChecksum()
	want.Write([]byte("part one part two"))

	if string(restored.Sum(nil)) != string(want.Sum(nil)) {
		t.Fatal("restored checksum does not continue the same digest as writing it all at once")
	}
}
