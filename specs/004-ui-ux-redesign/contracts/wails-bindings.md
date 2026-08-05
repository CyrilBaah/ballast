# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends `specs/002-resumable-upload-engine/contracts/wails-bindings.md`.
`Auth.SignOut`, `Files.*`, `Drive.ListFolders`, `Upload.GetStatus`,
`Upload.GetRecoverable`, `Upload.ConfirmRestart`, `Upload.Cancel`, and all
emitted events are unchanged and not repeated here. This document covers
only what this feature changes or adds.

## Bound methods (frontend calls backend)

### `Auth.GetStatus() -> AuthStatus` / `Auth.SignIn() -> AuthStatus` (payload extended)

```ts
type AuthStatus = {
  signedIn: boolean
  email?: string
  name?: string          // NEW — Google display name (research.md §7); absent if Google returned none
  pictureUrl?: string    // NEW — Google profile photo URL (research.md §7); absent if Google returned none
}
```

Both new fields are populated from the same userinfo response Feature
001's sign-in flow already fetches, once the additional `profile` OAuth
scope (research.md §7) is granted — no new call, no new consent step
beyond the one-time broader scope grant. Either field can be absent
(older accounts before this migration, or Google simply not returning
one); the sidebar (FR-011) falls back to `email` for the name line and a
generated-initials avatar when `pictureUrl` is absent or fails to load.

### `Drive.GetStorageQuota() -> StorageQuota` (NEW)

```ts
type StorageQuota = {
  usageBytes: number
  limitBytes?: number   // absent = unlimited-storage account (data-model.md)
}
```

Calls Drive's `about.get` once per invocation (research.md §8) — the
frontend calls this once per session (on reaching the picker/sidebar
after sign-in resolves) and does not re-poll it. Requires no new OAuth
scope (`drive.metadata.readonly`, already granted since Feature 001,
covers it). Rejects the same way other `Drive.*` calls do if called
while signed out.

If this call rejects for any other reason (network error, transient
Drive API failure), the frontend MUST catch it and omit the storage
indicator from the sidebar for that session — no error state, no retry
affordance, and the name/photo portion of the sidebar renders
unaffected (spec.md FR-012, Clarifications 2026-08-05).

### `Upload.Start(localPath: string, driveFolderId: string, driveFolderName: string) -> UploadId` (signature extended)

One new parameter, `driveFolderName` — the destination folder's display
name, already available to the picker screen from its breadcrumb state
(the same value already shown in the picker UI today; no new lookup is
required to obtain it). Persisted onto the new `Upload.drive_folder_name`
column (data-model.md) for the history list; it has no effect on upload
behavior, retry logic, or session handling (FR-010) — display data only.

Passing an empty string is valid (mirrors the root "My Drive" case,
`driveFolderId === ""`) and is displayed as `"My Drive"` by the same
fallback `Upload.ListRecent` applies server-side (data-model.md).

### `Upload.ListRecent() -> UploadListItem[]` (NEW)

Returns up to 50 uploads (data-model.md's `ListRecentUploads`), most
recent first by `started_at`, spanning every terminal and non-terminal
status. Read-only; does not affect FR-013's single-active-upload
constraint or any upload's state.

```ts
type UploadListItem = {
  id: UploadId
  fileName: string
  driveFolderName: string        // "My Drive" fallback if never recorded (data-model.md)
  status: "pending" | "in_progress" | "paused" | "awaiting_confirmation"
        | "cancelled" | "succeeded" | "failed"
  bytesSent: number
  totalBytes: number
  driveFileLink?: string          // present only when status === "succeeded"
  failureReason?: string          // present only when status === "failed"
  startedAt: string                // ISO 8601
}
```

Called once when the history screen mounts, for the initial snapshot
(research.md §5). The frontend is responsible for keeping entries current
afterward by subscribing to the existing `upload:progress` /
`upload:complete` / `upload:failed` / `upload:paused` /
`upload:awaiting-confirmation` events and matching by `id` — this method
is not re-polled.

## Emitted events (backend pushes to frontend)

No new events. The history screen (`frontend/src/screens/history.ts`)
subscribes to the same events `progress.ts` already consumes
(`specs/002-resumable-upload-engine/contracts/wails-bindings.md`'s Emitted
events section), updating whichever `UploadListItem` row matches the
event's `id`. An event for an `id` not present in the current list (e.g.
an upload started after the list's initial snapshot) is prepended as a
new row rather than ignored.
