package drive

import (
	"context"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UploadResult is what a successful upload reports back: a reference to
// the uploaded file's location in Drive.
type UploadResult struct {
	FileID      string
	WebViewLink string
}

// TerminalOutcome is returned by UploadFile when the transfer stops for a
// reason other than success: either TerminalNotRecoverable (stop
// retrying, tell the user a specific reason -- research.md §4) or
// TerminalRecoverable (stop retrying, but the user can explicitly restart
// from byte 0 once they confirm -- FR-010). UploadFile itself never
// touches storage; the caller applies whatever transition/event fits each case.
type TerminalOutcome struct {
	Bucket ErrorBucket
	Reason string
}

func (t *TerminalOutcome) Error() string { return t.Reason }

// ResumeState is what the caller already knows about a paused/in-progress
// upload when (re)starting the orchestrator. The zero value means "no
// session yet -- start fresh from byte 0." A zero ChunkSize means "no
// earned size to restore" and defaults to BaselineChunkSize (FR-002) --
// this covers both a brand-new upload and any restart that intentionally
// carries no chunk-size state forward.
type ResumeState struct {
	SessionURI           string
	BytesSent            int64
	ContentHashState     []byte
	ChunkSize            int64
	ConsecutiveSuccesses int
}

// UploadCallbacks are the side effects UploadFile triggers while still
// running -- persistence/events for state that changes mid-transfer, as
// opposed to the once-at-return terminal outcome the caller handles from
// UploadFile's return value.
type UploadCallbacks struct {
	// OnChunkAcked persists the latest Drive-acknowledged checkpoint,
	// atomically, after every acknowledged chunk (data-model.md) --
	// including the chunk-size state (current size, consecutive-success
	// streak) as of that same checkpoint.
	OnChunkAcked func(bytesSent int64, sessionURI string, hashState []byte, chunkSize int64, consecutiveSuccesses int)
	// OnPaused fires once when a retryable error is first hit -- not
	// repeated on every retry attempt within the same pause (FR-003, FR-007).
	OnPaused func()
	// OnResumed fires once a chunk send succeeds again after being paused.
	OnResumed func()
}

// sleepForRetry waits for d, or returns ctx.Err() early if ctx is
// cancelled first. Overridden in tests to avoid real delays.
var sleepForRetry = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UploadFile drives one upload's resumable session to completion:
// initiating a session if resume.SessionURI is empty, sending chunks
// strictly in order in ChunkSize multiples (Constitution Principle II),
// checkpointing after each acknowledgment, and retrying retryable errors
// with backoff indefinitely (FR-007) without ever re-sending an
// already-acknowledged byte. It returns once the upload reaches a
// terminal outcome (success or a *TerminalOutcome), or once ctx is
// cancelled (the app is shutting down -- the caller does nothing further,
// since the last checkpoint on disk is what a future launch resumes from).
//
// The source-file-identity check (research.md §5) runs whenever this call
// is about to (re)send a chunk on top of already-acknowledged bytes: once
// up front if resume.BytesSent > 0 (resuming a persisted upload -- after a
// restart, per research.md §7), and again before every chunk-send retry
// (quickstart.md Scenario 3, case 5 -- the file can change while a
// same-process pause is waiting on connectivity too). A fresh upload with
// nothing yet acknowledged has no baseline to check against, so the very
// first send of a brand-new upload skips it.
func UploadFile(ctx context.Context, client *http.Client, apiBase string, id int64, localPath, driveFolderID string, totalBytes int64, baseline IdentityBaseline, resume ResumeState, cb UploadCallbacks) (*UploadResult, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("local file can no longer be found: %v", err)}
	}
	defer f.Close()

	sessionURI := resume.SessionURI
	bytesSent := resume.BytesSent
	startChunkSize := resume.ChunkSize
	if startChunkSize == 0 {
		startChunkSize = BaselineChunkSize
	}
	policy := NewChunkSizePolicy(startChunkSize)
	policy.ConsecutiveSuccesses = resume.ConsecutiveSuccesses
	checksum := NewChecksum()
	if len(resume.ContentHashState) > 0 {
		checksum, err = UnmarshalChecksum(resume.ContentHashState)
		if err != nil {
			return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("internal error restoring upload checkpoint: %v", err)}
		}
	}

	if bytesSent > 0 {
		if outcome := checkIdentity(localPath, baseline, bytesSent, checksum); outcome != nil {
			return nil, outcome
		}
	}

	backoff := NewBackoffPolicy()
	paused := false

	if sessionURI == "" {
		for {
			uri, derr, terr := InitiateSession(ctx, client, apiBase, filepath.Base(localPath), driveFolderID, totalBytes)
			if terr == nil && derr == nil {
				sessionURI = uri
				break
			}
			outcome, retry := classifyAndMaybeRetry(ctx, terr, derr, true, cb, &paused, backoff, policy)
			if outcome != nil {
				return nil, outcome
			}
			if !retry {
				return nil, ctx.Err()
			}
		}
	}

	if paused {
		if cb.OnResumed != nil {
			cb.OnResumed()
		}
		paused = false
	}

	// Sized to the ceiling once per upload, not per chunk-size change --
	// reused via slicing so memory stays flat regardless of how large the
	// chunk size grows (SC-004, research.md §2).
	buf := make([]byte, MaxChunkSize)
	for bytesSent < totalBytes {
		if _, err := f.Seek(bytesSent, io.SeekStart); err != nil {
			return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("local file can no longer be found: %v", err)}
		}
		n, readErr := io.ReadFull(f, buf[:policy.Size])
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("local file can no longer be found: %v", readErr)}
		}
		chunk := buf[:n]

		result, derr, terr := SendChunk(ctx, client, sessionURI, chunk, bytesSent, totalBytes)
		if terr != nil || derr != nil {
			outcome, retry := classifyAndMaybeRetry(ctx, terr, derr, false, cb, &paused, backoff, policy)
			if outcome != nil {
				return nil, outcome
			}
			if !retry {
				return nil, ctx.Err()
			}
			if bytesSent > 0 {
				if outcome := checkIdentity(localPath, baseline, bytesSent, checksum); outcome != nil {
					return nil, outcome
				}
			}
			continue // retry the same chunk from the same start offset -- nothing already acknowledged is re-sent
		}

		if paused {
			if cb.OnResumed != nil {
				cb.OnResumed()
			}
			paused = false
		}
		backoff.Reset()
		policy.OnSuccess()

		checksum.Write(chunk)
		bytesSent += int64(n)
		hashState, herr := MarshalChecksum(checksum)
		if herr != nil {
			return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("internal error checkpointing upload: %v", herr)}
		}
		if cb.OnChunkAcked != nil {
			cb.OnChunkAcked(bytesSent, sessionURI, hashState, policy.Size, policy.ConsecutiveSuccesses)
		}

		if result.Done {
			return &UploadResult{FileID: result.FileID, WebViewLink: result.WebViewLink}, nil
		}
	}

	// Not normally reached (the final chunk's response always carries
	// Done), but guards against a total-size mismatch by asking Drive directly.
	res, derr, terr := QueryOffset(ctx, client, sessionURI, totalBytes)
	if terr == nil && derr == nil && res.Done {
		return &UploadResult{FileID: res.FileID, WebViewLink: res.WebViewLink}, nil
	}
	return nil, &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: "upload ended without Drive confirming completion"}
}

// classifyAndMaybeRetry classifies a failed attempt (either a transport
// error or a parsed Drive error -- exactly one is non-nil) and, for a
// Retryable outcome, shrinks the chunk-size policy (unless the failure was
// a session-initiation attempt, which has no chunk in flight yet --
// research.md §3), transitions to paused (once), and sleeps the backoff
// policy's next delay. Returns (outcome, false) for a terminal bucket
// (which never reaches the shrink call, satisfying FR-012), or (nil, true)
// once it's time to retry, or (nil, false) if ctx was cancelled while waiting.
func classifyAndMaybeRetry(ctx context.Context, transportErr error, de *DriveError, isSessionInitiation bool, cb UploadCallbacks, paused *bool, backoff *BackoffPolicy, policy *ChunkSizePolicy) (*TerminalOutcome, bool) {
	var c Classification
	if transportErr != nil {
		c = ClassifyTransportError(transportErr)
	} else {
		c = ClassifyDriveError(de, isSessionInitiation)
	}

	if c.Bucket != Retryable {
		return &TerminalOutcome{Bucket: c.Bucket, Reason: c.Reason}, false
	}

	if !isSessionInitiation {
		policy.OnFailure()
	}

	if !*paused {
		*paused = true
		if cb.OnPaused != nil {
			cb.OnPaused()
		}
	}
	delay := backoff.NextDelay()
	if err := sleepForRetry(ctx, delay); err != nil {
		return nil, false
	}
	return nil, true
}

// checkIdentity runs the source-file-identity check (research.md §5)
// before (re)sending a chunk on top of bytesSent already-acknowledged
// bytes: a cheap size/mtime comparison against baseline, falling back to a
// bounded re-hash of exactly the acknowledged prefix only when the cheap
// check fails. Returns nil if the file is still safe to resume against,
// or a *TerminalOutcome (TerminalRecoverable/file_changed, or
// TerminalNotRecoverable if the file is now missing/unreadable) otherwise.
func checkIdentity(localPath string, baseline IdentityBaseline, bytesSent int64, checksum hash.Hash) *TerminalOutcome {
	ok, err := CheapIdentityCheck(localPath, baseline)
	if err != nil {
		return &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("local file can no longer be found: %v", err)}
	}
	if ok {
		return nil
	}

	state, err := MarshalChecksum(checksum)
	if err != nil {
		return &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("internal error verifying upload checkpoint: %v", err)}
	}
	verified, err := VerifyPrefix(localPath, bytesSent, state)
	if err != nil {
		return &TerminalOutcome{Bucket: TerminalNotRecoverable, Reason: fmt.Sprintf("local file can no longer be found: %v", err)}
	}
	if verified {
		return nil
	}
	return &TerminalOutcome{Bucket: TerminalRecoverable, Reason: ReasonFileChanged}
}
