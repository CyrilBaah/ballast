# Data Model: Resumable, Crash-Safe Upload Engine

Extends Feature 001's `upload` table (`specs/001-auth-picker-upload/data-model.md`)
in place — this feature's Key Entities (Upload Session, Source File
Identity, Error Classification) are facets of the one row-per-upload-attempt
model Feature 001 already established, not new tables. `account` is
unchanged and not repeated here.

## Entity: Upload (extended)

One row per upload attempt — now spanning its full crash-safe lifecycle
(create → resumable session → chunks → pause/retry cycles any number of
times → succeeded/failed/awaiting-confirmation), not just a single
uninterrupted attempt.

| Field | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | unchanged |
| `local_path` | TEXT | unchanged |
| `local_size_bytes` | INTEGER | unchanged; doubles as the cheap-check size baseline (research.md §5) |
| `local_mtime` | DATETIME | **NEW.** Captured via `os.Stat` at creation, alongside `local_size_bytes`; the other half of the cheap-check baseline. |
| `drive_folder_id` | TEXT | unchanged |
| `status` | TEXT | **Extended.** Now `pending`, `in_progress`, `paused`, `awaiting_confirmation`, `cancelled`, `succeeded`, `failed` — see State transitions below. |
| `bytes_sent` | INTEGER | **Redefined.** Now specifically the last *Drive-acknowledged* offset (confirmed via a `308`/chunk-success response or an offset query — research.md §1), not merely bytes handed to the HTTP client. This is the value re-sent-from point on every resume. |
| `session_uri` | TEXT, nullable | **NEW.** The `Location` header returned by Drive's resumable-session-initiate call. Null before the first chunk is sent, or after a restart-from-0 clears it pending a new session. Not treated as a credential (plan.md's Constitution Check, Principle IV note) — plain-text column is sufficient. |
| `content_hash_state` | BLOB, nullable | **NEW.** Serialized `crypto/sha256` digest state (via its `encoding.BinaryMarshaler` support), checkpointed after each acknowledged chunk. Always covers exactly `local_path`'s bytes `[0, bytes_sent)` as of the last checkpoint — see research.md §5. Null before the first chunk is acknowledged. |
| `awaiting_confirmation_reason` | TEXT, nullable | **NEW.** Set only when `status = awaiting_confirmation`; one of `session_expired` or `file_changed` (drives the specific confirmation copy FR-010 requires). |
| `drive_file_id` | TEXT, nullable | unchanged |
| `drive_file_link` | TEXT, nullable | unchanged |
| `failure_reason` | TEXT, nullable | unchanged; set only on `failed` (research.md §4's terminal, not-recoverable rows) |
| `started_at` / `ended_at` | DATETIME | unchanged |

### Schema (additive migration over Feature 001's `upload` table)

```sql
ALTER TABLE upload ADD COLUMN local_mtime DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now'));
ALTER TABLE upload ADD COLUMN session_uri TEXT;
ALTER TABLE upload ADD COLUMN content_hash_state BLOB;
ALTER TABLE upload ADD COLUMN awaiting_confirmation_reason TEXT
    CHECK (awaiting_confirmation_reason IN ('session_expired', 'file_changed') OR awaiting_confirmation_reason IS NULL);

-- status's CHECK constraint is recreated (SQLite has no ALTER ... DROP CONSTRAINT)
-- to widen the allowed set: pending, in_progress, paused, awaiting_confirmation,
-- cancelled, succeeded, failed.
```

`ensureSchema()` stays idempotent (`CREATE TABLE IF NOT EXISTS`); the
`ALTER TABLE` statements above run once, guarded the same way Feature 001
guards table creation, so an existing Feature-001-only database upgrades in
place without data loss.

### State transitions

```text
pending ──────────────► in_progress ───────► succeeded
                            │  ▲
              (retryable    │  │ (retry succeeds:
               error hit)   ▼  │  chunk sent, ack'd)
                          paused
                            │
          (terminal-but-recoverable: session expired,
           or file-identity check fails on resume)
                            ▼
                  awaiting_confirmation
                            │
              (user calls Upload.ConfirmRestart:
               session_uri, bytes_sent, content_hash_state
               all reset to their initial/null state,
               a brand-new Drive session is initiated)
                            ▼
                       in_progress

in_progress / paused ──(terminal, not recoverable: quota exceeded,
                         permission revoked, local file missing,
                         destination folder no longer exists)──► failed

paused / awaiting_confirmation ──(user calls Upload.Cancel:
                                   best-effort DELETE of session_uri,
                                   ignoring its result)──► cancelled
```

**Startup normalization**: a row still in `in_progress` when the app starts
(the process died before it could persist a `paused` transition) is treated
identically to `paused` — the last-checkpointed `bytes_sent` /
`content_hash_state` /`session_uri` are exactly as trustworthy either way,
since both are only ever written after a chunk is Drive-acknowledged. There
is no `in_progress`-specific recovery step beyond this.

**Validation rules** (extending Feature 001's):
- Exactly one `Upload` may be in `in_progress`, `paused`, or
  `awaiting_confirmation` at a time — FR-013's single-active-upload
  constraint now spans all three non-terminal states, not just
  `in_progress`.
- `bytes_sent` and `content_hash_state` are always updated together, in the
  same transaction, only after Drive has acknowledged the corresponding
  chunk — never optimistically before send, never independently of each
  other (research.md §5's correctness invariant: the hash always covers
  exactly the acknowledged prefix).
- `awaiting_confirmation_reason` MUST be non-null iff `status =
  awaiting_confirmation`, and MUST be null for every other status —
  including `cancelled`, which needs no reason since it's a direct user
  action, not a detected condition.
- `cancelled` is reachable only from `paused` or `awaiting_confirmation`
  (FR-014) — never from `in_progress` directly, and never from a terminal
  state (`succeeded`/`failed`/already-`cancelled`). Once `cancelled`, a row
  is terminal like `failed`/`succeeded`: it does not count toward FR-013's
  single-active-upload constraint and is never resumed.
- A `succeeded` row MUST have `bytes_sent = local_size_bytes` and both
  `drive_file_id`/`drive_file_link` populated (unchanged guarantee from
  Feature 001, now additionally implying the resumable session reached
  Drive's final `200`/`201` response).
- Restarting from byte 0 (the `awaiting_confirmation → in_progress`
  transition) reuses the same `Upload` row/id rather than creating a new
  one — unlike Feature 001's `failed` terminal state, this is an explicit,
  user-approved continuation of the same logical upload attempt, not a
  fresh one.

## Entity: Source File Identity

Not a separate table — the fields listed in the spec's Key Entities
(`path`, `size`, `modification time`, `content hash`) are exactly
`Upload.local_path`, `local_size_bytes`, `local_mtime`, and
`content_hash_state` above. Documented here as its own conceptual entity
only because the spec calls it out separately; see research.md §5 for the
cheap-check-then-fallback algorithm that reads these fields.

## Entity: Error Classification

Not persisted as data — a code-level taxonomy applied at the moment an
error occurs, mapping an HTTP status/transport outcome to one of three
buckets (retryable / terminal-recoverable / terminal-not-recoverable), each
driving a specific `Upload.status` transition above. The full table (which
conditions map to which bucket, and the resulting user-visible reason) is
research.md §4 — not duplicated here since it's a behavioral spec, not a
stored entity.
