---

description: "Task list for Resumable, Crash-Safe Upload Engine"
---

# Tasks: Resumable, Crash-Safe Upload Engine

**Input**: Design documents from `/specs/002-resumable-upload-engine/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, quickstart.md (all present)

**Tests**: Included. Not explicitly requested in spec.md, but Constitution
Principle III makes tests for session/offset/resume logic, retry
classification, and the file-identity check NON-NEGOTIABLE, and plan.md's
Testing section and research.md §6 commit to a concrete Go-tests +
Playwright strategy backed by a fake resumable-server harness — skipping
test tasks here would silently drop work the plan already designed and the
constitution requires.

**Organization**: Tasks are grouped by user story (spec.md: US1 Upload
survives a dropped connection, US2 Upload survives an app crash/power
loss/restart, US3 Clear handling when an upload truly cannot continue) to
enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths follow plan.md's Project Structure (single Wails project,
  additive to Feature 001)

## Path Conventions

- Go backend: `app.go`, `internal/drive/`, `internal/storage/`, `internal/events/`
- E2E mock server: `mock_e2e.go` (repo root)
- Frontend: `frontend/src/screens/`, `frontend/src/api/`, `frontend/src/main.ts`, `frontend/tests/`

---

## Phase 1: Setup (Shared Test Infrastructure)

**Purpose**: Test-support scaffolding shared by every later story — no new
production dependencies are needed (the resumable protocol is implemented
with `net/http`/`crypto/sha256` from the standard library, per plan.md's
Technical Context), so Setup here is entirely about being able to write
failing tests before any implementation exists (Constitution Principle III).

- [X] T001 [P] Extend `mock_e2e.go`'s upload mock handler with configurable simulated outcomes needed by later tests/quickstart: resumable session initiate/chunk/offset-query endpoints, plus outcomes for a mid-chunk connection drop (existing hijack+close technique), `429`/`5xx`, `404`/`410` (expired session), `403 storageQuotaExceeded`, and a token-refresh failure (permission revoked) — driven by the existing `BALLAST_E2E_OUTCOME_FILE` mechanism
- [X] T002 [P] Build a Go-test-scoped fake resumable-upload `httptest.Server` harness supporting the same simulated outcomes as T001, reusable across `internal/drive` unit tests, in `internal/drive/testharness_test.go` (research.md §6)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core resumable-engine infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 Extend the `upload` table schema — add `local_mtime`, `session_uri`, `content_hash_state`, `awaiting_confirmation_reason` columns and widen the `status` CHECK constraint to include `paused`/`awaiting_confirmation` (data-model.md), as an additive migration guarded the same way `ensureSchema()` guards table creation, in `internal/storage/schema.go`
- [X] T004 [P] Extend `storage.Upload` and its persistence methods — `CreateUpload` captures `local_mtime`; new `SetUploadPaused`, `SetUploadAwaitingConfirmation` (with reason), `SetUploadResumed` transition methods; `UpdateUploadProgress` also persists `session_uri` and `content_hash_state` atomically with `bytes_sent`; the single-active-upload check now spans `in_progress`/`paused`/`awaiting_confirmation` — in `internal/storage/upload.go` (depends on T003)
- [X] T005 [P] Implement raw-HTTP resumable protocol primitives against Drive's upload endpoint — initiate a session (`POST ?uploadType=resumable`, returns the session URI from the `Location` header), send one chunk (`PUT` with `Content-Range: bytes {start}-{end}/{total}`), and query the current offset (`PUT` with `Content-Range: bytes */{total}`, empty body) — in `internal/drive/resumable.go` (research.md §1; Constitution Principle II: chunks strictly in order, 256 KiB multiples except the final chunk, never concurrent for one file)
- [X] T006 [P] Implement error classification (HTTP status/transport outcome → retryable / terminal-recoverable / terminal-not-recoverable, per research.md §4's table) and the two-tier backoff policy (2s fixed interval for the first 30s of continuous failure, then exponential base 2s ×2 capped at 30s) in `internal/drive/retry.go`, explicitly flagging both intervals as starting hypotheses pending harness validation (Constitution Principle III)
- [X] T007 [P] Implement the source-file-identity check — cheap `size`+`mtime` comparison against the stored baseline, incremental SHA-256 checkpoint update after each acknowledged chunk (serialized via `encoding.BinaryMarshaler`), and the bounded `[0, bytes_sent)` prefix-hash fallback verify used only when the cheap check fails — in `internal/drive/identity.go` (research.md §5)
- [X] T008 [P] Add `upload:paused` and `upload:awaiting-confirmation` event payload types and emit helpers to `internal/events/events.go` (contracts/wails-bindings.md)

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 - Upload survives a dropped connection (Priority: P1) 🎯 MVP

**Goal**: An upload pauses (not fails) on a lost connection and
automatically resumes from the last Drive-acknowledged byte once
connectivity returns, re-sending nothing already acknowledged.

**Independent Test**: Start an upload, simulate a network drop mid-transfer,
restore connectivity, and confirm it completes with total bytes sent over
the wire matching only the unsent remainder (quickstart.md Scenario 1).

### Tests for User Story 1

- [X] T009 [P] [US1] Go unit tests for the resumable chunk loop's retry-on-network-error behavior (pauses, resumes from the last acknowledged offset, zero re-transmission) against the T002 harness, in `internal/drive/upload_test.go`
- [X] T010 [P] [US1] Go unit tests for the two-tier backoff/classification logic (fast-tier timing, escalation, classification table rows) in `internal/drive/retry_test.go`
- [X] T011 [P] [US1] Playwright test covering quickstart.md Scenario 1 (single drop/restore, byte-identical result, three rapid drop/restore cycles) in `frontend/tests/upload-resume.spec.ts`

### Implementation for User Story 1

- [X] T012 [US1] Rewrite `UploadFile` as the resumable orchestrator in `internal/drive/upload.go` — initiate a session if none exists (T005), send chunks strictly in order in 8 MiB multiples, persist `bytes_sent`/`session_uri`/`content_hash_state` (T004) after each acknowledged chunk, and on a retryable error (T006) emit `upload:paused` and retry per the backoff policy without re-sending acknowledged bytes (depends on T004, T005, T006)
- [X] T013 [US1] Update `App.runUpload`/`UploadStart` in `app.go` to drive the new orchestrator and react to its paused/resumed transitions
- [X] T014 [US1] Update `frontend/src/screens/progress.ts` to listen for `upload:paused` and render it as "still in progress, retrying…" — never as a failure state

**Checkpoint**: User Story 1 is fully functional and independently testable

---

## Phase 4: User Story 2 - Upload survives an app crash, power loss, or restart (Priority: P2)

**Goal**: An upload interrupted by the app/process/OS dying is detected on
next launch and resumes from its last persisted checkpoint, with no lost
progress.

**Independent Test**: Start an upload, force-kill the app process
mid-transfer, relaunch, and confirm it's shown as recoverable and completes
successfully from its last saved position (quickstart.md Scenario 2).

### Tests for User Story 2

- [X] T015 [P] [US2] Go unit test simulating a process restart (opening a new `storage.DB`/`App` instance against the same SQLite file mid-upload) confirming the interrupted upload's checkpoint (`bytes_sent`, `session_uri`, `content_hash_state`) is intact and detected, in `internal/storage/upload_test.go`
- [X] T016 [P] [US2] Playwright test covering quickstart.md Scenario 2 (force-kill + relaunch, and graceful quit + relaunch, both recovering and completing correctly) in `frontend/tests/upload-recovery.spec.ts`

### Implementation for User Story 2

- [X] T017 [US2] Implement recoverable-upload detection in `App.startup()` — query for a row in `in_progress`/`paused`/`awaiting_confirmation` (normalizing a stale `in_progress` to `paused`, per data-model.md's startup-normalization rule), and kick off its resume in the background when the identity/session checks (T007) still pass, in `app.go` (depends on T004, T007, T012)
- [X] T018 [US2] Implement the `Upload.GetRecoverable` Wails-bound method in `app.go`, exported via `frontend/src/api/upload.ts` (contracts/wails-bindings.md)
- [X] T019 [US2] Wire the startup recovery check into the frontend boot sequence — call `Upload.GetRecoverable()` once sign-in resolves; route straight to the progress screen showing "Resuming upload…" if one exists, otherwise proceed to the picker screen as today — in `frontend/src/main.ts`

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - Clear handling when an upload truly cannot continue (Priority: P3)

**Goal**: Terminal conditions (quota exceeded, permission revoked, expired
session, changed/missing source file) stop automatic retrying and surface a
specific, actionable reason — with an explicit confirmation step before ever
restarting a transfer from byte 0.

**Independent Test**: Independently induce each terminal condition and
confirm the app stops retrying, shows a specific reason, and — for the two
recoverable conditions — only restarts after explicit confirmation
(quickstart.md Scenario 3).

### Tests for User Story 3

- [X] T020 [P] [US3] Go unit tests for each terminal-not-recoverable condition (quota exceeded, permission revoked/signed out, local file missing) transitioning to `failed` with the correct reason and no further retries, in `internal/drive/retry_test.go`
- [X] T021 [P] [US3] Go unit tests for the two terminal-recoverable conditions (expired session `404`/`410`, changed source file) transitioning to `awaiting_confirmation` with the correct reason, and for the false-positive mtime-touch-without-content-change case resuming normally without confirmation, in `internal/drive/identity_test.go`
- [X] T022 [P] [US3] Playwright test covering quickstart.md Scenario 3 (all conditions, plus `Upload.ConfirmRestart` restarting from byte 0 and completing correctly) in `frontend/tests/upload-terminal.spec.ts`

### Implementation for User Story 3

- [X] T023 [US3] Wire terminal-not-recoverable classification results (T006) to `SetUploadFailed` with the specific reason text (research.md §4) in `internal/drive/upload.go`
- [X] T024 [US3] Wire terminal-recoverable classification results (expired session, changed file — T006/T007) to the `awaiting_confirmation` transition, `awaiting_confirmation_reason`, and the `upload:awaiting-confirmation` emit, in `internal/drive/upload.go`
- [X] T025 [US3] Implement `ResetUploadForRestart` (clears `session_uri`/`bytes_sent`/`content_hash_state`, transitions back to `in_progress`) in `internal/storage/upload.go`, and the `Upload.ConfirmRestart` Wails-bound method in `app.go` that calls it and initiates a fresh Drive session, exported via `frontend/src/api/upload.ts`
- [X] T026 [US3] Extend `frontend/src/screens/progress.ts` to render the `awaiting_confirmation` prompt with reason-specific copy and a confirm action calling `Upload.ConfirmRestart`

**Checkpoint**: All three user stories independently functional — full feature complete

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T027 [P] Run quickstart.md Scenario 4 manually against a real large file and a real (or shaped) degraded network connection, recording results for SC-003 (degraded-network completion) and SC-006 (bounded memory on hundreds-of-GB files)
- [X] T028 Audit all new log output (session URI, byte offsets, retry attempts, identity-check outcomes) to confirm nothing credential-shaped is ever logged, per Constitution Principle IV
- [ ] T029 Run CI across macOS/Windows/Linux and confirm the new Go tests and Playwright specs (T009–T011, T015–T016, T020–T022) pass consistently under CI's mocked network conditions

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup (T002's harness is used by Foundational's own future test tasks) — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational only
- **User Story 2 (Phase 4)**: Depends on Foundational; T017 additionally depends on US1's orchestrator (T012) existing, so in practice follows US1
- **User Story 3 (Phase 5)**: Depends on Foundational; T023/T024 additionally depend on US1's orchestrator (T012) existing, so in practice follows US1 (and can proceed in parallel with US2)
- **Polish (Phase 6)**: Depends on all three user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation (Constitution Principle III, NON-NEGOTIABLE for this feature)
- Storage/protocol/classification primitives (Foundational) before orchestration (`upload.go`) before Wails-bound methods (`app.go`) before frontend screens
- Story complete before moving to next priority

### Parallel Opportunities

- T001, T002 (Setup) can run in parallel
- T004, T005, T006, T007, T008 (Foundational — different files, no interdependency beyond T003→T004) can run in parallel
- All [P] test tasks within a story (e.g., T009–T011) can run in parallel with each other, before that story's implementation tasks
- T020–T022 (US3 tests) can run in parallel with T015–T016 (US2 tests) if staffed, since they touch different files — but note both stories' *implementation* tasks depend on US1's orchestrator (T012) existing

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Go unit tests for retry-on-network-error resume behavior in internal/drive/upload_test.go"
Task: "Go unit tests for backoff/classification logic in internal/drive/retry_test.go"
Task: "Playwright test for drop/restore resume in frontend/tests/upload-resume.spec.ts"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run quickstart.md Scenario 1 end-to-end
5. This alone proves the resumable-session/chunk/checkpoint mechanism before crash-recovery or terminal-condition handling exist

### Incremental Delivery

1. Setup + Foundational → resumable-engine primitives ready
2. Add User Story 1 → validate Scenario 1 → dropped-connection resilience proven (MVP)
3. Add User Story 2 → validate Scenario 2 → crash/restart resilience added
4. Add User Story 3 → validate Scenario 3 → feature complete per spec.md
5. Polish → CI green on all three platforms, quickstart fully validated

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Constitution Principle III is NON-NEGOTIABLE for this feature specifically — do not skip writing T009–T011/T015–T016/T020–T022 before their corresponding implementation tasks
- Commit after each task or logical group, one PR per task per CONTRIBUTING.md
- Stop at any checkpoint to validate a story independently
- Avoid: vague tasks, same-file conflicts, cross-story dependencies that break independence
