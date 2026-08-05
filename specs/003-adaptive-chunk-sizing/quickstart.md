# Quickstart: Validating Adaptive Chunk-Size Tuning

This is a runnable validation guide, not implementation instructions — it
proves the feature works end-to-end against the entities in
[data-model.md](./data-model.md) and the (unchanged) contracts in
[contracts/wails-bindings.md](./contracts/wails-bindings.md). Each scenario
maps to an Acceptance Scenario in [spec.md](./spec.md) and, where noted, a
Success Criterion.

## Prerequisites

- Everything from `specs/002-resumable-upload-engine/quickstart.md`'s
  Prerequisites — this feature reuses the same auth/picker flows, mock
  server, and `wails dev`/Playwright setup unchanged.
- The extended `fakeResumableServer` (research.md §6), which now exposes
  `acceptedChunkSizes()` — the byte length of every chunk it accepted, in
  order — in addition to Feature 002's existing `wireBytes`/`received`
  counters.
- A test file large enough to span many chunks at every size the policy can
  reach — at minimum several hundred MB, so a run can plausibly grow past
  the 64 MiB ceiling before the file ends.

## Scenario 1 — Chunk size grows on an all-success run (User Story 1)

1. Start an upload of the large test file against `fakeResumableServer`
   with its outcome left at the default (`approve`) throughout.
2. Let it run to completion.
   - **Expect**: `acceptedChunkSizes()` starts at 8 MiB, and after every 3
     consecutive accepted chunks at a given size, the next size doubles —
     8, 8, 8, 16, 16, 16, 32, 32, 32, 64, 64, ... — capping at 64 MiB and
     staying there for the remainder of the transfer, with only the final
     entry possibly smaller (the file's remainder) (Acceptance Scenario
     1/2).
3. Compare total wall-clock time against an equivalent run pinned to
   Feature 002's old fixed 8 MiB chunk size (or compute the expected
   round-trip savings from the recorded chunk-count reduction).
   - **Expect**: at least 15% faster for a 1 GB+ file over a stable
     connection (**SC-001**).
4. Verify the uploaded content is byte-identical to the source file
   regardless of the varying chunk sizes used to send it.

## Scenario 2 — Chunk size shrinks on failure, then re-grows gradually (User Story 2)

1. Start an upload and let the chunk size grow above 8 MiB (per Scenario
   1's pattern).
2. Set the mock outcome to `network-fail` for exactly one chunk attempt,
   then back to `approve`.
   - **Expect**: `acceptedChunkSizes()` shows the next accepted chunk at
     exactly half the size that failed (not the failed size, and not
     baseline) — Acceptance Scenario 1.
3. Let 3 more consecutive chunks succeed at that halved size.
   - **Expect**: the size does *not* jump back to the pre-failure size;
     growth resumes under the same 3-consecutive-success rule, taking the
     same number of successes to double again as any other growth step
     (Acceptance Scenario 3) — not an immediate return to what just
     failed.
4. Repeat the failure injection on several consecutive attempts in a row
   (fail, fail, fail...).
   - **Expect**: the size keeps halving on each failure and never drops
     below 1 MiB, holding there through further consecutive failures
     (Acceptance Scenario 2; **SC-003**).
5. Set the outcome to a terminal, not-recoverable condition (e.g.
   `403-quota`) instead of a retryable one.
   - **Expect**: the upload transitions to `failed` per Feature 002's
     existing behavior, and the chunk size recorded just before the
     terminal failure is unchanged by it (FR-012) — there is no further
     chunk to size.

## Scenario 3 — Resume after a pause or crash keeps the earned size (User Story 3)

Requires Scenario 1's pattern to get the chunk size above baseline first.

1. Start an upload, let it grow past 8 MiB, then set the mock outcome to
   `network-fail` to pause it mid-transfer.
2. Restore connectivity (`approve`).
   - **Expect**: the first chunk sent after resuming is at the size the
     upload had reached before the drop, not baseline (Acceptance Scenario
     1; **SC-005**).
3. Repeat, but force-kill the app process while paused (per Feature 002's
   Scenario 2) instead of just leaving it paused, then relaunch.
   - **Expect**: `Upload.GetRecoverable()`'s silently-resumed transfer
     also starts its next chunk at the pre-crash size (Acceptance Scenario
     2).
4. Immediately after either resume, inject one more `network-fail`.
   - **Expect**: the size shrinks from the *restored* size following the
     same rule as Scenario 2 — not from baseline (Acceptance Scenario 3).

## Scenario 4 — A session-expiry restart also keeps the earned size (User Story 3, Clarification)

1. Start an upload, let it grow past 8 MiB, then pause it
   (`network-fail`).
2. While paused, set the mock outcome to `404-session` (simulating the
   Drive session having expired during the pause), then let the upload
   attempt to resume.
   - **Expect**: `upload:awaiting-confirmation` fires with
     `reason: "session_expired"` (Feature 002's existing behavior,
     unchanged).
3. Call `Upload.ConfirmRestart(id)`.
   - **Expect**: the transfer restarts from byte 0 against a brand-new
     session (Feature 002's existing behavior), but the very first chunk
     of the restarted transfer is sent at the size the upload had earned
     before the pause — not baseline (spec Clarifications; this feature's
     change to `ConfirmRestart`'s internal behavior, contracts/
     wails-bindings.md).
4. Repeat steps 1–3 but trigger the *other* `awaiting_confirmation` reason
   instead — modify the local file's content while paused so the identity
   check reports `file_changed`.
   - **Expect**: the same size-preservation behavior on restart, since
     `ConfirmRestart` applies it uniformly regardless of which reason
     triggered the restart (research.md §5).

## Scenario 5 — Manual spot-check: memory stays flat as chunk size grows (SC-004)

Not practical to fully automate in CI; run manually before release.

1. Start an upload of a real, large file (large enough to let the chunk
   size grow to the 64 MiB ceiling per Scenario 1) over a real network
   connection.
2. Monitor the app process's memory usage as the chunk size grows from 8
   MiB toward 64 MiB.
   - **Expect**: memory usage does not grow proportionally with chunk
     size — it stays bounded at roughly one ceiling-sized buffer's worth
     regardless of how large the chunk size has reached (**SC-004**;
     research.md §2's single ceiling-sized reusable buffer).

## CI notes

Scenarios 1–4 run fully mocked against the extended `fakeResumableServer`
(research.md §6), needing neither a real Google account nor real network
conditions — consistent with Feature 002's existing CI approach. Scenario 5
is a manual pre-release check, the same category of "verifiably real"
outcome Feature 002's quickstart already reserves for a real-account pass.
