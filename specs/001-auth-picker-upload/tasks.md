---

description: "Task list for Google Sign-In, File/Folder Picker & Basic Upload"
---

# Tasks: Google Sign-In, File/Folder Picker & Basic Upload

**Input**: Design documents from `/specs/001-auth-picker-upload/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, quickstart.md (all present)

**Tests**: Included. Not requested explicitly in spec.md, but plan.md's Testing
section and research.md §5 commit to a concrete Go-tests + Playwright strategy,
and quickstart.md's three scenarios are written as executable test cases —
skipping test tasks here would silently drop work the plan already designed.

**Organization**: Tasks are grouped by user story (spec.md: US1 Sign in with
Google, US2 Select file+destination and upload, US3 Progress and confirmation)
to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths follow plan.md's Project Structure (single Wails project)

## Path Conventions

- Go backend: `main.go`, `app.go`, `internal/<package>/`
- Frontend: `frontend/src/screens/`, `frontend/tests/`
- CI: `.github/workflows/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [X] T001 Initialize the Wails project skeleton (`wails init` vanilla-ts template) matching plan.md's Project Structure, producing `main.go`, `wails.json`, and `frontend/` scaffolding at the repo root
- [X] T002 [P] Add Go module dependencies (`google.golang.org/api/drive/v3`, `golang.org/x/oauth2`, `golang.org/x/oauth2/google`, `modernc.org/sqlite`, `github.com/zalando/go-keyring`) to `go.mod`
- [X] T003 [P] Configure Go linting/formatting (`gofmt`, `go vet`, golangci-lint) in `.golangci.yml`
- [X] T004 [P] Set up Playwright in the frontend project (`frontend/package.json` dev dependency, `frontend/playwright.config.ts`) pointed at `wails dev`'s server (`http://localhost:34115`, per research.md §5)
- [X] T005 [P] Create GitHub Actions CI workflow running `go build`/`go vet`/`go test` and the Playwright suite across macOS/Windows/Linux runners in `.github/workflows/ci.yml`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T006 Implement SQLite connection/bootstrap (`modernc.org/sqlite`, DB file in the OS app-data directory) in `internal/storage/db.go`
- [X] T007 Create the `Account` and `Upload` table schemas per data-model.md in `internal/storage/schema.go`
- [X] T008 [P] Implement the OS-keychain wrapper (`go-keyring`: fetch-or-create the AES-256-GCM data-encryption key; fail closed with a clear error if the keychain is unavailable, per research.md §4 / Constitution Principle VII) in `internal/keychain/keychain.go`
- [X] T009 [P] Implement AES-256-GCM token encryption/decryption helpers (per-operation nonce, never reused) in `internal/storage/crypto.go` — pure functions over byte blobs, no dependency on T006/T007
- [X] T010 [P] Define Wails event-emission helpers for `auth:changed`, `upload:progress`, `upload:complete`, `upload:failed` in `internal/events/events.go`
- [X] T011 Wire up the Wails entrypoint: bind the `App` struct (empty methods for now) and window config in `main.go` and `app.go`
- [X] T012 [P] Configure structured logging with an explicit rule that credential values (tokens, ciphertext, keychain material) are never logged at any level, per Constitution Principle IV, in `internal/logging/logging.go`

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 - Sign in with Google (Priority: P1) 🎯 MVP

**Goal**: A user connects the app to their Google account, the session
persists across restarts, and sign-out cleanly revokes local access.

**Independent Test**: Launch the app, complete Google sign-in, relaunch and
confirm no re-prompt, then sign out and confirm file/folder/upload
capabilities are no longer reachable (quickstart.md Scenario 1).

### Tests for User Story 1

- [X] T013 [P] [US1] Playwright test covering quickstart.md Scenario 1 (sign-in, persistence across relaunch, sign-out, cancel/deny mid-flow) in `frontend/tests/auth.spec.ts`
- [X] T014 [P] [US1] Go unit tests for AES-256-GCM token encryption/decryption round-trip in `internal/storage/crypto_test.go`
- [X] T015 [P] [US1] Go unit tests for the OAuth loopback+PKCE request/response handling, with the HTTP exchange mocked, in `internal/auth/oauth_test.go`
- [X] T015a [P] [US1] Go unit test for `Auth.SignOut`'s revoke-endpoint call (mocked HTTP: verify request shape/token, and that local Account deletion still proceeds when the mocked call fails) in `internal/auth/revoke_test.go`

### Implementation for User Story 1

- [X] T016 [US1] Implement `Account` row persistence — create on sign-in, delete (not flag) on sign-out, enforcing the single-account constraint at the application layer (data-model.md) — in `internal/storage/account.go`
- [X] T017 [US1] Implement the OAuth 2.0 loopback+PKCE flow (local listener on an OS-assigned port, opens system browser, exchanges code, requests `drive.file` + `drive.metadata.readonly` scopes) in `internal/auth/oauth.go`
- [X] T018 [US1] Implement `Auth.GetStatus`, `Auth.SignIn`, `Auth.SignOut` Wails-bound methods on the `App` struct, emitting `auth:changed`, in `app.go` — `Auth.SignOut` MUST call Google's OAuth revocation endpoint (`https://oauth2.googleapis.com/revoke`) with the stored refresh token before deleting the `Account` row (FR-003); proceed with local deletion even if the revoke call fails
- [X] T019 [US1] Implement silent access-token refresh (check `access_token_expiry` before Drive API calls) in `internal/auth/refresh.go`
- [X] T020 [US1] Build the sign-in screen calling `Auth.GetStatus`/`Auth.SignIn` and reacting to `auth:changed` in `frontend/src/screens/signin.ts`
- [X] T021 [US1] Surface the fail-closed keychain-unavailable error (research.md §4) as a clear, specific message in `frontend/src/screens/signin.ts`

**Checkpoint**: User Story 1 is fully functional and independently testable

---

## Phase 4: User Story 2 - Select a file and destination, then upload (Priority: P2)

**Goal**: A signed-in user picks a local file, browses/selects a Drive
destination folder, and the file is transferred to that folder.

**Independent Test**: While signed in, pick a local file, list Drive folders
(including root with no sub-folders), start an upload, and confirm the file
is verifiable in Drive afterward; separately confirm a vanished local file
and a network-loss mid-upload both fail clearly rather than hang
(quickstart.md Scenario 2).

### Tests for User Story 2

- [X] T022 [P] [US2] Playwright test covering quickstart.md Scenario 2 (pick file, list folders, start upload, file-vanished-before-upload case, network-loss failure) in `frontend/tests/upload-flow.spec.ts`
- [X] T023 [P] [US2] Go unit tests for the Drive folder-listing query construction (mimeType filter, parent-folder scoping, root handling) in `internal/drive/folders_test.go`
- [X] T024 [P] [US2] Go unit tests for `Upload` row state transitions (`pending → in_progress → succeeded|failed`) in `internal/storage/upload_test.go`

### Implementation for User Story 2

- [X] T025 [P] [US2] Implement `Upload` row persistence and state-transition logic per data-model.md in `internal/storage/upload.go`
- [X] T026 [P] [US2] Implement the Drive folder-listing client wrapper (`Files.List` filtered to folders, `PARENT_ID` scoping, `"root"` for My Drive) in `internal/drive/folders.go`
- [X] T027 [US2] Implement the native local-file-picker binding (`Files.PickLocal`, Wails runtime dialog, single-select mode per FR-004's "exactly one file") in `app.go`
- [X] T028 [US2] Implement the `Drive.ListFolders` Wails-bound method (rejects when signed out, per FR-001) in `app.go`
- [X] T029 [US2] Implement the non-resumable Drive upload (`Files.Create(...).Media(reader, googleapi.ChunkSize(0))`), re-stating the local file immediately before starting to catch a vanished file (FR-011), in `internal/drive/upload.go`
- [X] T030 [US2] Implement `Upload.Start` (rejecting with a distinct auth error when signed out, per FR-001 — not a generic upload failure) and `Upload.GetStatus` Wails-bound methods in `app.go`
- [X] T031 [US2] Build the file+folder picker screen calling `Files.PickLocal`, `Drive.ListFolders`, and `Upload.Start` in `frontend/src/screens/picker.ts`

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - See upload progress and confirmation (Priority: P3)

**Goal**: While an upload runs, the user sees live progress; when it ends,
they see an unambiguous success (with a Drive link) or failure message.

**Independent Test**: Start an upload large enough to take a few seconds,
observe `upload:progress` events at least every 5 seconds with
non-decreasing bytes sent, then confirm a terminal `upload:complete` (with
link) or `upload:failed` (with reason) always fires — never neither
(quickstart.md Scenario 3).

### Tests for User Story 3

- [ ] T032 [P] [US3] Playwright test covering quickstart.md Scenario 3 (progress event cadence, success confirmation with link, failure messaging) in `frontend/tests/upload-progress.spec.ts`
- [ ] T033 [P] [US3] Go unit tests for the counting-reader progress wrapper's throttling (~1/s emit, non-decreasing bytes) in `internal/drive/progress_test.go`

### Implementation for User Story 3

- [ ] T034 [US3] Implement the counting-reader wrapper that emits `upload:progress` (throttled to ~1/s, comfortably inside SC-004's 5s bound) in `internal/drive/progress.go`
- [ ] T035 [US3] Emit terminal `upload:complete` / `upload:failed` events from the upload flow, guaranteeing exactly one always fires for an `in_progress` upload, in `internal/drive/upload.go`
- [ ] T036 [US3] Build the upload progress/result screen listening for `upload:progress`, `upload:complete`, and `upload:failed` in `frontend/src/screens/progress.ts`

**Checkpoint**: All three user stories independently functional — full feature complete

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T037 [P] Add a Quickstart section to `README.md` (it currently has none) with `wails dev` run instructions
- [ ] T038 Audit all log output at every level to confirm no credential values (tokens, ciphertext, keychain material) are ever logged (Constitution Principle IV)
- [ ] T039 [P] Run `quickstart.md` Scenarios 1–3 manually end-to-end against a real Google test account, verifying SC-003 ("verifiably present in Drive") specifically, since CI's mocked run cannot prove it
- [ ] T040 Run CI across macOS/Windows/Linux and confirm the keychain-unavailable fallback message (T021) actually appears on a Linux runner without a keyring daemon (Constitution Principle VII)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational only
- **User Story 2 (Phase 4)**: Depends on Foundational; T026/T029 depend on US1's authenticated Drive client (T017) being available, so in practice follows US1
- **User Story 3 (Phase 5)**: Depends on Foundational; T034/T035 depend on US2's upload flow (T029) existing, so in practice follows US2
- **Polish (Phase 6)**: Depends on all three user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Storage/entity tasks before client/flow tasks before Wails-bound methods before frontend screens
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] (T002–T005) can run in parallel after T001
- Foundational tasks T008, T009, T010, T012 (different files, no interdependency) can run in parallel; T006→T007 is a strict chain
- All [P] test tasks within a story (e.g., T013–T015) can run in parallel with each other, before that story's implementation tasks
- T023/T024 (US2 tests) can run in parallel with T013–T015 (US1 tests) if staffed, since they touch different files — but note US2's *implementation* tasks depend on US1's auth client existing

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Playwright test for sign-in/persist/sign-out in frontend/tests/auth.spec.ts"
Task: "Go unit tests for token encryption round-trip in internal/storage/crypto_test.go"
Task: "Go unit tests for OAuth loopback+PKCE handling in internal/auth/oauth_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run quickstart.md Scenario 1 end-to-end
5. This alone proves the auth+persistence pipeline before any upload code exists

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. Add User Story 1 → validate Scenario 1 → sign-in pipeline proven
3. Add User Story 2 → validate Scenario 2 → core upload value delivered (full MVP)
4. Add User Story 3 → validate Scenario 3 → feature complete per spec.md
5. Polish → CI green on all three platforms, quickstart fully validated

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Verify tests fail before implementing (Constitution's test-first spirit, even though Principle III's NON-NEGOTIABLE gate is scoped to the later resumable engine, per plan.md's Constitution Check)
- Commit after each task or logical group, one PR per task per CONTRIBUTING.md
- Stop at any checkpoint to validate a story independently
- Avoid: vague tasks, same-file conflicts, cross-story dependencies that break independence
