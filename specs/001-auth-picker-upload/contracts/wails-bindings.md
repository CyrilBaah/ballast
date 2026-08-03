# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Ballast is a single Wails process — there is no external network API for
this feature. The interface this feature exposes is the set of Go methods
Wails binds to the frontend (called as `async` TS functions via generated
`wailsjs` bindings) plus the events the backend emits via
`runtime.EventsEmit`. This is the contract other work (Playwright specs, the
frontend screens) is written against.

## Bound methods (frontend calls backend)

### `Auth.GetStatus() -> AuthStatus`
Returns the current session state on app load, so the frontend knows
whether to show the sign-in screen or the picker screen.

```ts
type AuthStatus = {
  signedIn: boolean
  email?: string        // present only if signedIn
}
```

### `Auth.SignIn() -> AuthStatus`
Starts the OAuth loopback flow (research.md §1): opens the system browser,
waits for the redirect, exchanges the code, persists the encrypted session.
Resolves once the flow completes or is cancelled/denied.

- On success: `{ signedIn: true, email }`.
- On user cancel/deny: `{ signedIn: false }` — no partial session (Edge
  Case). Not an error/rejection; a valid, expected outcome.
- On unrecoverable failure (e.g. OS keychain unavailable — research.md §4
  fallback): rejects with a message safe to show verbatim in the UI.

### `Auth.SignOut() -> void`
Revokes the Google OAuth grant server-side (POST to Google's token-revocation
endpoint with the stored refresh token — FR-003) and clears the persisted
session (deletes the Account row entirely, not a flag — data-model.md). If
the revoke call itself fails (e.g. network error), local deletion still
proceeds — the user is never left "signed in" locally against a dead grant.
After this resolves, `GetStatus()` MUST report `signedIn: false`.

### `Files.PickLocal() -> LocalFileRef | null`
Opens the native OS file-picker dialog (Wails runtime dialog, not a custom
UI). Returns `null` if the user cancels the dialog.

```ts
type LocalFileRef = {
  path: string
  name: string
  sizeBytes: number
}
```

### `Drive.ListFolders(parentId: string | null) -> DriveFolder[]`
Lists child folders of `parentId`; `null`/omitted means the Drive root ("My
Drive" — Acceptance Scenario 2 of User Story 2). Backing call: Drive API
`Files.List` filtered to folders (research.md §2). Requires a signed-in
session; rejects with an auth error if called while signed out (FR-001).

```ts
type DriveFolder = {
  id: string        // "root" for My Drive
  name: string
  hasChildren: boolean   // lets the UI show an expand affordance without an extra round trip
}
```

### `Upload.Start(localPath: string, driveFolderId: string) -> UploadId`
Requires a signed-in session; rejects with a distinct auth error (not a
generic upload failure) if called while signed out (FR-001) — the frontend
must surface this as "sign in first," not as an upload failure message.
Given a signed-in session, validates the local file still exists (re-stats
it — FR-011) and, if so, creates an `Upload` row (`pending` → `in_progress`)
and begins the non-resumable transfer (research.md §3). Rejects immediately,
before creating any `Upload` row, if the file can no longer be found — the
frontend is expected to show the "choose again" messaging from Acceptance
Scenario 3 of User Story 2 in that case.

```ts
type UploadId = number   // Upload.id from data-model.md
```

### `Upload.GetStatus(id: UploadId) -> UploadStatus`
Point-in-time read, for reconnecting the UI after a reload; live updates
during an active upload arrive via the `upload:progress` event instead of
polling this.

```ts
type UploadStatus = {
  status: "pending" | "in_progress" | "succeeded" | "failed"
  bytesSent: number
  totalBytes: number
  driveFileLink?: string   // present only when status === "succeeded"
  failureReason?: string   // present only when status === "failed"
}
```

## Emitted events (backend pushes to frontend)

### `auth:changed`
Payload: `AuthStatus` (same shape as `Auth.GetStatus`'s return). Emitted
whenever sign-in completes, sign-out completes, or a session is force-cleared
because a refresh token was revoked/expired and silent renewal failed (Edge
Case: re-auth mid-session, including mid-upload).

### `upload:progress`
Payload: `{ id: UploadId, bytesSent: number, totalBytes: number }`. Emitted
at least once every 5 seconds during an `in_progress` upload (SC-004),
throttled to ~1/second internally (research.md §3) so the frontend always
has a fresher-than-required signal.

### `upload:complete`
Payload: `{ id: UploadId, driveFileLink: string }`. Terminal event for a
successful upload — corresponds to the `succeeded` status and FR-008's
"reference to the uploaded file's location in Drive."

### `upload:failed`
Payload: `{ id: UploadId, reason: string }`. Terminal event for any failure
path: connectivity loss (Acceptance Scenario 4 of User Story 2),
re-auth-required mid-upload (Edge Case), or any other transfer error.
Corresponds to FR-009's "MUST NOT leave the user in an indefinite or
ambiguous waiting state" — every `in_progress` upload MUST eventually reach
either `upload:complete` or `upload:failed`, never neither.
