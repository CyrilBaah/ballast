# Quickstart: Validating the Resumable, Crash-Safe Upload Engine

This is a runnable validation guide, not implementation instructions — it
proves the feature works end-to-end against the contracts in
[contracts/wails-bindings.md](./contracts/wails-bindings.md) and the entities
in [data-model.md](./data-model.md). Each scenario maps to an Acceptance
Scenario in [spec.md](./spec.md) and, where noted, a Success Criterion.

## Prerequisites

- Everything from `specs/001-auth-picker-upload/quickstart.md`'s
  Prerequisites (OAuth client, test account, `wails dev` running,
  Playwright installed) — this feature reuses Feature 001's auth/picker
  flows unchanged.
- A way to simulate a dropped connection mid-upload without killing the
  whole test run: the extended `mock_e2e.go` fake resumable-upload server
  (research.md §6), driven by `BALLAST_E2E_OUTCOME_FILE` the same way
  Feature 001's `network-fail` outcome already works, plus new outcomes for
  `429`/`5xx`, an expired-session `404`/`410` response, and quota-exceeded
  `403`.
- A moderately large test file (large enough that an upload takes at least
  several seconds, so a mid-transfer interruption is reliably injectable)
  for Scenarios 1 and 2; a real multi-hundred-MB-or-larger file for the
  manual memory-bound spot-check in Scenario 4 (SC-006).

## Scenario 1 — Upload survives a dropped connection (User Story 1)

1. Sign in, pick a file and destination, call `Upload.Start`.
2. Once `upload:progress` reports partial bytes sent, switch the mock
   outcome to `network-fail` (simulated connection drop).
   - **Expect**: `upload:paused` fires (not `upload:failed`); `bytesSent`
     does not decrease (Acceptance Scenario 1).
3. Switch the mock outcome back to `approve` (connection restored).
   - **Expect**: within 2 seconds, `upload:progress` resumes from the
     `bytesSent` value at the point of interruption — not from 0 — and the
     total bytes actually sent over the wire (assert via the mock server's
     received-bytes counter) equals only the unsent remainder (Acceptance
     Scenario 2; **SC-001**).
4. Let the upload finish.
   - **Expect**: `upload:complete` fires; the file recorded by the mock
     server is byte-identical to the source file (Acceptance Scenario 3;
     **SC-005**).
5. Repeat steps 1–4, but toggle `network-fail` on and off three times in
   quick succession before letting it finish.
   - **Expect**: each drop/restore cycle repeats the pause/resume behavior
     with no manual intervention required (Edge Case: rapid reconnects).

## Scenario 2 — Upload survives an app crash/restart (User Story 2)

Requires Scenario 1, step 2 (an upload paused mid-transfer with a
persisted `session_uri`/`bytes_sent`).

1. With the upload from step 2 still paused, force-kill the app process
   (not a graceful quit).
2. Relaunch the app.
   - **Expect**: `Upload.GetRecoverable()` returns the upload with its
     `status` reflecting the last-persisted checkpoint and a `bytesSent`
     matching what was acknowledged before the kill — no progress lost
     (Acceptance Scenario 1; **SC-002**).
3. Restore connectivity (mock outcome `approve`).
   - **Expect**: the backend resumes automatically (no confirmation prompt,
     since neither the session nor the file changed) and the frontend shows
     "Resuming upload…" driven by `Upload.GetRecoverable()`'s response,
     then live `upload:progress` events (Acceptance Scenario 2).
4. Let it finish.
   - **Expect**: `upload:complete` fires; output file is correct and
     complete, identical to an uninterrupted upload (Acceptance Scenario 3).
5. Repeat steps 1–4, but quit the app gracefully (normal quit) instead of
   force-killing it.
   - **Expect**: identical recovery behavior on relaunch (Edge Case: manual
     close vs. crash — same recovery path).

## Scenario 3 — Terminal conditions, with and without confirmation (User Story 3)

Each condition is independent; run them separately against a fresh upload.

1. **Retryable, not visible as failure**: set the mock outcome to `429`
   (or `503`) for several consecutive chunk attempts, then `approve`.
   - **Expect**: `upload:paused` fires repeatedly but `upload:failed` never
     fires while retries are ongoing; the upload completes once the mock
     reverts to `approve` (Acceptance Scenario 1).
2. **Terminal, not recoverable — quota exceeded**: set the mock outcome to
   `403` with `storageQuotaExceeded`.
   - **Expect**: `upload:failed` fires within 5 seconds of the condition
     being detected, with a reason specifically naming storage quota — no
     further retries occur (Acceptance Scenario 2; **SC-004**).
3. **Terminal, not recoverable — permission revoked**: sign the mock
   account out server-side (mock token refresh failure) mid-upload.
   - **Expect**: `upload:failed` fires with a "signed out" reason (Edge
     Case: revoked session mid-upload); no infinite retry loop.
4. **Terminal, recoverable — expired session**: set the mock outcome to
   `404`/`410` on the session URI.
   - **Expect**: `upload:awaiting-confirmation` fires with
     `reason: "session_expired"`; calling `Upload.ConfirmRestart(id)`
     restarts the transfer from byte 0 and it completes correctly
     (Acceptance Scenario 3).
5. **Terminal, recoverable — source file changed**: pause an upload
   (Scenario 1, step 2), modify the local file's content (and therefore its
   size/mtime) on disk, then let it attempt to resume.
   - **Expect**: `upload:awaiting-confirmation` fires with
     `reason: "file_changed"` — the identity check (research.md §5) catches
     the mismatch before any bytes are sent onto the now-stale session;
     confirming restarts from byte 0 against the new file content
     (Acceptance Scenario 3).
6. **Terminal, not recoverable — source file deleted**: pause an upload,
   delete the local file, then let it attempt to resume.
   - **Expect**: `upload:failed` fires with a "local file can no longer be
     found" reason — not `awaiting-confirmation` (there's nothing to
     restart with) and not a silent hang (Edge Case).
7. **False-positive mtime touch**: pause an upload, touch the file's mtime
   without changing its content (e.g. `touch` the file, or copy it onto
   itself preserving bytes), then let it attempt to resume.
   - **Expect**: the cheap check (size+mtime) fails, but the fallback
     content-hash check (research.md §5) passes, so the upload resumes
     normally with no confirmation prompt.
8. **Terminal, not recoverable — destination folder deleted**: pause an
   upload, delete (or revoke access to) the Drive destination folder, then
   let it attempt to resume.
   - **Expect**: `upload:failed` fires within 5 seconds naming the missing
     destination — not `upload:awaiting-confirmation` (there's no session
     restart that fixes a deleted destination); `Upload.Start` on a new
     destination is required instead (Edge Case; research.md §4).
9. **Cancel a stuck upload**: pause an upload (or drive it into
   `awaiting_confirmation` via case 4 or 5 above), then call
   `Upload.Cancel(id)` instead of letting it resume or confirming a
   restart.
   - **Expect**: `Upload.GetStatus` reports `status: "cancelled"`
     immediately; a subsequent `Upload.Start` for a different file
     succeeds right away, since the single-active-upload slot is freed
     (Acceptance Scenario 4; FR-014).

## Scenario 4 — Manual spot-checks (SC-003, SC-006)

Not practical to fully automate in CI; run manually before release.

1. Start an upload of a real file of at least several hundred MB (ideally
   larger) over a real network connection.
2. While it runs, monitor the app process's memory usage.
   - **Expect**: memory usage stays flat/bounded, not growing proportional
     to bytes transferred (**SC-006**) — consistent with the chunked,
     streaming design (no full-file buffering, per FR-012 and
     Constitution Principle VI).
3. Optionally, use OS-level network shaping (e.g. `tc`/Network Link
   Conditioner) to simulate ~5% packet loss and ~500ms RTT for the duration
   of the transfer.
   - **Expect**: the upload still completes successfully without manual
     intervention (**SC-003**), just more slowly, exercising the retry/pause
     cycle organically rather than via the mock server.

## CI notes

Scenarios 1–3 run fully mocked (extending `mock_e2e.go` per research.md §6)
so CI needs neither a real Google account nor real network conditions,
matching Feature 001's existing CI approach. Scenario 4 is a manual
pre-release check, reserved for the same real-account pass Feature 001's
quickstart already carves out for SC-003-equivalent "verifiably real"
outcomes a mock can't prove.
