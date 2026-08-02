# Data Model: Google Sign-In, File/Folder Picker & Basic Upload

Derived from the spec's Key Entities, scoped to what this feature persists.
Nothing here models the resumable/adaptive engine (session offsets, chunk
state, retry counters) — that belongs to a later feature's data model.

## Entity: Account (User Account)

The single authenticated Google identity connected to the app. One row for
the entire feature (single account per installation, per Assumptions).

| Field | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Always `1` in this feature — enforced at the application layer, not a DB constraint, since multi-account is a future feature's concern, not this one's. |
| `google_user_id` | TEXT | Stable Google account identifier (`sub` claim), for detecting "same account re-signed-in" vs. a different account. |
| `email` | TEXT | Display-only, shown in the UI so the user knows which account is connected. |
| `access_token_ciphertext` | BLOB | AES-256-GCM ciphertext; see research.md §4. |
| `access_token_nonce` | BLOB | GCM nonce for `access_token_ciphertext`. |
| `refresh_token_ciphertext` | BLOB | AES-256-GCM ciphertext. |
| `refresh_token_nonce` | BLOB | GCM nonce for `refresh_token_ciphertext`. |

**Implementation note**: split into two nonce columns (one per ciphertext)
rather than the single shared `token_nonce` implied above — a shared column
can't satisfy this same table's "never reuse a nonce across access/refresh
token" rule once there are two independently encrypted values.
| `access_token_expiry` | DATETIME | Used to decide whether a silent refresh is needed before an API call. |
| `created_at` | DATETIME | First sign-in timestamp. |

**Validation rules**:
- No row exists in the signed-out state (FR-003: sign-out clears the row,
  it does not just flip a flag — this guarantees a stale token can never be
  read after sign-out).
- Sign-out MUST call Google's OAuth revocation endpoint with the stored
  refresh token before/while deleting the row (FR-003); local deletion
  proceeds even if the revoke call fails.
- Presence of a valid, non-expired (or refreshable) row is the sole gate for
  FR-001's "file selection, folder browsing, or upload capabilities" access
  check.

**State transitions**: `absent → signed_in` (sign-in success) →
`signed_in → absent` (explicit sign-out, or a revoked/expired refresh token
that can't be silently renewed — Edge Case: re-auth prompt).

## Entity: Local File

Not persisted beyond the in-memory selection for the current upload attempt
— it has no independent lifecycle worth a table. Represented as a
transient value the frontend holds and passes to the backend when starting
an upload.

| Field | Type | Notes |
|---|---|---|
| `path` | string | Absolute local filesystem path from the native file-picker dialog. |
| `name` | string | Derived from `path`; used as the default uploaded filename. |
| `size_bytes` | int64 | Read from the filesystem immediately before upload starts, to catch FR-011 (file moved/renamed/deleted since selection). |

**Validation rules**: Re-stat the file at upload-start time, not just at
selection time (Acceptance Scenario 3 of User Story 2 — the failure must be
caught "before the upload starts," which can be arbitrarily later than
selection).

## Entity: Drive Destination

Also transient — a folder identifier chosen via the custom Drive folder
browser (research.md §2), not stored past the current upload attempt.

| Field | Type | Notes |
|---|---|---|
| `folder_id` | string | Drive folder ID; the well-known value `"root"` represents "My Drive" (Acceptance Scenario 2 of User Story 2). |
| `folder_name` | string | Display-only, for confirmation UI ("Uploading to: <folder_name>"). |

## Entity: Upload

One row per upload attempt, created when the user starts an upload and
updated as it progresses. Persisted (rather than purely in-memory) so a
crash or app restart mid-upload doesn't leave the user without a record of
what happened — but note this feature does not resume the transfer itself,
it only preserves the outcome record.

| Field | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | |
| `local_path` | TEXT | Snapshot of the Local File's path at start time. |
| `local_size_bytes` | INTEGER | Snapshot at start time. |
| `drive_folder_id` | TEXT | Snapshot of the Drive Destination at start time. |
| `status` | TEXT | One of `pending`, `in_progress`, `succeeded`, `failed` (spec's Key Entities). |
| `bytes_sent` | INTEGER | Updated on each progress event; drives the progress indicator (FR-007). |
| `drive_file_id` | TEXT, nullable | Set only on `succeeded`; the reference shown per FR-008. |
| `drive_file_link` | TEXT, nullable | `webViewLink` from the Drive API response, shown to the user on success. |
| `failure_reason` | TEXT, nullable | Set only on `failed`; human-readable cause (network loss, re-auth required, file vanished) per FR-009. |
| `started_at` / `ended_at` | DATETIME | |

**State transitions**: `pending → in_progress → (succeeded | failed)`. No
transition resumes a `failed` upload — FR-010 requires the user to start a
new one manually, which creates a new row rather than mutating the failed
one (preserves the failure record intact).

**Validation rules**:
- Exactly one `Upload` may be `in_progress` at a time (this feature has no
  concurrency — Scale/Scope in plan.md).
- A `succeeded` row MUST have both `drive_file_id` and `drive_file_link`
  populated (SC-003's verifiability requirement); a `failed` row MUST have
  `failure_reason` populated (SC-006's "zero silent failures").
