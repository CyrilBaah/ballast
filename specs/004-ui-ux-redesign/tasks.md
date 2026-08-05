---

description: "Task list for Full Experience UI/UX Redesign"
---

# Tasks: Full Experience UI/UX Redesign

**Input**: Design documents from `/specs/004-ui-ux-redesign/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, contracts/design-tokens.md, quickstart.md (all present)

**Tests**: Included. Not explicitly requested in spec.md, and Constitution
Principle III (test-first, NON-NEGOTIABLE) is explicitly N/A for this
feature per plan.md's Constitution Check — no session/offset/resume/
retry-classification/chunk-sizing logic is touched. Tests are included
anyway because plan.md's Testing section and research.md commit to a
concrete Playwright strategy for the redesigned screens plus Go tests for
the new `ListRecentUploads`/account-profile read paths — skipping those
tasks would silently drop work the plan already designed.

**Organization**: Tasks are grouped by user story (spec.md: US1 Cohesive
visual identity, US2 Consistent state feedback, US3 Upload history list)
to enable independent implementation and testing of each story. Account
identity/storage-quota (FR-011/FR-012) is part of US1 — it's rendered by
the same sidebar shell US1 introduces and is the other half of "the app
feels like one considered product."

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths follow plan.md's Project Structure (single Wails project,
  additive to Features 001–003)

## Path Conventions

- Go backend: `app.go`, `internal/auth/`, `internal/drive/`, `internal/storage/`
- E2E mock server: `mock_e2e.go` (repo root)
- Frontend: `frontend/src/styles/`, `frontend/src/screens/`,
  `frontend/src/api/`, `frontend/src/main.ts`, `frontend/tests/`

---

## Phase 1: Setup (Shared Test Infrastructure)

**Purpose**: Test-support scaffolding needed to deterministically trigger
the picker's loading/error states and the account-identity/storage-quota
variants in Playwright — no new production dependencies are introduced
(plan.md's Technical Context: no new framework/library).

- [X] T001 [P] Extend `mock_e2e.go`'s `/files` (Drive folder listing)
  handler with two new outcomes read via the existing
  `BALLAST_E2E_OUTCOME_FILE` mechanism: `slow-list` (artificial delay
  before responding, to make the loading state observable) and
  `500-list` (a Drive-shaped error response, to trigger a folder-load
  failure) — needed by User Story 2's Playwright tests
- [X] T002 [P] Extend `mock_e2e.go`'s userinfo endpoint and add an
  `about.get` handler, both driven by `BALLAST_E2E_OUTCOME_FILE`: userinfo
  outcomes for name+picture, name-only, and neither; `about.get` outcomes
  for `storageQuota.limit` present vs. absent (unlimited-storage case),
  plus a hard-failure outcome (500/network-fail) to test the
  silent-omission path (spec.md FR-012, Clarifications 2026-08-05) —
  needed by User Story 1's account-identity tests and quickstart.md
  Scenario 1 steps 5–7

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Design-token foundation and data/binding plumbing that MUST
be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 [P] Create `frontend/src/styles/tokens.css` implementing the
  full token set from contracts/design-tokens.md — color roles
  (`--color-bg`, `--color-surface`, `--color-text`, `--color-text-muted`,
  `--color-accent`, `--color-accent-hover`, `--color-border`,
  `--color-error`, `--color-success`, `--color-warning`) with light
  values by default and dark overrides under
  `@media (prefers-color-scheme: dark)`, typography scale
  (`--font-family`, `--font-size-body`, `--font-size-small`,
  `--font-size-heading`, `--font-weight-heading`), spacing scale
  (`--space-xs` … `--space-xl`), shape/elevation tokens (`--radius-sm`,
  `--radius-md`, `--shadow-card`), the six `--filetype-*` accent tokens
  (reused by the Avatar fallback gradient per contracts/design-tokens.md),
  and a `--focus-ring` token
- [X] T004 Import `tokens.css` in `frontend/src/main.ts` before
  `style.css`/`app.css` so every screen has the tokens available
  (depends on T003)
- [X] T005 [P] Add a `drive_folder_name TEXT` column to the `upload`
  table (additive migration, guarded the same way `ensureSchema()`
  guards existing migrations) in `internal/storage/schema.go`
  (data-model.md)
- [X] T006 Extend `storage.Upload`/`CreateUpload` to accept and persist
  `driveFolderName`, and add `ListRecentUploads` (rows ordered by
  `started_at DESC`, capped at 50) to `internal/storage/upload.go`
  (data-model.md) (depends on T005)
- [X] T007 Update `App.UploadStart`'s signature to accept
  `driveFolderName` and pass it through to `CreateUpload`; add the
  `Upload.ListRecent` Wails-bound method returning `UploadListItemDTO`
  (`fileName` via `filepath.Base(local_path)`, `driveFolderName` falling
  back to `"My Drive"` when null) in `app.go` (contracts/wails-bindings.md)
  (depends on T006)
- [X] T008 [P] Export the updated `Start` signature and the new
  `ListRecent` binding via `frontend/src/api/upload.ts` (depends on T007)
- [X] T009 [P] Add the `profile` OAuth scope to `oauthConfig` and parse
  `name`/`picture` from the userinfo response into `UserInfo`/`Session`
  in `internal/auth/oauth.go` (research.md §7)
- [X] T010 [P] Add `display_name TEXT`/`picture_url TEXT` columns to the
  `account` table (additive migration) in `internal/storage/schema.go`,
  and extend `storage.Account`/`UpsertAccount`/`GetAccount` to carry them
  in `internal/storage/account.go` (data-model.md)
- [X] T011 Update `App.AuthGetStatus`/`App.AuthSignIn` to populate and
  persist `display_name`/`picture_url` and return `name`/`pictureUrl` in
  `AuthStatus` (contracts/wails-bindings.md); export via
  `frontend/src/api/auth.ts` (depends on T009, T010)
- [X] T012 [P] Implement `internal/drive/about.go` wrapping Drive's
  `about.get` (`fields=storageQuota`) via the existing `driveService(ctx)`
  helper (research.md §8); add the `Drive.GetStorageQuota` Wails-bound
  method in `app.go` returning `StorageQuota` (`limitBytes` omitted for
  unlimited accounts, data-model.md); export via `frontend/src/api/drive.ts`

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 - A cohesive, polished visual identity across the whole app (Priority: P1) 🎯 MVP

**Goal**: Sign-in, picker, and progress screens share one consistent
visual system (colors, typography, spacing, control styling), remain
legible across the app's supported window sizes, and the sidebar shows
who's signed in — name, photo, and Drive storage usage.

**Independent Test**: Walk sign-in → picker → progress using only the
redesigned interface and confirm no jarring style shift between screens;
resize the window at each screen and confirm no clipped or overlapping
content; confirm the sidebar shows account name/photo/storage, falling
back to a generated avatar when no photo is available (quickstart.md
Scenario 1).

### Tests for User Story 1

- [X] T013 [P] [US1] Playwright test asserting no layout
  overflow/clipping on the sign-in, picker, and progress screens (the
  latter two inside the new sidebar shell) at minimum, default, and
  large supported window sizes, in
  `frontend/tests/visual-consistency.spec.ts` (quickstart.md Scenario 1
  step 4; SC-002)
- [X] T014 [P] [US1] Playwright test covering the sidebar's account
  row: name + photo render when both are present (T002's name+picture
  outcome); a generated-initials avatar renders when no photo is
  available or it fails to load (T002's name-only outcome), falling back
  to the email's initial when name is also absent (T002's neither
  outcome); an unlimited-storage account (T002's no-`limit` outcome)
  renders without dividing by zero; and a failed `GetStorageQuota` call
  (T002's hard-failure outcome) omits the storage bar while name/photo
  still render (spec.md FR-012), in
  `frontend/tests/account-identity.spec.ts` (quickstart.md Scenario 1
  steps 5–7; SC-006)

### Implementation for User Story 1

- [X] T015 [US1] Rewrite `frontend/src/style.css` to consume
  `tokens.css` custom properties instead of hardcoded values (removes
  the hardcoded `rgba(27, 38, 54, 1)` background and `white` text)
  (depends on T003, T004)
- [X] T016 [US1] Implement the persistent left sidebar shell in
  `frontend/src/main.ts` — two nav items (Upload, History) and a bottom
  account-status row wrapping the signed-in screens (picker, progress,
  and the upcoming history screen); sign-in stays full-screen and
  outside this shell, unchanged in structure (research.md §6). The
  account-status row calls `Auth.GetStatus` for `name`/`pictureUrl` and
  `Drive.GetStorageQuota` once on mount, rendering an `<img>` for the
  photo with a generated-initials fallback on error/absence (falling
  back to the email's first letter if `name` is also absent), and a
  storage-usage bar that omits itself gracefully both when `limitBytes`
  is absent (unlimited account) and when the `GetStorageQuota` call
  itself fails (contracts/wails-bindings.md, spec.md FR-012) (depends on
  T004, T011, T012)
- [X] T017 [US1] Rewrite `frontend/src/app.css` so `.signin-screen`
  (full-screen, unchanged structurally aside from T033's wave layer),
  the new sidebar shell (including the avatar circle and storage-usage
  bar), and `.picker-screen`/`.progress-screen` and their controls
  (buttons, inputs, breadcrumb, folder list) consume `tokens.css`'s
  color/typography/spacing/shape scale instead of today's ad hoc hex
  values and bare `opacity` text-dimming, and remain legible across the
  app's supported window sizes; also implements the gradient `.btn` fill
  (Primary button fill), the sidebar's translucent/blurred surface
  (Sidebar translucency), and the `.content` scrim (Content scrim) per
  contracts/design-tokens.md (depends on T003, T004, T016)
- [X] T033 [P] [US1] Add the ambient wave/dot `<svg class="wave-layer">`
  markup (contracts/design-tokens.md's Ambient background) to
  `frontend/src/screens/signin.ts` (behind the sign-in card, full-screen)
  and once to the sidebar shell markup in `frontend/src/main.ts` (behind
  `.shell`, shared across picker/progress/history rather than duplicated
  per screen) (depends on T004, T016)

**Checkpoint**: User Story 1 is fully functional and independently testable

---

## Phase 4: User Story 2 - Always-clear feedback for what's happening (Priority: P2)

**Goal**: Loading, error, success, and retrying/recovering states render
with one consistent, unmistakable visual treatment across every screen.

**Independent Test**: Trigger the picker's loading state, a sign-in
failure, a folder-load failure, an upload failure, an upload retry after
a dropped connection, and a successful upload completion — confirm each
renders a distinct, consistent visual treatment (quickstart.md Scenario 2).

### Tests for User Story 2

- [X] T018 [P] [US2] Playwright test triggering the picker's
  folder-loading state (T001's `slow-list` outcome) and folder-load
  error (T001's `500-list` outcome), asserting the correct
  state-treatment classes render, in
  `frontend/tests/state-feedback.spec.ts`
- [X] T019 [P] [US2] Playwright test triggering a sign-in failure
  (existing `deny` outcome), an upload failure, an upload success, and an
  upload retry-after-drop (existing `network-fail` outcome), asserting
  each renders its own consistent state-treatment class distinct from
  the others, and that the sign-in-failure message renders mapped
  plain-language text rather than a raw error string (FR-003), in
  `frontend/tests/state-feedback.spec.ts`

### Implementation for User Story 2

- [X] T020 [US2] Add shared state-treatment CSS classes (loading, error,
  success, warning) to `frontend/src/app.css`, each keyed to
  `tokens.css`'s `--color-error`/`--color-success`/`--color-warning`/
  `--color-accent` (contracts/design-tokens.md's State-treatment
  convention) (depends on T003)
- [X] T021 [US2] Add a loading indicator to
  `frontend/src/screens/picker.ts`, shown while `Drive.ListFolders` is
  in flight, and apply the shared error-state class/markup to its
  folder-load and upload-start error paragraphs (depends on T020)
- [X] T022 [P] [US2] Apply the shared error-state class/markup to
  `frontend/src/screens/signin.ts`'s error paragraph (depends on T020)
- [X] T023 [US2] Apply the shared success/error/warning state classes to
  `frontend/src/screens/progress.ts`'s result states (succeeded, failed,
  paused/retrying, awaiting_confirmation), replacing today's inline
  `.progress-result--*` hex colors (depends on T020)
- [X] T032 [US2] Add a small error-message mapping (e.g.
  `frontend/src/errors.ts`) that translates known technical error
  substrings — sign-in denial, keychain-unavailable, network/connection
  failures, Drive API error codes — into plain-language copy, with a
  generic fallback ("Something went wrong. Please try again.") for
  anything unmapped; wire it into signin.ts, picker.ts, and
  progress.ts's existing `err.message`/`failureReason` render sites in
  place of the raw string (FR-003) (depends on T021, T022, T023)

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - See past and current uploads at a glance (Priority: P3)

**Goal**: A screen listing past and current uploads (file name,
destination folder, status), updating live as uploads progress and
surviving app restarts.

**Independent Test**: Complete one upload, start a second, and confirm
the history screen shows both — the first `succeeded`, the second live
`in_progress` — without leaving the app; force a terminal failure and
confirm it's clearly marked; restart the app and confirm status persists
(quickstart.md Scenario 3).

### Tests for User Story 3

- [X] T024 [P] [US3] Go unit test for `ListRecentUploads` — ordering
  (most recent first), the 50-row limit, and `driveFolderName`'s
  `"My Drive"` fallback for null values — in
  `internal/storage/upload_test.go` (depends on T006)
- [X] T025 [P] [US3] Playwright test covering quickstart.md Scenario 3
  (completed + live in-progress + failed rows, status persists across
  `DebugRestart`) in `frontend/tests/upload-history.spec.ts`

### Implementation for User Story 3

- [X] T026 [US3] Implement `frontend/src/screens/history.ts` — fetch
  `Upload.ListRecent` on mount, render one row per upload (file name,
  destination folder, status, progress or failure reason, and a
  file-type icon colored via the extension-derived `--filetype-*` token
  per contracts/design-tokens.md) using US2's shared state-treatment
  classes and US1's tokens, then subscribe to
  `upload:progress`/`upload:complete`/`upload:failed`/`upload:paused`/
  `upload:awaiting-confirmation` and update the matching row by `id`,
  prepending a new row for any unseen `id` (contracts/wails-bindings.md)
  (depends on T008, T016, T017, T020)
- [X] T027 [US3] Wire the sidebar's History nav item (rendered by T016)
  in `frontend/src/main.ts` to display `history.ts` via the existing
  screen-swap (`teardown`) pattern (depends on T026)

**Checkpoint**: All three user stories independently functional — full
feature complete

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect every screen, not one story

- [X] T028 [P] Add global `:focus-visible` focus-ring styling (the
  `--focus-ring` token) to every interactive element (buttons, links,
  folder-list items, sidebar nav items, history rows) across all four
  screens in `frontend/src/app.css`, per contracts/design-tokens.md's
  Accessibility requirements (FR-007)
- [X] T029 Manual keyboard-only walkthrough and OS light/dark theme
  toggle across all four screens per quickstart.md's Edge Cases
  checklist, recording results for SC-004/SC-005
- [X] T030 [P] Audit `tokens.css`'s color pairs (text/background, each
  state color/background) for WCAG AA contrast in both light and dark
  values, adjusting any failing token values (contracts/design-tokens.md's
  Accessibility requirements)
- [X] T031 Run quickstart.md Scenarios 1–3 end-to-end and confirm
  SC-001–SC-006

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: No dependency on Setup (different files/
  concerns) but BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational (T003, T004, T011,
  T012); T014's test additionally depends on Setup (T002); T033 depends
  on T016 (the shell it adds the wave layer to)
- **User Story 2 (Phase 4)**: Depends on Foundational (T003); T018's
  test additionally depends on Setup (T001)
- **User Story 3 (Phase 5)**: Depends on Foundational (T006, T008) and,
  for T026, on US1's sidebar shell (T016) and `app.css` rewrite (T017)
  and US2's state classes (T020) existing — so in practice follows US1
  and US2
- **Polish (Phase 6)**: Depends on all three user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Tokens/schema/bindings (Foundational) before screen CSS/markup before
  cross-story integration (history screen depends on US1's sidebar shell
  and CSS rewrite, and US2's state classes)
- Story complete before moving to next priority

### Parallel Opportunities

- T001, T002 (Setup — different endpoints in the same file, but
  independent outcome branches) can be developed in parallel and merged
- T003, T005, T009, T010 (Foundational — different files/concerns) can
  run in parallel; T008 can run in parallel with T009+ once T007 lands;
  T012 can run in parallel with T009–T011 (different package)
- All [P] test tasks within a story (e.g., T013/T014, T018/T019) can run
  in parallel with each other, before that story's implementation tasks
- T022 (signin.ts) can run in parallel with T021 (picker.ts) — different
  files, both depend only on T020
- T033 (signin.ts + main.ts wave layer) can run in parallel with T017
  (app.css) — different files, both depend only on T016
- T024 (Go test) can run in parallel with T025 (Playwright test) — both
  depend only on Foundational, not on each other

---

## Parallel Example: User Story 1

```bash
# Launch both tests for User Story 1 together:
Task: "Playwright test for layout at all supported window sizes in frontend/tests/visual-consistency.spec.ts"
Task: "Playwright test for account identity (name/photo/storage, and fallbacks) in frontend/tests/account-identity.spec.ts"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run quickstart.md Scenario 1 end-to-end
5. This alone proves the app looks and feels like one cohesive product —
   sidebar shell and account identity included — before state-feedback
   polish or the history screen exist

### Incremental Delivery

1. Setup + Foundational → tokens and data plumbing ready
2. Add User Story 1 → validate Scenario 1 → cohesive visual identity,
   sidebar shell, and account identity (MVP)
3. Add User Story 2 → validate Scenario 2 → state feedback made
   legible and consistent
4. Add User Story 3 → validate Scenario 3 → upload history visible
5. Polish → accessibility/contrast verified, quickstart fully validated

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- No task in this feature touches `internal/drive/upload.go`,
  `resumable.go`, `retry.go`, or `identity.go` — the resumable-upload
  path — per FR-010. `internal/drive/about.go` (T012) is a separate,
  unrelated read-only endpoint.
- Commit after each task or logical group, one PR per task per
  CONTRIBUTING.md
- Stop at any checkpoint to validate a story independently
- Avoid: vague tasks, same-file conflicts, cross-story dependencies that
  break independence
