# Quickstart: Validating Google Sign-In, File/Folder Picker & Basic Upload

This is a runnable validation guide, not implementation instructions — it
proves the feature works end-to-end against the contracts in
[contracts/wails-bindings.md](./contracts/wails-bindings.md) and the entities
in [data-model.md](./data-model.md). Each scenario maps to an Acceptance
Scenario in [spec.md](./spec.md).

## Prerequisites

- A Google Cloud OAuth client of type **Desktop app**, with the Drive API
  enabled and scopes `drive.file` + `drive.metadata.readonly` configured
  (research.md §1).
- A real Google test account with at least one existing folder in Drive
  (beyond the root), to exercise folder browsing beyond "My Drive."
- `wails dev` running locally (starts the frontend dev server + Go backend
  together).
- Playwright installed in `frontend/` (`npm install`, `npx playwright
  install`), pointed at `http://localhost:34115` (research.md §5).

## Scenario 1 — Sign in, persist, sign out (User Story 1)

1. Launch the app for the first time (no prior session).
   - **Expect**: sign-in screen; `Auth.GetStatus()` reports `signedIn: false`.
2. Trigger sign-in, complete the Google consent screen in the system browser.
   - **Expect**: `auth:changed` fires with `signedIn: true`; app shows the
     picker screen (Acceptance Scenario 1).
3. Quit and relaunch the app.
   - **Expect**: `Auth.GetStatus()` reports `signedIn: true` immediately, no
     consent screen shown again (Acceptance Scenario 2, SC-001/SC-005).
4. Sign out from within the app.
   - **Expect**: `auth:changed` fires with `signedIn: false`; attempting
     `Drive.ListFolders` or `Upload.Start` now rejects with an auth error
     (Acceptance Scenario 3; FR-001). The OAuth grant is also revoked at
     Google's end (FR-003) — in CI, assert the mocked revoke endpoint was
     called with the stored refresh token; in the manual real-account pass
     (T039), confirm Ballast no longer appears under
     myaccount.google.com/permissions.
5. Repeat step 2 but click "Cancel"/"Deny" on the Google consent screen.
   - **Expect**: app returns to the signed-out state with a clear message; no
     `Account` row is created (Edge Case).

## Scenario 2 — Select file + destination, upload (User Story 2)

Requires being signed in (Scenario 1, step 2).

1. Call `Files.PickLocal()`, choose a small real file from disk.
   - **Expect**: returns `{ path, name, sizeBytes }` matching the chosen file.
2. Call `Drive.ListFolders(null)`.
   - **Expect**: response includes a "My Drive" root entry usable as a valid
     destination even if the account has no sub-folders (Acceptance
     Scenario 2).
3. Call `Upload.Start(path, "root")`.
   - **Expect**: resolves with an `UploadId`; the file appears in the Drive
     account's root shortly after (verify via the Drive web UI or API) —
     SC-003.
4. Repeat steps 1–2, but delete/rename/move the picked file on disk before
   calling `Upload.Start`.
   - **Expect**: `Upload.Start` rejects before creating any `Upload` row; UI
     shows "file can no longer be found, choose again" (Acceptance
     Scenario 3; FR-011).
5. Start an upload of a larger file, then disable networking mid-transfer and
   leave it off until the transfer cannot complete.
   - **Expect**: `upload:failed` fires (not a hang); `Upload.GetStatus`
     reports `status: "failed"` with a populated `failureReason`; no
     automatic retry occurs (Acceptance Scenario 4; FR-010).

## Scenario 3 — Progress and confirmation (User Story 3)

Requires Scenario 2, step 3, on a file large enough to take a few seconds to
transfer.

1. While the upload from Scenario 2 step 3 is in flight, observe
   `upload:progress` events.
   - **Expect**: at least one event every 5 seconds, with strictly
     non-decreasing `bytesSent` (SC-004; Acceptance Scenario 1).
2. Wait for completion.
   - **Expect**: `upload:complete` fires with a non-empty `driveFileLink`;
     UI shows a success message referencing that link (Acceptance
     Scenario 2; FR-008).
3. Re-run Scenario 2 step 5 (forced network-loss failure).
   - **Expect**: `upload:failed` fires with a non-empty `reason`; UI shows a
     clear failure message, never an indefinite spinner (Acceptance
     Scenario 3; FR-009; SC-006).

## CI notes

For automated Playwright runs, mock the Google OAuth consent screen and the
Drive API at the network boundary (research.md §5) so Scenario 1–3 run
without a real Google account or real file uploads in CI; reserve the real
end-to-end run (against an actual test Google account) for manual
pre-release verification of SC-003 specifically, since "verifiably present
in Drive" is the one outcome a mock cannot prove.
