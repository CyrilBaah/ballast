# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends `specs/001-auth-picker-upload/contracts/wails-bindings.md`. `Auth.*`,
`Files.*`, and `Drive.*` are unchanged and not repeated here. This document
covers only what this feature changes or adds under `Upload.*` and the
events namespace.

## Bound methods (frontend calls backend)

### `Upload.Start(localPath: string, driveFolderId: string) -> UploadId` (behavior changed)

Signature is unchanged from Feature 001. Internally, this now initiates a
Drive resumable session (research.md §1) instead of a single-request
upload, and creates the `Upload` row with `local_mtime` populated alongside
`local_size_bytes` (data-model.md). Still rejects before creating any row if
the local file can't be found, and still requires a signed-in session.

Still rejects up front (FR-013, unchanged from Feature 001) if another
upload is already `in_progress`, `paused`, or `awaiting_confirmation` —
the check now spans all three non-terminal states, not just `in_progress`.

### `Upload.GetStatus(id: UploadId) -> UploadStatus` (payload extended)

```ts
type UploadStatus = {
  status: "pending" | "in_progress" | "paused" | "awaiting_confirmation"
        | "cancelled" | "succeeded" | "failed"
  bytesSent: number       // now specifically Drive-acknowledged bytes (data-model.md)
  totalBytes: number
  driveFileLink?: string        // present only when status === "succeeded"
  failureReason?: string        // present only when status === "failed"
  awaitingConfirmationReason?: "session_expired" | "file_changed"
                                 // present only when status === "awaiting_confirmation"
}
```

`paused`, `awaiting_confirmation`, and `cancelled` are all new possible
values. The frontend MUST render `paused` as "still in progress" copy
(FR-007 — never as a failure state), and `awaiting_confirmation` with the
specific reason-driven prompt from FR-010 (not as a generic failure
either). `cancelled` is a clean terminal state distinct from `failed` — it
reflects a deliberate user action (FR-014), not an error, and carries no
`failureReason`.

`failureReason` on a `failed` upload now also covers "the destination
folder no longer exists" (a deleted/inaccessible Drive parent folder) in
addition to Feature 001's original failure reasons — this is a terminal,
not-recoverable outcome, distinct from `awaiting_confirmation`'s two
recoverable conditions, since restarting the same logical upload can't fix
a destination that's gone.

### `Upload.GetRecoverable() -> RecoverableUpload | null` (NEW)

Called once on app startup, after `Auth.GetStatus()`. Returns the single
non-terminal upload left over from a previous run (research.md §7), if any —
`null` if there is none (the common case: no interrupted upload, or none has
ever existed).

```ts
type RecoverableUpload = {
  id: UploadId
  localPath: string
  fileName: string
  status: "in_progress" | "paused" | "awaiting_confirmation"
        // never "pending"/"succeeded"/"failed" — those aren't recoverable
  bytesSent: number
  totalBytes: number
  awaitingConfirmationReason?: "session_expired" | "file_changed"
}
```

If the returned upload's `status` is `in_progress` or `paused`, the backend
has already begun resuming it in the background by the time this call
resolves (research.md §7 — silent resume, no confirmation needed for a
same-transfer continuation per the spec's Assumptions); the frontend should
route straight to the progress screen and let `upload:progress` events drive
it. If `status` is `awaiting_confirmation`, the frontend must prompt using
`awaitingConfirmationReason` before anything resumes (FR-010).

### `Upload.ConfirmRestart(id: UploadId) -> void` (NEW)

Only valid when the given upload's status is `awaiting_confirmation`
(FR-010's explicit confirmation gate). Clears `session_uri`, resets
`bytes_sent` to 0 and `content_hash_state` to null, initiates a brand-new
Drive resumable session, and transitions the row back to `in_progress`,
restarting the transfer from byte 0 (data-model.md's state diagram).
Rejects if the upload is not currently `awaiting_confirmation` (e.g. called
twice, or called on an upload that isn't this one).

### `Upload.Cancel(id: UploadId) -> void` (NEW)

Only valid when the given upload's status is `paused` or
`awaiting_confirmation` (FR-014) — rejects otherwise (e.g. on an
`in_progress`, already-terminal, or unknown upload). Makes a best-effort
attempt to release the Drive resumable session (research.md §8), then
transitions the row directly to `cancelled`, freeing FR-013's single-active-
upload slot for a new `Upload.Start` call. No event is emitted — the caller
already has the outcome from this call's resolution.

## Emitted events (backend pushes to frontend)

### `upload:progress` (unchanged)
Payload: `{ id: UploadId, bytesSent: number, totalBytes: number }`.
`bytesSent` is now specifically the Drive-acknowledged offset. Emitted at
least once every 5 seconds during `in_progress` (SC-004, unchanged from
Feature 001), and also whenever a `paused → in_progress` transition
resumes after a retry succeeds.

### `upload:paused` (NEW)
Payload: `{ id: UploadId, retryingSince: string }` (ISO 8601 timestamp of
when the current retry sequence began). Emitted when a retryable error
(research.md §4) is hit. The frontend MUST show this as "upload paused,
retrying…" — explicitly not a failure state (FR-003, FR-007) — distinct
from `upload:progress` so the UI can show a retry-specific indicator
without waiting for the next successful chunk's progress event.

### `upload:awaiting-confirmation` (NEW)
Payload: `{ id: UploadId, reason: "session_expired" | "file_changed" }`.
Emitted when a terminal-but-recoverable condition is detected
(research.md §4) — the upload stops auto-retrying and the frontend must
prompt the user per FR-010, offering `Upload.ConfirmRestart(id)`.

### `upload:complete` (unchanged)
Payload: `{ id: UploadId, driveFileLink: string }`.

### `upload:failed` (unchanged payload, expanded conditions)
Payload: `{ id: UploadId, reason: string }`. Now also covers the
terminal-not-recoverable conditions from research.md §4 (storage quota
exceeded, permission revoked/signed out, local file missing) in addition to
Feature 001's original failure paths. Every upload still reaches exactly one
of `upload:complete` or `upload:failed` eventually — `upload:paused` and
`upload:awaiting-confirmation` are intermediate, not terminal, states.
