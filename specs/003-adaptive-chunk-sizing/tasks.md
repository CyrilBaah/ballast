---

description: "Task list for Adaptive Chunk-Size Tuning"
---

# Tasks: Adaptive Chunk-Size Tuning

**Input**: Design documents from `/specs/003-adaptive-chunk-sizing/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, quickstart.md (all present)

**Tests**: Included. Not explicitly requested in spec.md, but Constitution
Principle III names "adaptive chunk-sizing/concurrency algorithms" as
NON-NEGOTIABLE test-first, alongside its existing session/offset/resume and
retry-classification clause — skipping test tasks here would drop work the
constitution requires and plan.md's Testing section already commits to.

**Organization**: Tasks are grouped by user story (spec.md: US1 chunk size
grows on a healthy connection, US2 chunk size backs off on an unstable
connection, US3 a resumed upload keeps the size it earned) to enable
independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths follow plan.md's Project Structure (single Wails project, additive
  to Feature 002)

## Path Conventions

- Go backend: `app.go`, `internal/drive/`, `internal/storage/`
- No frontend changes — this feature has no UI surface (spec Assumptions)

---

## Phase 1: Setup (Shared Test Infrastructure)

**Purpose**: Test-support scaffolding shared by every later story — no new
production dependency is needed (plan.md's Technical Context), so Setup is
entirely about being able to observe and assert chunk sizes in tests before
any implementation exists.

- [X] T001 [P] Extend `fakeResumableServer` in `internal/drive/testharness_test.go` to record the byte length of every accepted chunk PUT (in addition to the existing scalar `wireBytes`/`received` counters), exposed via a new `acceptedChunkSizes() []int64` accessor (research.md §6)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core AIMD-policy and persistence plumbing that MUST be
complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Extend the `upload` table schema — add `chunk_size_bytes INTEGER NOT NULL DEFAULT 8388608` and `consecutive_chunk_successes INTEGER NOT NULL DEFAULT 0` columns (data-model.md) as a second, independently-guarded additive migration chained after Feature 002's existing `local_mtime`-guarded one, in `internal/storage/schema.go` (research.md §4)
- [X] T003 [P] Extend `storage.Upload` with `ChunkSizeBytes`/`ConsecutiveChunkSuccesses` fields, update `GetUpload` to scan them, and extend `UpdateUploadProgress` to persist both atomically alongside `bytes_sent`/`session_uri`/`content_hash_state` in the same write, in `internal/storage/upload.go` (depends on T002)
- [X] T004 [P] Implement `ChunkSizePolicy` in new `internal/drive/chunksize.go` — exported constants `BaselineChunkSize` (8 MiB), `MinChunkSize` (1 MiB), `MaxChunkSize` (64 MiB), `GrowthThreshold` (3), and a struct with `OnSuccess()` (increments the streak; every `GrowthThreshold`th success doubles the size up to `MaxChunkSize` and resets the streak) and `OnFailure()` (halves the size down to `MinChunkSize` and resets the streak) methods, mirroring `retry.go`'s `BackoffPolicy` shape (research.md §1); remove the old fixed `ChunkSize` constant from `internal/drive/resumable.go`
- [X] T005 Extend `ResumeState` with `ChunkSize int64`/`ConsecutiveSuccesses int` fields (defaulting to `BaselineChunkSize`/0 inside `UploadFile` when `ChunkSize` is zero), size `UploadFile`'s reusable read buffer to `MaxChunkSize` once per call and slice it per current chunk size instead of a fixed `ChunkSize`-sized buffer (research.md §2), and extend `UploadCallbacks.OnChunkAcked`'s signature to also carry the current chunk size and consecutive-success count so `app.go`'s `runUpload` can pass them into T003's extended `UpdateUploadProgress` — in `internal/drive/upload.go` and `app.go` (depends on T003, T004)

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 - Uploads speed up automatically on a healthy connection (Priority: P1) 🎯 MVP

**Goal**: A chunk size that starts at 8 MiB doubles after every 3
consecutive Drive-acknowledged chunks, capping at 64 MiB, so a long,
failure-free upload finishes faster than one pinned to a fixed size.

**Independent Test**: Start an upload over a fake connection with no
induced failures and confirm the amount of data sent per request grows
over the transfer up to the 64 MiB ceiling and stays there, with the file
still arriving byte-identical to the source (quickstart.md Scenario 1).

### Tests for User Story 1

- [X] T006 [P] [US1] Table-driven unit tests for `ChunkSizePolicy.OnSuccess` — doubles only on the 3rd consecutive call, caps at `MaxChunkSize`, keeps growing indefinitely at the ceiling without erroring, in `internal/drive/chunksize_test.go`
- [X] T007 [P] [US1] Go integration test driving `UploadFile` to completion against T001's extended fake server with no induced failures, asserting `acceptedChunkSizes()` follows the 8→16→32→64 MiB growth pattern (every 3rd chunk) and stays at 64 MiB thereafter, with only the final entry possibly smaller; additionally asserting every non-final entry satisfies `size % (256*1024) == 0` (FR-007), independent of the specific expected values — in `internal/drive/upload_test.go`

### Implementation for User Story 1

- [X] T008 [US1] Wire `policy.OnSuccess()` into `UploadFile`'s chunk-acknowledgment branch (the same branch that already calls `backoff.Reset()`), growing the chunk size used for the next read, in `internal/drive/upload.go` (depends on T005)

**Checkpoint**: User Story 1 is fully functional and independently testable — a failure-free run demonstrates growth to the ceiling

---

## Phase 4: User Story 2 - Uploads back off automatically on an unstable connection (Priority: P2)

**Goal**: Any retried chunk failure halves the chunk size immediately, down
to a 1 MiB floor, and resets the growth streak so recovery is gradual, not
an instant jump back to the size that just failed.

**Independent Test**: Grow the chunk size above baseline, induce a single
simulated chunk failure, and confirm the next attempt uses a smaller
amount of data; repeat failures and confirm the size never drops below the
floor (quickstart.md Scenario 2).

### Tests for User Story 2

- [X] T009 [P] [US2] Table-driven unit tests for `ChunkSizePolicy.OnFailure` — halves immediately regardless of streak progress, floors at `MinChunkSize`, and resets the consecutive-success streak to 0, in `internal/drive/chunksize_test.go`
- [X] T010 [P] [US2] Go integration test driving `UploadFile` past baseline growth, then injecting one `network-fail` outcome against T001's fake server, asserting the next accepted chunk is exactly half the pre-failure size; then asserting 3 more consecutive successes are required before the next doubling (not an immediate jump back); then injecting several consecutive failures in a row and asserting the size holds at `MinChunkSize` and never goes lower; then, in a separate run, injecting a terminal outcome (`403-quota`) after some growth and asserting the resulting `TerminalOutcome` leaves the chunk size at its pre-failure value (FR-012) — in `internal/drive/upload_test.go`

### Implementation for User Story 2

- [X] T011 [US2] Wire `policy.OnFailure()` into `classifyAndMaybeRetry`, only for the `Retryable` bucket and only when the failure is a chunk-send failure (`isSessionInitiation == false`) — never for a session-initiation retry and never for a terminal bucket, in `internal/drive/upload.go` (depends on T008; research.md §3 — this ordering also satisfies FR-012 for free, since terminal buckets already return before reaching this point)

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - Resuming an upload picks up at a sensible size, not from scratch (Priority: P3)

**Goal**: A paused, crashed, or session-expired-and-restarted upload
resumes its next chunk at the size it had earned before the interruption,
never re-ramping from baseline.

**Independent Test**: Grow an upload's chunk size above baseline, pause or
interrupt it (including a simulated Drive session expiry that forces a
byte-0 restart), let it resume, and confirm the next chunk sent uses the
size it had reached, not the baseline (quickstart.md Scenarios 3–4).

### Tests for User Story 3

- [X] T012 [P] [US3] Go unit test calling `UploadFile` with a `ResumeState` whose `ChunkSize` is above baseline, asserting the very first chunk of that call is sent at the restored size, not `BaselineChunkSize` — and that a failure immediately after resume shrinks from the restored size per US2's rule, not from baseline — in `internal/drive/upload_test.go`
- [X] T013 [P] [US3] Go unit test asserting `ResetUploadForRestart` leaves `chunk_size_bytes`/`consecutive_chunk_successes` unchanged while still zeroing `bytes_sent`/`session_uri`/`content_hash_state` and clearing `awaiting_confirmation_reason`, in `internal/storage/upload_test.go`

### Implementation for User Story 3

- [X] T014 [US3] Remove `chunk_size_bytes`/`consecutive_chunk_successes` from `ResetUploadForRestart`'s `UPDATE` statement so a restart no longer resets them, in `internal/storage/upload.go` (depends on T003; research.md §5)
- [X] T015 [US3] Update `UploadGetRecoverable`'s silent-resume path and `UploadConfirmRestart` in `app.go` to read the upload's current `ChunkSizeBytes`/`ConsecutiveChunkSuccesses` and carry them into the `ResumeState` passed to `startUpload`, instead of a zero-value `ResumeState{}` — applied uniformly regardless of which `awaiting_confirmation` reason (`session_expired` or `file_changed`) triggered the restart (depends on T005, T014; research.md §5, contracts/wails-bindings.md)

**Checkpoint**: All three user stories independently functional — full feature complete

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation that can't be fully automated in CI, plus a final
cross-platform check

- [X] T016 [P] Manually run quickstart.md Scenario 4 end-to-end via `wails dev` — pause an upload above baseline, force a session-expiry (`404`/`410`) or file-changed restart, call `Upload.ConfirmRestart`, and confirm the restarted transfer's first chunk uses the pre-interruption size for both `awaiting_confirmation` reasons
- [X] T017 [P] Manually run quickstart.md Scenario 5 against a real large file and real network connection, monitoring the app process's memory as the chunk size grows toward the 64 MiB ceiling, confirming it stays flat (SC-004)
- [X] T018 Manually run quickstart.md Scenario 1's throughput comparison (SC-001: ≥15% faster than a fixed 8 MiB baseline for a 1 GB+ file) and Scenario 2's degraded-network figures (SC-002: ≤5% re-sent bytes) over a real or shaped connection, recording whether the research.md §1 AIMD constants hold up or need adjustment per Constitution Principle III
- [X] T019 Run CI across macOS/Windows/Linux and confirm the new Go tests (T006–T007, T009–T010, T012–T013) pass consistently under CI's mocked network conditions

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup (T007/T010's tests use T001's `acceptedChunkSizes()` accessor) — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational only
- **User Story 2 (Phase 4)**: Depends on Foundational; T011 additionally depends on US1's growth wiring (T008) being in place in the same function, so in practice follows US1
- **User Story 3 (Phase 5)**: Depends on Foundational; T015 additionally depends on T005's `ResumeState` fields and T014's storage change, so in practice follows US1/US2 but has no functional dependency on their AIMD behavior itself
- **Polish (Phase 6)**: Depends on all three user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation (Constitution Principle III, NON-NEGOTIABLE for adaptive chunk-sizing algorithms)
- Policy/storage primitives (Foundational) before loop wiring (`upload.go`) before restart/resume wiring (`app.go`)
- Story complete before moving to next priority

### Parallel Opportunities

- T002 and T004 (Foundational — different files, no interdependency) can run in parallel; T003 depends on T002
- All [P] test tasks within a story (e.g., T006–T007, T009–T010, T012–T013) can run in parallel with each other, before that story's implementation task
- T012 and T013 (US3 tests) touch different files and can run in parallel with each other
- T016 and T017 (Polish, manual) are independent scenarios and can run in parallel if staffed

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Table-driven ChunkSizePolicy.OnSuccess tests in internal/drive/chunksize_test.go"
Task: "Full-run growth-sequence integration test in internal/drive/upload_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run quickstart.md Scenario 1 end-to-end
5. This alone proves the growth half of the AIMD policy before the shrink/resume halves exist

### Incremental Delivery

1. Setup + Foundational → policy type and persistence ready
2. Add User Story 1 → validate Scenario 1 → growth on a healthy connection proven (MVP)
3. Add User Story 2 → validate Scenario 2 → back-off on an unstable connection added
4. Add User Story 3 → validate Scenarios 3–4 → resume/restart preserves earned size, feature complete per spec.md
5. Polish → CI green on all three platforms, quickstart fully validated, AIMD constants checked against real network conditions

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Constitution Principle III is NON-NEGOTIABLE for this feature specifically — do not skip writing T006–T007/T009–T010/T012–T013 before their corresponding implementation tasks
- Commit after each task or logical group, one PR per task per CONTRIBUTING.md
- Stop at any checkpoint to validate a story independently
- Avoid: vague tasks, same-file conflicts, cross-story dependencies that break independence
