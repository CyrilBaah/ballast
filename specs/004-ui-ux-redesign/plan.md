# Implementation Plan: Full Experience UI/UX Redesign

**Branch**: `004-ui-ux-redesign` | **Date**: 2026-08-04 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-ui-ux-redesign/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Replace the current unstyled Wails scaffold CSS with a small, hand-rolled
visual design system (CSS custom-property tokens for color, typography,
spacing, shape/elevation, and file-type accents, supporting both OS light
and dark themes) applied consistently across the sign-in, file/folder
picker, and upload progress screens, plus a consistent visual treatment
for loading/error/success states so the reliability work already done in
Features 002–003 (retries, resume, adaptive chunk sizing) is legible to
the user instead of invisible background behavior. The concrete visual
direction (light neutral surface, one confident blue accent, rounded
cards, a persistent left sidebar shell) follows established cloud-storage
desktop dashboard conventions rather than an unopinionated default
(research.md §6). A new upload-history screen is added, listing past and
current uploads by reading the `upload` table Feature 002 already
persists (extended with one new denormalized column, the destination
folder's display name) and subscribing to the same Wails events
progress.ts already consumes for live status; it's reached via the new
sidebar's nav items alongside the existing Upload flow. The sidebar also
surfaces the signed-in account's display name, profile photo (or a
generated-initials fallback), and Drive storage usage (FR-011/FR-012) —
the one deliberate exception to an otherwise presentation-only feature:
an additive OAuth `profile` scope and a read-only Drive `about.get` call,
neither of which changes upload, retry, resume, or chunk-sizing behavior
(spec FR-010, research.md §7–§8).

## Technical Context

**Language/Version**: Go 1.25 (backend, unchanged); TypeScript (Wails "vanilla-ts" frontend, unchanged from Features 001–003)

**Primary Dependencies**: Wails v2 (desktop shell, unchanged) — no new frontend framework, CSS library, or component kit is introduced. The design system is implemented as plain CSS custom properties (`frontend/src/styles/tokens.css`, new) consumed by the existing hand-rolled screen modules, consistent with Constitution Principle I (a new major dependency requires justification, and none exists here: five small screens don't warrant a CSS framework or router library).

**Storage**: SQLite (same single local file as Features 001–002), adding one column to the existing `upload` table — `drive_folder_name` (destination folder's display name, captured at upload creation, mirroring how `local_mtime` was denormalized in Feature 002) — rather than a new table or a live per-row Drive API lookup at list-render time (research.md §3). Also adds two columns to the existing `account` table — `display_name`, `picture_url` (research.md §7) — captured at sign-in the same way `email` already is. Drive storage quota (research.md §8) is explicitly NOT persisted — fetched fresh each session via a new `Drive.GetStorageQuota` call.

**Testing**: Go `testing` for the new `ListRecentUploads` storage query and the extended account-profile persistence (ordinary read/write-path tests, not subject to Constitution Principle III's NON-NEGOTIABLE test-first clause since no session/offset/resume/retry/chunk-sizing logic is touched); Playwright for the redesigned screens' interaction/state-feedback flows, the new history screen, and the sidebar's account-identity rendering (photo/fallback-avatar/storage-quota states), extending the existing `wails dev` + network-boundary-mocked CI setup (`mock_e2e.go` gains userinfo and `about.get` outcome variants, T002); manual keyboard-navigation and OS light/dark-theme walkthroughs captured in quickstart.md (no new visual-regression or accessibility-scanning dependency introduced — research.md §4).

**Target Platform**: Desktop — macOS, Windows, Linux (unchanged; Constitution Principle VII)

**Project Type**: desktop-app (single Wails project, unchanged structure from Features 001–003)

**Performance Goals**: State changes (loading → success/error, progress updates) render on the same event-driven path Features 001–003 already use (`upload:progress`, `upload:complete`, `upload:failed`, `upload:paused`, `upload:awaiting-confirmation`) — no polling is introduced, so perceived responsiveness matches the existing engine's own event latency, not a new target.

**Constraints**: Presentation- and interaction-layer only, aside from FR-011/FR-012's two additive read-only data sources — MUST NOT alter upload, retry, resume, or chunk-sizing behavior, and MUST NOT change the sign-in flow's shape (still one OAuth round trip, just a broader scope) (FR-010); MUST remain legible and usable under both OS light and dark color themes via `prefers-color-scheme` rather than an in-app toggle (spec Assumptions); MUST NOT introduce a new frontend framework, CSS library, or build tool; every interactive control MUST be keyboard-operable with sufficient color contrast (FR-007); a missing profile photo or storage quota MUST degrade to a fallback, never a blank/broken sidebar (FR-011).

**Scale/Scope**: Single user, single Google account, one active upload at a time (unchanged) — four screens total after this feature (sign-in, picker, progress, new upload-history list), not a multi-page application.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS** | No new language, datastore, or major dependency. The design system is vanilla CSS custom properties, not a CSS framework or component library; the history screen reuses the existing hand-rolled `teardown`-based screen-swap pattern in `main.ts`, not a new router. |
| II. Protocol Correctness Over Cleverness | **N/A** | This feature touches no Drive upload/chunk code at all — FR-010 explicitly excludes it. `Drive.GetStorageQuota` (research.md §8) is a single read-only metadata call, not part of the resumable-upload byte stream. |
| III. Test-First for the Upload Engine (NON-NEGOTIABLE) | **N/A** | No session/offset/resume/retry-classification/chunk-sizing logic is added or modified. The new storage queries (`ListRecentUploads`, account profile columns) are plain read/write paths, tested the same way existing non-engine storage code already is — not the code this principle targets. |
| IV. Security by Default | **PASS** | No new secret is introduced or exposed. `drive_folder_name`, `display_name`, and `picture_url` are display labels (the latter two no more sensitive than the existing unencrypted `email` column they sit beside), not credentials, and need no encryption. The new OAuth `profile` scope (research.md §7) requests no additional access beyond public-ish profile info Google already groups with `email` in its consent screen — it does not touch how the existing encrypted access/refresh tokens are stored or handled. |
| V. Simplicity & Bounded Scope | **PASS** | No in-app theme toggle (OS preference only), no true concurrent-upload capability added (the history list surfaces existing one-at-a-time data), no new frameworks, no quota polling loop (once-per-session fetch only, research.md §8) — matches spec Assumptions exactly. |
| VI. Reliability Gates as Acceptance Criteria | **N/A, feature has its own gates** | This feature isn't on the upload reliability path; its own Success Criteria (SC-001–SC-005) are the acceptance gates, exercised in quickstart.md. |
| VII. Cross-Platform Parity | **PASS** | `prefers-color-scheme` and keyboard focus/contrast are standard CSS/DOM behavior, identical across macOS/Windows/Linux — no OS-specific code path is introduced. |

No violations requiring Complexity Tracking justification.

## Project Structure

### Documentation (this feature)

```text
specs/004-ui-ux-redesign/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
app.go                     # UploadStart gains a driveFolderName parameter (display-only);
                            # new bound methods UploadListRecent, DriveGetStorageQuota;
                            # AuthGetStatus/AuthSignIn responses gain name/pictureUrl
                            # (contracts/wails-bindings.md)

internal/
├── auth/
│   └── oauth.go            # extended: oauthConfig gains the profile scope; UserInfo/Session
│                            # gain Name/Picture fields (research.md §7)
├── drive/
│   └── about.go             # NEW: thin wrapper around Drive's about.get (research.md §8)
├── storage/
│   ├── schema.go            # extended: upload table gains drive_folder_name; account table
│   │                         # gains display_name/picture_url (in-place migrations, same
│   │                         # pattern as Feature 002's schema upgrade)
│   ├── upload.go             # extended: CreateUpload accepts driveFolderName; new
│   │                          # ListRecentUploads query (data-model.md)
│   └── account.go             # extended: UpsertAccount/GetAccount carry display_name/picture_url
├── keychain/               # unchanged
└── events/                 # unchanged (history screen reuses existing event payloads)

frontend/
├── src/
│   ├── styles/
│   │   └── tokens.css      # NEW: color/typography/spacing/avatar-gradient custom properties,
│   │                        # light + dark via prefers-color-scheme (contracts/design-tokens.md)
│   ├── app.css              # rewritten atop tokens.css instead of hardcoded values
│   ├── style.css            # rewritten atop tokens.css (removes hardcoded dark-only colors)
│   ├── screens/
│   │   ├── signin.ts        # visual-only changes: consistent state/error treatment,
│   │   │                     # ambient wave-layer background (contracts/design-tokens.md)
│   │   ├── picker.ts        # visual-only changes: consistent state/error treatment,
│   │   │                     # clearer breadcrumb/current-location styling
│   │   ├── progress.ts      # visual-only changes: consistent state treatment
│   │   └── history.ts       # NEW: upload list screen (past + current uploads)
│   ├── api/
│   │   ├── upload.ts        # extended: ListRecent binding, Start passes driveFolderName
│   │   ├── auth.ts           # extended: AuthStatus carries name/pictureUrl
│   │   └── drive.ts           # extended: GetStorageQuota binding
│   └── main.ts               # extended: signed-in screens (picker/progress/history) now
│                              # render inside a persistent left sidebar shell (Upload /
│                              # History nav items + bottom account-status row showing
│                              # avatar/name/storage, research.md §6–§8), plus the shared
│                              # ambient wave-layer background (contracts/design-tokens.md,
│                              # one instance behind the shell rather than duplicated per
│                              # screen); sign-in remains full-screen, unchanged in layout,
│                              # matching the reference convention of a plain auth screen
│                              # outside the dashboard shell. Still the existing screen-swap
│                              # (teardown) pattern, no router library
└── tests/                    # new Playwright specs: state-feedback consistency, history
                               # screen (completed/in-progress/failed rows), keyboard nav,
                               # avatar fallback rendering
```

**Structure Decision**: Same single Wails project as Features 001–003 —
this feature is additive within the existing `internal/storage` package
(new columns, one new query), `internal/auth` (scope + parsed-field
additions), a new `internal/drive/about.go` for the single quota call,
and `frontend/src/screens` (one new
screen, three rewritten in place), plus a new `frontend/src/styles/`
directory for the design tokens. No new top-level directory, package, or
build tool is introduced.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Not applicable — see Constitution Check above.

## Post-Design Constitution Check (re-evaluated after Phase 1)

Per the constitution's Compliance Review requirement, the gate above was
re-checked against the concrete design in data-model.md and
contracts/wails-bindings.md / contracts/design-tokens.md:

- **I. Stack Discipline**: data-model.md's schema changes are additional
  columns on the existing `upload`/`account` tables; contracts/design-tokens.md
  defines CSS custom properties only, no new package.json dependency was
  introduced; `Drive.GetStorageQuota` (research.md §8) reuses the existing
  `driveService(ctx)` client, no new SDK. **PASS**.
- **II. Protocol Correctness Over Cleverness**: **N/A**, confirmed — no
  file in contracts/ or data-model.md touches session/chunk/offset logic;
  `about.get` is unrelated to the resumable-upload byte stream.
- **III. Test-First for the Upload Engine**: **N/A**, confirmed —
  `ListRecentUploads` and the account-profile columns (data-model.md) are
  plain reads/writes, no retry/resume/state-machine logic.
- **IV. Security by Default**: `drive_folder_name`, `display_name`, and
  `picture_url` (data-model.md) are declared plain TEXT columns,
  explicitly not credentials — same treatment as the existing unencrypted
  `email` column. **PASS**.
- **V. Simplicity & Bounded Scope**: contracts/wails-bindings.md adds two
  new bound methods (`Upload.ListRecent`, `Drive.GetStorageQuota`) and two
  changed signatures/payloads (`Upload.Start` gains a display-only
  parameter; `Auth.GetStatus`/`Auth.SignIn` gain two optional fields) — no
  new events, no concurrency surface, no theme-toggle setting, no quota
  polling loop. **PASS**.
- **VI. Reliability Gates as Acceptance Criteria**: quickstart.md's
  scenarios map directly to SC-001 through SC-006. **PASS**.
- **VII. Cross-Platform Parity**: confirmed — `prefers-color-scheme`,
  the token contract, and the OAuth/Drive calls are all pure CSS or
  platform-agnostic HTTP, no OS-specific branch anywhere in the design.
  **PASS**.

No new violations surfaced during design; Complexity Tracking remains empty.
