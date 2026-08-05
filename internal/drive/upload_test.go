package drive

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

func makeTestFile(t *testing.T, size int64) (string, []byte) {
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
	size := BaselineChunkSize + 1024
	path, data := makeTestFile(t, size)

	var pausedCount, resumedCount int
	var lastCheckpoint int64
	cb := UploadCallbacks{
		OnChunkAcked: func(bytesSent int64, sessionURI string, hashState []byte, chunkSize int64, consecutiveSuccesses int) {
			lastCheckpoint = bytesSent
		},
		OnPaused:  func() { pausedCount++ },
		OnResumed: func() { resumedCount++ },
	}

	// Fail the second chunk once (simulating a drop after the first chunk
	// lands), then let it through.
	srv.setOutcome("approve")
	go func() {
		for srv.bytesReceived() < BaselineChunkSize {
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
	if srv.totalWireBytes() > int64(size)+BaselineChunkSize {
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

	size := BaselineChunkSize + 512
	path, data := makeTestFile(t, size)

	// Manually drive the fake server's session forward by "size - 512"
	// bytes to emulate a checkpoint from a prior (now-dead) process, then
	// build the matching content-hash-state checkpoint the same way
	// UploadFile itself would have produced it.
	firstLeg := int64(BaselineChunkSize)
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
	cb := UploadCallbacks{OnChunkAcked: func(bytesSent int64, _ string, _ []byte, _ int64, _ int) { acked = append(acked, bytesSent) }}

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

	path, _ := makeTestFile(t, BaselineChunkSize)
	_, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", int64(BaselineChunkSize), statBaseline(t, path), ResumeState{}, UploadCallbacks{})
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

// TestUploadFileGrowsChunkSizeOnAllSuccessRun covers User Story 1
// (Acceptance Scenarios 1-2): on a failure-free run, the chunk size
// doubles every 3 consecutive acknowledged chunks up to the 64 MiB
// ceiling, then stays there for the rest of the transfer.
func TestUploadFileGrowsChunkSizeOnAllSuccessRun(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	const mib = int64(1024 * 1024)
	const finalPartial = 3 * mib
	rampUp := (3*8 + 3*16 + 3*32) * mib
	atCeiling := 2 * 64 * mib
	size := rampUp + atCeiling + finalPartial
	path, data := makeTestFile(t, size)

	srv.setOutcome("approve")
	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", size, statBaseline(t, path), ResumeState{}, UploadCallbacks{})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID == "" {
		t.Fatal("expected a completed upload result")
	}
	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("uploaded content is not byte-identical to the source file")
	}

	sizes := srv.acceptedChunkSizes()
	want := []int64{
		8 * mib, 8 * mib, 8 * mib,
		16 * mib, 16 * mib, 16 * mib,
		32 * mib, 32 * mib, 32 * mib,
		64 * mib, 64 * mib,
		finalPartial,
	}
	if len(sizes) != len(want) {
		t.Fatalf("accepted chunk sizes = %v, want %v", sizes, want)
	}
	for i, w := range want {
		if sizes[i] != w {
			t.Fatalf("chunk %d size = %d, want %d (full sequence: %v)", i, sizes[i], w, sizes)
		}
	}
	// FR-007: every chunk except the final (possibly smaller) one must be
	// a 256 KiB multiple, independent of the specific expected values above.
	for i, s := range sizes[:len(sizes)-1] {
		if s%(256*1024) != 0 {
			t.Fatalf("chunk %d size = %d is not a 256 KiB multiple", i, s)
		}
	}
}

// TestUploadFileShrinksThenRegrowsGradually covers User Story 2
// (Acceptance Scenarios 1 and 3): a retried chunk failure halves the size
// immediately, and growth afterward requires the same 3 consecutive
// successes as any other growth step -- not an immediate jump back to the
// size that just failed.
func TestUploadFileShrinksThenRegrowsGradually(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	const mib = int64(1024 * 1024)
	rampUp := (3*8 + 3*16) * mib     // 6 chunks: reaches 32 MiB after the 6th ack
	afterShrink := (3*16 + 32) * mib // shrink to 16, 3 successes regrow to 32
	size := rampUp + afterShrink
	path, data := makeTestFile(t, size)

	// Attempt index 6 (0-indexed, the 7th attempt) is the first one sent
	// at the just-grown 32 MiB size -- fail it exactly once, deterministically
	// by attempt count rather than racing a goroutine against retry timing.
	srv.failAttempts(6)
	srv.setOutcome("approve")

	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", size, statBaseline(t, path), ResumeState{}, UploadCallbacks{})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID == "" {
		t.Fatal("expected a completed upload result")
	}
	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("uploaded content is not byte-identical to the source file")
	}

	sizes := srv.acceptedChunkSizes()
	want := []int64{
		8 * mib, 8 * mib, 8 * mib,
		16 * mib, 16 * mib, 16 * mib,
		16 * mib, 16 * mib, 16 * mib, // shrunk from the failed 32 MiB attempt
		32 * mib, // regrown only after 3 more consecutive successes
	}
	if len(sizes) != len(want) {
		t.Fatalf("accepted chunk sizes = %v, want %v", sizes, want)
	}
	for i, w := range want {
		if sizes[i] != w {
			t.Fatalf("chunk %d size = %d, want %d (full sequence: %v)", i, sizes[i], w, sizes)
		}
	}
}

// TestUploadFileFloorsAtMinChunkSizeUnderSustainedFailures covers User
// Story 2 (Acceptance Scenario 2): repeated consecutive chunk failures
// keep shrinking the size until it reaches the floor, and it never drops
// below that floor no matter how many further attempts fail. Failing 5
// consecutive attempts (0-indexed 0-4) is more than the 3 halvings
// (8->4->2->1 MiB) needed to floor from baseline, deterministically.
func TestUploadFileFloorsAtMinChunkSizeUnderSustainedFailures(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	size := 3 * MinChunkSize // enough for several floor-sized chunks once floored
	path, data := makeTestFile(t, size)

	srv.failAttempts(0, 1, 2, 3, 4)
	srv.setOutcome("approve")

	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", size, statBaseline(t, path), ResumeState{}, UploadCallbacks{})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID == "" {
		t.Fatal("expected a completed upload result")
	}
	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("uploaded content is not byte-identical to the source file")
	}

	sizes := srv.acceptedChunkSizes()
	if len(sizes) == 0 {
		t.Fatal("expected at least one accepted chunk")
	}
	for i, s := range sizes {
		if s != MinChunkSize {
			t.Fatalf("chunk %d size = %d, want the floored size %d for every chunk in this all-failed-then-floored run (full sequence: %v)", i, s, MinChunkSize, sizes)
		}
	}
}

// TestClassifyAndMaybeRetryDoesNotShrinkOnTerminalFailure covers FR-012:
// a terminal (non-retried) failure must never trigger a chunk-size change,
// even though shrink and terminal classification share the same call site
// in classifyAndMaybeRetry.
func TestClassifyAndMaybeRetryDoesNotShrinkOnTerminalFailure(t *testing.T) {
	policy := NewChunkSizePolicy(32 * 1024 * 1024)
	paused := false
	backoff := NewBackoffPolicy()

	de := &DriveError{StatusCode: 403, Reason: "storageQuotaExceeded"}
	outcome, retry := classifyAndMaybeRetry(context.Background(), nil, de, false, UploadCallbacks{}, &paused, backoff, policy)
	if retry {
		t.Fatal("expected classifyAndMaybeRetry to stop retrying for a terminal-not-recoverable error")
	}
	if outcome == nil || outcome.Bucket != TerminalNotRecoverable {
		t.Fatalf("outcome = %v, want a TerminalNotRecoverable outcome", outcome)
	}
	if policy.Size != 32*1024*1024 {
		t.Fatalf("Size = %d, want unchanged %d after a terminal failure (FR-012)", policy.Size, 32*1024*1024)
	}
	if policy.ConsecutiveSuccesses != 0 {
		t.Fatalf("ConsecutiveSuccesses = %d, want unchanged 0", policy.ConsecutiveSuccesses)
	}
}

// TestUploadFileResumeHonorsRestoredChunkSizeAndShrinksFromIt covers User
// Story 3 (FR-009, Acceptance Scenario 3): a resumed upload's first chunk
// uses the size carried in ResumeState, not the baseline, and a failure
// right after resuming shrinks from that restored size, not from baseline.
func TestUploadFileResumeHonorsRestoredChunkSizeAndShrinksFromIt(t *testing.T) {
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	restoredSize := int64(32 * 1024 * 1024)
	size := restoredSize*2 + 1024*1024 // two chunks at the restored size, plus a small remainder
	path, data := makeTestFile(t, size)

	// BytesSent is intentionally left at 0 -- this test's sole concern is
	// ChunkSize restoration, not the byte-offset resume path already
	// covered by TestUploadFileResumesFromPersistedCheckpoint.
	resume := ResumeState{ChunkSize: restoredSize, ConsecutiveSuccesses: 1}

	// Attempt index 1 (0-indexed, the 2nd attempt) is the first one sent
	// after the restored size's initial success -- fail it exactly once.
	srv.failAttempts(1)
	srv.setOutcome("approve")

	result, err := UploadFile(context.Background(), srv.Client(), srv.URL, 1, path, "folder-1", size, statBaseline(t, path), resume, UploadCallbacks{})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID == "" {
		t.Fatal("expected a completed upload result")
	}
	got := srv.receivedContent()
	if string(got) != string(data) {
		t.Fatal("resumed upload's content is not byte-identical to the source file")
	}

	sizes := srv.acceptedChunkSizes()
	if len(sizes) == 0 || sizes[0] != restoredSize {
		t.Fatalf("chunk sizes = %v, want the first chunk at the restored size %d, not baseline %d", sizes, restoredSize, BaselineChunkSize)
	}
	if len(sizes) < 2 || sizes[1] != restoredSize/2 {
		t.Fatalf("chunk sizes = %v, want the 2nd chunk to shrink to %d (half of the restored size), not from baseline", sizes, restoredSize/2)
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

	size := BaselineChunkSize + 1024
	path, _ := makeTestFile(t, size)
	baseline := statBaseline(t, path)

	// Waited on via the deferred wg.Wait() below so the goroutine's
	// t.Errorf calls, if any, always land before the test itself
	// completes -- calling them afterward panics the whole test binary.
	var wg sync.WaitGroup
	wg.Add(1)
	srv.setOutcome("approve")
	go func() {
		defer wg.Done()
		for srv.bytesReceived() < BaselineChunkSize {
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
	defer wg.Wait()

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
	if runtime.GOOS == "windows" {
		// Windows opens files without FILE_SHARE_DELETE by default, so it
		// refuses to delete a file UploadFile still has open -- this exact
		// interleaving (external deletion while our own handle is open)
		// isn't reproducible there the way it is on POSIX, where unlinking
		// an open file is always allowed. The "local file missing" code
		// path itself is simple enough (a failing os.Stat) that this gap
		// doesn't leave it meaningfully untested on the platforms where the
		// scenario can actually occur.
		t.Skip("deleting a file that's still open elsewhere is not reproducible on Windows")
	}
	noopSleep(t)
	srv := newFakeResumableServer()
	defer srv.Close()

	size := BaselineChunkSize + 1024
	path, _ := makeTestFile(t, size)
	baseline := statBaseline(t, path)

	var wg sync.WaitGroup
	wg.Add(1)
	srv.setOutcome("approve")
	go func() {
		defer wg.Done()
		for srv.bytesReceived() < BaselineChunkSize {
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
	defer wg.Wait()

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
