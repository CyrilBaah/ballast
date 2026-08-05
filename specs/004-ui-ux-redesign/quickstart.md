# Quickstart: Validating the Full Experience UI/UX Redesign

This is a runnable validation guide, not implementation instructions — it
proves the feature works end-to-end against the contracts in
[contracts/wails-bindings.md](./contracts/wails-bindings.md),
[contracts/design-tokens.md](./contracts/design-tokens.md), and the
entities in [data-model.md](./data-model.md). Each scenario maps to an
Acceptance Scenario in [spec.md](./spec.md) and, where noted, a Success
Criterion. Most of this feature is visual/interaction, so scenarios below
are manual walkthroughs against a running `wails dev`, not just automated
assertions — automated Playwright coverage (structural: right elements,
right classes, right events consumed) supplements but doesn't replace
looking at the screen.

## Prerequisites

- Everything from `specs/001-auth-picker-upload/quickstart.md`'s
  Prerequisites (OAuth client, test account, `wails dev` running via
  `./scripts/dev.sh`).
- A way to toggle the OS-level color theme (System Settings → Appearance
  on macOS, or equivalent) without restarting the app, to verify
  `prefers-color-scheme` live-updates per FR-006.
- A file and folder name long enough to force truncation/wrapping (e.g. a
  100+ character filename) for the Edge Cases check.
- The `mock_e2e.go` outcomes from Feature 002 (`BALLAST_E2E_OUTCOME_FILE`)
  to reliably trigger loading/error/retrying/success states on demand
  rather than depending on real network conditions.
- `mock_e2e.go`'s userinfo and `about.get` mock responses configurable to
  return: a name + picture URL, a name with no picture, and neither — plus
  an `about.get` response with `storageQuota.limit` present vs. absent,
  and a hard-failure outcome for `about.get` — needed for Scenario 1
  steps 5–9.

## Scenario 1 — Cohesive visual identity across screens (User Story 1)

1. Launch the app fresh (sign-in screen).
   - **Expect**: colors, typography, and control styling match
     contracts/design-tokens.md's token values.
2. Sign in and proceed to the picker screen.
   - **Expect**: the picker now renders inside the persistent left
     sidebar shell (Upload/History nav items, account-status row);
     colors, typography, and spacing scale carry over from sign-in with
     no jarring shift (Acceptance Scenario 1; research.md §6).
3. Pick a file and destination, start an upload, and move to the progress
   screen.
   - **Expect**: same visual system and sidebar shell continue
     (Acceptance Scenario 2).
4. Resize the app window across its supported range (small, default,
   large) on each of the three screens.
   - **Expect**: no clipped or overlapping content at any size
     (Acceptance Scenario 3; **SC-002**).
5. With a test account that has a Google profile photo, check the
   sidebar's account row.
   - **Expect**: display name, profile photo, and storage usage
     ("X GB of Y GB") all render (Acceptance Scenario 4; **SC-006**).
6. Repeat with a test account that has no profile photo (or force
   `pictureUrl` to fail to load).
   - **Expect**: a generated-initials avatar renders instead of a broken
     image or blank space (FR-011).
7. Simulate `Drive.GetStorageQuota` returning no `limitBytes` (an
   unlimited-storage account).
   - **Expect**: the sidebar shows usage without dividing by zero or
     crashing — no progress bar, or one that reads as "unlimited"
     (data-model.md's StorageQuota note).
8. Simulate `Drive.GetStorageQuota` failing outright (network error/500).
   - **Expect**: the storage indicator is omitted silently for that
     session — no error message, no retry button — while name and photo
     still render normally (FR-012, Clarifications 2026-08-05).
9. Simulate a userinfo response with neither a name nor a picture.
   - **Expect**: the name line falls back to the email address, and the
     avatar shows the email's first letter as its initial (FR-011,
     Clarifications 2026-08-05).

## Scenario 2 — Consistent state feedback (User Story 2)

1. On the picker screen, throttle or delay the `Drive.ListFolders` mock
   response.
   - **Expect**: a clear loading indicator appears while folders are
     loading, not a blank list (Acceptance Scenario 1).
2. Trigger a sign-in failure (revoke/misconfigure test OAuth
   credentials), a folder-load failure, and an upload failure (via
   `mock_e2e.go`'s `network-fail` outcome escalated to a terminal
   condition) in turn.
   - **Expect**: each renders with the same error visual treatment
     (`--color-error` per contracts/design-tokens.md) and a message a
     non-technical user could understand (Acceptance Scenario 2).
3. Let an upload complete successfully.
   - **Expect**: an unambiguous success state (`--color-success`),
     visually distinct from the in-progress state (Acceptance Scenario 3).
4. Show each of the loading, error, and success states from steps 1-3
   above (plus the retrying/recovering state from step 5 below) to
   someone unfamiliar with the app, presented one at a time out of
   order, without any explanation of what they're looking at.
   - **Expect**: they can correctly name which state is which — working,
     failed, or succeeded — using only what's on screen (**SC-001**).
5. Using `mock_e2e.go`'s `network-fail` outcome (Feature 002), interrupt
   an upload mid-transfer, then restore connectivity.
   - **Expect**: while paused/retrying, the interface shows an
     active/recovering treatment (`--color-warning`), not one visually
     identical to a hard failure (Acceptance Scenario 4).

## Scenario 3 — Upload history list (User Story 3)

1. Complete one upload, then start a second (different file).
2. Open the history screen (new nav entry point from `main.ts`).
   - **Expect**: both uploads appear — the first shows `succeeded` with
     file name and destination folder; the second shows live
     `in_progress` status that updates as `upload:progress` events arrive,
     without leaving the app (Acceptance Scenario 1, 2; **SC-003** — locate
     status in under 10 seconds).
3. Using `mock_e2e.go`, force the second upload to a terminal failure.
   - **Expect**: its row updates to a clearly marked `failed` status with
     `failureReason`, visually distinct from the `succeeded` row
     (Acceptance Scenario 3).
4. Quit and relaunch the app (`DebugRestart` or a real restart) while an
   upload is `paused`/`awaiting_confirmation`.
   - **Expect**: the history list reflects the upload's actual persisted
     status on next launch (Edge Case: crash-safety carries through to the
     list, per Feature 002).

## Edge Cases checklist

- Long filename/folder name (Prerequisites): confirm truncation/wrapping,
  no layout break, on both the picker and history screens.
- Empty Drive folder (a destination with no subfolders): confirm a clear
  empty state, distinguishable from loading or a broken list.
- OS theme toggle while the app is running: confirm the interface updates
  to match (or at minimum remains legible after a relaunch, if live
  media-query updates aren't picked up by the running window) — no
  unreadable text or invisible controls in either mode (**SC-004**).
- Keyboard-only walkthrough: `Tab` through sign-in, picker (including the
  folder list and breadcrumb), progress, and history screens; confirm
  every interactive control receives a visible focus ring and can be
  activated with `Enter`/`Space` (**SC-005**).
