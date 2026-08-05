# Data Model: Full Experience UI/UX Redesign

Extends Feature 002's `upload` table (`specs/002-resumable-upload-engine/data-model.md`)
and Feature 001's `account` table (`specs/001-auth-picker-upload/data-model.md`)
in place, each with display-only columns. This feature also introduces a
purely presentational entity (Visual Design Tokens) and a live,
non-persisted view entity (Storage Quota) — documented here for
completeness since spec.md names all of these as Key Entities.

## Entity: Upload (extended)

| Field | Type | Notes |
|---|---|---|
| *(all Feature 002 fields)* | — | unchanged — see `specs/002-resumable-upload-engine/data-model.md` |
| `drive_folder_name` | TEXT, nullable | **NEW.** The destination folder's display name (e.g. `"My Drive"` or a subfolder name), captured from the picker's already-in-memory breadcrumb state at the moment `Upload.Start` is called (research.md §3). Nullable only for rows created before this migration ran; always populated for new uploads going forward. Not a live link to Drive — purely a label for the history list (FR-008); staleness if the folder is later renamed in Drive is acceptable, per research.md §3. |

### Schema (additive migration over Feature 002's `upload` table)

```sql
ALTER TABLE upload ADD COLUMN drive_folder_name TEXT;
```

`ensureSchema()` stays idempotent; this single `ALTER TABLE` runs once,
guarded the same way Feature 002 guards its own additive columns, so an
existing Feature-002-shape database upgrades in place without data loss.
No `status` or other constraint changes — this feature adds no new
states, only a new label field.

### New read path: `ListRecentUploads`

Returns Upload rows ordered by `started_at DESC`, capped at a fixed limit
(50 — a reasonable default for "recent" per spec.md's Assumptions; not a
user-configurable or paginated value in this feature's scope). Used only
by the new `Upload.ListRecent` binding (contracts/wails-bindings.md); no
existing read path changes.

## View Entity: UploadListItemDTO (frontend-facing projection, not a table)

The shape returned by `Upload.ListRecent`, one entry per Upload row — see
contracts/wails-bindings.md for the exact JSON shape. Derived fields:

| Field | Derived from | Notes |
|---|---|---|
| `fileName` | `local_path` | Basename only (`filepath.Base`), matching how `progress.ts` already derives the display name from `LocalFileRef.name` in Feature 001. |
| `driveFolderName` | `drive_folder_name` | Falls back to `"My Drive"` when null (pre-migration rows, or the root folder case where the picker's breadcrumb name is already `"My Drive"`). |
| `status` | `status` | Same enum as Feature 002's `UploadState` — the history screen maps each value to one of the four visual states from FR-002 (idle/loading/error/success), collapsing `paused`/`awaiting_confirmation` into the "in progress / needs attention" visual family alongside `in_progress`. |
| `bytesSent` / `totalBytes` | `bytes_sent` / `local_size_bytes` | unchanged meaning from Feature 002. |
| `failureReason` | `failure_reason` | Surfaced only when `status = failed`, per FR-008. |

`fileType` (doc/pdf/image/audio/archive/generic, for the row's icon
accent color) is deliberately *not* one of these fields — it's derived
client-side in `history.ts` from `fileName`'s extension, per
contracts/design-tokens.md's File-type accent tokens. Purely cosmetic
categorization has no reason to round-trip through the backend.

## Entity: Account (extended)

| Field | Type | Notes |
|---|---|---|
| *(all Feature 001 fields)* | — | unchanged — `google_user_id`, `email`, encrypted tokens, etc. |
| `display_name` | TEXT, nullable | **NEW.** Google's `name` claim, captured at sign-in alongside `email` (research.md §7). Nullable for pre-migration accounts and the rare case Google returns no name — the frontend falls back to `email` for the sidebar's name line when null. |
| `picture_url` | TEXT, nullable | **NEW.** Google's `picture` claim — a URL, loaded directly by the frontend via `<img>`, not downloaded/cached locally (research.md §7). Nullable whenever Google returns none, or for pre-migration accounts; the frontend renders a generated-initials avatar in that case. Not a credential — plain TEXT, no encryption, same treatment as the existing unencrypted `email` column. |

### Schema (additive migration over Feature 001's `account` table)

```sql
ALTER TABLE account ADD COLUMN display_name TEXT;
ALTER TABLE account ADD COLUMN picture_url TEXT;
```

Guarded the same idempotent way as this feature's other additive
migrations (data-model.md above, and Feature 002's precedent).

## View Entity: StorageQuota (live, not persisted)

| Field | Source | Notes |
|---|---|---|
| `usageBytes` | Drive `about.get().storageQuota.usage` | Bytes currently used across the account's entire Drive, not just Ballast-uploaded files. |
| `limitBytes` | Drive `about.get().storageQuota.limit` | Total bytes available on the account's plan. Google omits this field entirely for unlimited-storage accounts — the frontend treats an absent `limitBytes` as "unlimited" (hide the usage bar, show used bytes only) rather than dividing by zero. |

Fetched fresh via `Drive.GetStorageQuota` each session (research.md §8);
never written to SQLite.

## Entity: Visual Design Tokens (presentational, no storage)

Not a database entity — the shared vocabulary every screen's CSS draws
from, defined once in `frontend/src/styles/tokens.css` (contracts/design-tokens.md
has the exact property names/values). Listed here because spec.md names
it as a Key Entity:

| Token category | Examples | Notes |
|---|---|---|
| Color roles | `--color-bg`, `--color-text`, `--color-accent`, `--color-error`, `--color-success`, `--color-warning` | Each has a light and a dark value, switched via `prefers-color-scheme` (research.md §2) — not a new persisted setting. |
| Typography scale | `--font-size-body`, `--font-size-heading`, `--font-family` | Consolidates the current ad hoc font-size values scattered across `app.css`. |
| Spacing scale | `--space-xs` … `--space-xl` | Consolidates the current ad hoc `rem` values (`0.4rem`, `0.6rem`, `1.5rem`, …) into a consistent step scale. |
| State treatment | `--color-error`, `--color-success`, `--color-warning` (reused from color roles) | The single source every screen's loading/error/success styling (FR-002) points to, replacing today's per-screen hardcoded hex values (e.g. `#ff6b6b`, `#2f9e44`, `#ffb020` in `app.css`). |
