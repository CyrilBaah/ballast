# Data Model: Adaptive Chunk-Size Tuning

Extends Feature 002's `upload` table
(`specs/002-resumable-upload-engine/data-model.md`) in place — this
feature's Key Entity (Chunk-Size State) is a facet of the same
one-row-per-upload-attempt model, not a new table. `account` is unchanged
and not repeated here.

## Entity: Upload (extended)

| Field | Type | Notes |
|---|---|---|
| *(all Feature 001/002 fields)* | | unchanged — see Feature 002's data-model.md |
| `chunk_size_bytes` | INTEGER | **NEW.** The chunk size currently in use for this upload's next chunk-send attempt. Defaults to the baseline (8 MiB / `8388608`) at creation (FR-002). Always in `[FloorChunkSize, CeilingChunkSize]` (`[1 MiB, 64 MiB]`) and a multiple of 256 KiB (FR-004/FR-006/FR-007 — research.md §1). |
| `consecutive_chunk_successes` | INTEGER | **NEW.** Count of Drive-acknowledged chunks in a row since `chunk_size_bytes` last changed. Defaults to 0 at creation. Reset to 0 on every change to `chunk_size_bytes`, whether growth or shrink (FR-008; research.md §1). |

### Schema (additive migration over Feature 002's `upload` table)

```sql
ALTER TABLE upload ADD COLUMN chunk_size_bytes INTEGER NOT NULL DEFAULT 8388608;
ALTER TABLE upload ADD COLUMN consecutive_chunk_successes INTEGER NOT NULL DEFAULT 0;
```

Applied as a second, independently-guarded migration step (detected via
`chunk_size_bytes`'s absence), chained after Feature 002's existing
`local_mtime`-guarded migration in `ensureSchema()` — research.md §4. A
database that has never run Feature 002's migration runs both in sequence
on first launch after this feature ships; one that already has Feature
002's columns runs only this feature's step.

### State transitions (extends Feature 002's diagram)

Feature 002's `pending → in_progress → {paused, awaiting_confirmation} →
{in_progress, cancelled, failed} → succeeded` state machine is otherwise
unchanged. This feature changes what happens to `chunk_size_bytes` /
`consecutive_chunk_successes` **across** those transitions, not the
transitions themselves:

- **`in_progress` chunk loop** (`bytes_sent` advances on every ack): after
  each acknowledged chunk, `consecutive_chunk_successes` increments; every
  3rd consecutive success, `chunk_size_bytes` doubles (capped at 64 MiB)
  and `consecutive_chunk_successes` resets to 0 (research.md §1/§3).
- **`in_progress → paused`** (a retryable chunk-send failure):
  `chunk_size_bytes` halves immediately (floored at 1 MiB) and
  `consecutive_chunk_successes` resets to 0 (FR-005/FR-008). A
  session-*initiation* retry does not touch either column (research.md
  §3).
- **`paused`/`awaiting_confirmation`/crashed-`in_progress` → resumed
  `in_progress`** (`UploadGetRecoverable`'s silent resume, or app
  relaunch): `chunk_size_bytes`/`consecutive_chunk_successes` are read
  as-is and carried into the resumed transfer's first chunk — **not**
  reset to baseline (FR-009, spec Clarifications).
- **`awaiting_confirmation → in_progress`** (`ConfirmRestart`, either
  reason — `session_expired` or `file_changed`): unlike `bytes_sent`/
  `session_uri`/`content_hash_state`, which reset to their initial/null
  state, `chunk_size_bytes` and `consecutive_chunk_successes` are **left
  untouched** by this transition (research.md §5) — a byte-0 restart of
  the same logical upload is not a "new upload" for chunk-size purposes.
- **Terminal, not-recoverable failure** (`→ failed`) and **user cancel**
  (`→ cancelled`): neither column changes (FR-012) — there is no next
  chunk to size.

**Validation rules** (extending Feature 002's):
- `chunk_size_bytes` MUST always satisfy `1_048_576 ≤ chunk_size_bytes ≤
  67_108_864` and `chunk_size_bytes % 262_144 == 0`.
- `consecutive_chunk_successes` MUST be reset to 0 in the same write that
  changes `chunk_size_bytes` — the two never move independently of that
  invariant (mirrors Feature 002's `bytes_sent`/`content_hash_state`
  same-transaction invariant).
- `chunk_size_bytes` and `consecutive_chunk_successes` are persisted in the
  same atomic write as `bytes_sent`/`session_uri`/`content_hash_state`
  after every acknowledged chunk (data-model.md's existing
  `UpdateUploadProgress` call site), never optimistically before Drive
  acknowledges the chunk.
- `ResetUploadForRestart` (the `awaiting_confirmation → in_progress`
  transition) MUST NOT modify `chunk_size_bytes` or
  `consecutive_chunk_successes`, unlike every other column it resets.

## Entity: Chunk-Size State

Not a separate table — the fields named in the spec's Key Entities
(current chunk size, consecutive-success count) are exactly
`Upload.chunk_size_bytes` and `Upload.consecutive_chunk_successes` above,
carried alongside the same row's `bytes_sent`/`session_uri` per the spec's
framing. Documented here as its own conceptual entity only because the
spec calls it out separately; research.md §1 has the growth/shrink
algorithm that mutates these fields, and §5 has the restart-preservation
rule.
