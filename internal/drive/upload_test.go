package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// noopSleep replaces the real backoff delay with an instant, ctx-aware
// no-op so retry-heavy tests run fast and deterministically.
func noopSleep(t *testing.T) {
	t.Helper()
	orig := sleepForRetry
	sleepForRetry = func(ctx context.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t.Cleanup(func() { sleepForRetry = orig })
}

func makeTestFile(t *testing.T, size int) (string, []byte) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "upload-me.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path, data
}

func statBaseline(t *testing.T, path string) IdentityBaseline {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return IdentityBaseline{Size: info.Size(), Mtime: info.ModTime()}
}

// TestUploadFileResumesAfterNetworkDropWithoutRetransmission covers User
// Story 1 (Acceptance Scenarios 1-3): a mid-transfer connection drop
// pauses (not fails) the upload, and once connectivity returns it resumes
// from the last acknowledged offset -- resending nothing already acknowledged.
func TestUploadFileResumesAfterNetworkDropWithoutRetransmission(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	// Two chunks' worth of data, forcing at least one mid-transfer chunk boundary.
	size := ChunkSize + 1024
	path, data := makeTestFile(t, size)

	var pausedCount, resumedCount int
	var lastCheckpoint int64
	cb := UploadCallbacks{
		OnChunkAcked: func(bytesSent int64, sessionURI string, hashState []byte) {
			lastCheckpoint = bytesSent
		},
		OnPaused:  func() { pausedCount++ },
		OnResumed: func() { resumedCount++ },
	}

	// Fail the second chunk once (simulating a drop after the first chunk
	// lands), then let it through.
	srv.setOutcome("approve")
	go func() {
		for srv.bytesReceived() < ChunkSize {
			time.Sleep(time.Millisecond)
		}
		srv.setOutcome("network-fail")
		time.Sleep(20 * time.Millisecond)
		srv.setOutcome("approve")
	}()

	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(size), statBaseline(t, path), ResumeState{}, cb)
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID != "fake-file-id" {
		t.Fatalf("FileID = %q, want fake-file-id", result.FileID)
	}
	if lastCheckpoint != int64(size) {
		t.Fatalf("last checkpoint = %d, want %d", lastCheckpoint, size)
	}
	if pausedCount == 0 {
		t.Fatal("expected OnPaused to fire at least once for the dropped connection")
	}
	if resumedCount != pausedCount {
		t.Fatalf("resumedCount = %d, want to match pausedCount = %d", resumedCount, pausedCount)
	}

	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("uploaded content is not byte-identical to the source file")
	}
	if srv.totalWireBytes() > int64(size)+ChunkSize {
		t.Fatalf("wire bytes = %d suggest re-transmission of already-acknowledged data (size=%d)", srv.totalWireBytes(), size)
	}
}

// TestUploadFileResumesFromPersistedCheckpoint covers User Story 2: a
// process restart provides UploadFile a non-empty ResumeState (recovered
// from SQLite), and it must continue from that checkpoint, not from 0.
func TestUploadFileResumesFromPersistedCheckpoint(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	size := ChunkSize + 512
	path, data := makeTestFile(t, size)

	// Manually drive the fake server's session forward by "size - 512"
	// bytes to emulate a checkpoint from a prior (now-dead) process, then
	// build the matching content-hash-state checkpoint the same way
	// UploadFile itself would have produced it.
	firstLeg := int64(ChunkSize)
	uri, derr, terr := InitiateSession(context.Background(), srv.Client(), srv.URL, "upload-me.bin", "folder-1", int64(size))
	if terr != nil || derr != nil {
		t.Fatalf("InitiateSession: terr=%v derr=%v", terr, derr)
	}
	if _, derr, terr := SendChunk(context.Background(), srv.Client(), uri, data[:firstLeg], 0, int64(size)); terr != nil || derr != nil {
		t.Fatalf("priming SendChunk: terr=%v derr=%v", terr, derr)
	}
	checksum := NewChecksum()
	checksum.Write(data[:firstLeg])
	hashState, err := MarshalChecksum(checksum)
	if err != nil {
		t.Fatalf("MarshalChecksum: %v", err)
	}

	resume := ResumeState{SessionURI: uri, BytesSent: firstLeg, ContentHashState: hashState}

	var acked []int64
	cb := UploadCallbacks{OnChunkAcked: func(bytesSent int64, _ string, _ []byte) { acked = append(acked, bytesSent) }}

	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(size), statBaseline(t, path), resume, cb)
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID == "" {
		t.Fatal("expected a completed upload result")
	}
	if len(acked) == 0 || acked[0] <= firstLeg {
		t.Fatalf("acked checkpoints = %v; expected progress to continue past the resumed offset %d, not restart from 0", acked, firstLeg)
	}

	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("resumed upload's final content is not byte-identical to the source file")
	}
	// Only the unsent remainder (size - firstLeg) should have crossed the
	// wire during the resumed UploadFile call (the priming send above
	// already accounted for `firstLeg` bytes separately).
	if srv.totalWireBytes() != int64(size) {
		t.Fatalf("total wire bytes = %d, want exactly %d (no re-transmission of the primed prefix)", srv.totalWireBytes(), size)
	}
}

func TestUploadFileStopsRetryingOnQuotaExceeded(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()
	srv.setOutcome("403-quota")

	path, _ := makeTestFile(t, ChunkSize)
	_, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(ChunkSize), statBaseline(t, path), ResumeState{}, UploadCallbacks{})
	if err == nil {
		t.Fatal("expected UploadFile to stop with an error on storageQuotaExceeded")
	}
	var outcome *TerminalOutcome
	if !asOutcome(err, &outcome) {
		t.Fatalf("error = %v, want a *TerminalOutcome", err)
	}
	if outcome.Bucket != TerminalNotRecoverable {
		t.Fatalf("bucket = %v, want TerminalNotRecoverable", outcome.Bucket)
	}
}

func asOutcome(err error, target **TerminalOutcome) bool {
	o, ok := err.(*TerminalOutcome)
	if !ok {
		return false
	}
	*target = o
	return true
}

// TestUploadFileDetectsFileChangedDuringPause covers quickstart.md
// Scenario 3 case 5: the source file changes while a chunk-send retry is
// waiting on connectivity, so the resumed attempt must catch the mismatch
// before sending any more bytes against the now-stale acknowledged prefix,
// rather than silently uploading a corrupted mix of old and new content.
func TestUploadFileDetectsFileChangedDuringPause(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	size := ChunkSize + 1024
	path, _ := makeTestFile(t, size)
	baseline := statBaseline(t, path)

	srv.setOutcome("approve")
	go func() {
		for srv.bytesReceived() < ChunkSize {
			time.Sleep(time.Millisecond)
		}
		srv.setOutcome("network-fail")
		// Replace the file's content while the chunk retry is paused, and
		// force the mtime unambiguously forward so the cheap check can't
		// pass by coincidence on filesystems with coarse mtime resolution.
		time.Sleep(10 * time.Millisecond)
		if err := os.WriteFile(path, []byte("completely different content, same-ish length padding to reach original size!!"), 0o600); err != nil {
			t.Errorf("rewrite file during pause: %v", err)
		}
		newMtime := baseline.Mtime.Add(time.Hour)
		if err := os.Chtimes(path, newMtime, newMtime); err != nil {
			t.Errorf("chtimes during pause: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		srv.setOutcome("approve")
	}()

	_, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(size), baseline, ResumeState{}, UploadCallbacks{})
	if err == nil {
		t.Fatal("expected UploadFile to stop when the source file changes mid-pause")
	}
	var outcome *TerminalOutcome
	if !asOutcome(err, &outcome) {
		t.Fatalf("error = %v, want a *TerminalOutcome", err)
	}
	if outcome.Bucket != TerminalRecoverable {
		t.Fatalf("bucket = %v, want TerminalRecoverable", outcome.Bucket)
	}
	if outcome.Reason != ReasonFileChanged {
		t.Fatalf("reason = %q, want %q", outcome.Reason, ReasonFileChanged)
	}
}

// TestUploadFileFailsNotRecoverableWhenSourceFileDeletedDuringPause covers
// quickstart.md Scenario 3 case 6: a deleted source file has nothing to
// restart with, so it must reach failed (terminal, not recoverable) --
// never awaiting_confirmation, and never an indefinite hang.
func TestUploadFileFailsNotRecoverableWhenSourceFileDeletedDuringPause(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	size := ChunkSize + 1024
	path, _ := makeTestFile(t, size)
	baseline := statBaseline(t, path)

	srv.setOutcome("approve")
	go func() {
		for srv.bytesReceived() < ChunkSize {
			time.Sleep(time.Millisecond)
		}
		srv.setOutcome("network-fail")
		time.Sleep(10 * time.Millisecond)
		if err := os.Remove(path); err != nil {
			t.Errorf("remove file during pause: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		srv.setOutcome("approve")
	}()

	_, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(size), baseline, ResumeState{}, UploadCallbacks{})
	if err == nil {
		t.Fatal("expected UploadFile to stop when the source file is deleted mid-pause")
	}
	var outcome *TerminalOutcome
	if !asOutcome(err, &outcome) {
		t.Fatalf("error = %v, want a *TerminalOutcome", err)
	}
	if outcome.Bucket != TerminalNotRecoverable {
		t.Fatalf("bucket = %v, want TerminalNotRecoverable", outcome.Bucket)
	}
}
