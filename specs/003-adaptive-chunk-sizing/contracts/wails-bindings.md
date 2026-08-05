# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends `specs/002-resumable-upload-engine/contracts/wails-bindings.md`,
which itself extends Feature 001's. Every bound method, event, and DTO
shape from those two documents is **unchanged** by this feature — no new
method, no new event, no new field, on the wire. This document exists only
to record the one internal behavioral note worth calling out explicitly,
per research.md §5.

## `Upload.ConfirmRestart(id: UploadId) -> void` (internal behavior changed, signature unchanged)

Still only valid when the given upload's status is `awaiting_confirmation`
(FR-010, unchanged). Still clears `session_uri`, resets `bytes_sent` to 0
and `content_hash_state` to null, initiates a brand-new Drive resumable
session, and transitions the row back to `in_progress`.

**What changes**: the upload's `chunk_size_bytes` and
`consecutive_chunk_successes` (data-model.md — not part of Feature 002's
model) are **no longer implicitly reset** by this call, because they never
existed before this feature. The restarted transfer's first chunk uses the
size the upload had earned before the interruption, not the baseline
(spec Clarifications, FR-009) — regardless of which
`awaitingConfirmationReason` (`session_expired` or `file_changed`)
triggered the restart. This is invisible on the wire: the method's
signature, its precondition, and its return type are all identical to
Feature 002's.

## Everything else

`Upload.Start`, `Upload.GetStatus`, `Upload.GetRecoverable`,
`Upload.Cancel`, and all five `upload:*` events keep the exact shapes
Feature 002 defined. In particular, `UploadStatus` and
`RecoverableUpload` gain **no** chunk-size field — this is a deliberate
scope decision (research.md §7), not an oversight: chunk size is not a
user-facing setting (FR-011) and the spec introduces no new UI (spec
Assumptions).
