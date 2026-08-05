# Research: Adaptive Chunk-Size Tuning

All items below were left open by the spec's Assumptions (the exact
growth/shrink policy) or are technical unknowns raised by Technical Context.
Each is resolved with a decision, rationale, and alternatives considered.

## 1. Growth/shrink algorithm and constants

**Decision**: A single AIMD-style policy, adopted verbatim from
`Ballast_Project_Problem_Statement.md`'s proposed strawman — the same
document Feature 002's research.md already pulled its fixed 8 MiB starting
value from:

- **Baseline**: 8 MiB (32 × 256 KiB) — every new upload starts here (FR-002).
- **Growth**: on 3 consecutive Drive-acknowledged chunks, double the chunk
  size, up to a **64 MiB ceiling** (FR-003/FR-004).
- **Shrink**: on any chunk attempt that fails in a retried way, halve the
  chunk size immediately, down to a **1 MiB floor** (FR-005/FR-006).
- **Reset**: any size change — growth or shrink — resets the consecutive-
  success counter to 0 (FR-008; the spec's Key Entities wording "since the
  size last changed" already implies this for growth too, not only shrink).
- All resulting sizes are multiples of 256 KiB by construction, since
  baseline/floor/ceiling are themselves 256 KiB multiples and doubling/
  halving preserves that alignment (FR-007).

Implemented as one small type, `ChunkSizePolicy`, in a new
`internal/drive/chunksize.go`, mirroring `retry.go`'s existing
`BackoffPolicy` shape (a struct with `OnSuccess()`/`OnFailure()` methods and
exported constants for the four tunable numbers).

**Rationale**: The spec's Assumptions explicitly defer these exact numbers
to implementation, flagged as "adopted from the project's own problem
statement as a starting policy... expected to be validated and tuned
against real network-simulation testing before being trusted." That is
precisely Constitution Principle III's treatment of empirically-tuned
constants — a starting hypothesis, not a settled default — so this research
decision resolves the *shape* of the algorithm now (needed to write any
code or test at all) while leaving the *validation* of the specific numbers
to the network-simulation harness, per the constitution.

**Alternatives considered**:
- *Additive growth/shrink (fixed step, e.g. +2 MiB/-2 MiB)* — rejected:
  the problem statement's own AIMD framing (borrowed from TCP congestion
  control) uses multiplicative growth specifically so a healthy connection
  reaches its effective ceiling in a handful of round trips rather than
  dozens; multiplicative shrink is the classic AIMD asymmetry that makes
  the system "lean cautious" under flapping conditions per the spec's edge
  case, whereas a fixed-step shrink would take many failures to recover
  from a bad size.
- *A continuously-running counter that doesn't reset on growth* (grow again
  every 3rd success total, not every 3rd success *since the last change*)
  — rejected: contradicts the spec's Key Entities wording directly ("since
  the size last changed") and would let the size race to the ceiling
  faster than intended after the first growth step.

## 2. Reusable read buffer sized to the ceiling, not the current chunk size

**Decision**: `UploadFile`'s per-upload read buffer is allocated once, at
`MaxChunkSize` (64 MiB), for the lifetime of one `UploadFile` call. Each
read slices `buf[:currentChunkSize]` rather than reallocating a
differently-sized buffer on every growth or shrink step.

**Rationale**: SC-004 requires memory use to "stay flat regardless of how
large the chunk size has grown." A single fixed-size allocation reused via
slicing satisfies this trivially and avoids GC churn from resizing on every
AIMD step; the worst case is one 64 MiB buffer per active upload, and
FR-013 (Feature 002, unchanged) already limits the app to one active
upload at a time, so this is a small, bounded, one-time cost regardless of
file size — consistent with the existing streaming design (no whole-file
buffering).

**Alternatives considered**:
- *Reallocate a new `[]byte` of exactly `currentChunkSize` on every change*
  — rejected: works correctly but churns the allocator on every growth/
  shrink step for no benefit, since the ceiling-sized buffer is already
  small relative to the files this project targets (hundreds of GB).

## 3. Where growth/shrink hooks plug into the existing chunk loop

**Decision**: In `UploadFile`'s main loop (`internal/drive/upload.go`):
- On a successful `SendChunk` (the branch that currently calls
  `backoff.Reset()`), also call `policy.OnSuccess()` before computing the
  next iteration's read size.
- In `classifyAndMaybeRetry`, only for a chunk-send failure (not a session-
  *initiation* failure — `isSessionInitiation == false`) and only for the
  `Retryable` bucket (the same branch that already flips `*paused` and
  schedules backoff), also call `policy.OnFailure()`.
- A `TerminalNotRecoverable`/`TerminalRecoverable` outcome never reaches
  the shrink call, since `classifyAndMaybeRetry` returns before that point
  for non-`Retryable` buckets — satisfying FR-012 (a non-retried failure
  must not change chunk size) for free, with no extra conditional needed.

**Rationale**: This maps FR-003/FR-005 onto the exact two points in the
existing loop where "a chunk was just acknowledged" and "a chunk attempt
just failed in a retried way" are already distinguished, requiring no new
control flow — only two additional method calls in branches that already
exist. Excluding session-initiation failures matches FR-002's framing
(chunk-size policy governs *chunks*; a session hasn't sent any chunk yet)
and keeps `InitiateSession`'s own retry/backoff behavior (unchanged from
Feature 002) independent of chunk sizing.

**Alternatives considered**:
- *Also shrink on a session-initiation retry* — rejected: there is no
  chunk size in play yet at that point (FR-002's baseline applies to the
  first *chunk*, not the session-open call), and shrinking before the
  first byte is even sent would make the very first chunk of every upload
  that hit a transient initiation hiccup start below baseline for no
  stated reason in the spec.

## 4. Persisting Chunk-Size State (schema/migration)

**Decision**: Two new columns on the existing `upload` table —
`chunk_size_bytes INTEGER NOT NULL DEFAULT 8388608` (the baseline) and
`consecutive_chunk_successes INTEGER NOT NULL DEFAULT 0` — added via a
second, chained additive migration in `internal/storage/schema.go`,
following the exact rename-recreate-copy pattern Feature 002 already
established for its own new columns (guarded by a fresh column-presence
check, e.g. `chunk_size_bytes`'s absence, run after the existing
`local_mtime`-guarded migration).

**Rationale**: SQLite has no `ALTER TABLE ... ADD COLUMN ... DEFAULT
(expr)` limitation issue here (a literal default is fine via plain `ALTER
TABLE ADD COLUMN`, unlike the CHECK-constraint widening Feature 002 needed)
— but chaining a second guarded migration rather than editing the first
one keeps a database that already upgraded through Feature 002's migration
upgrading cleanly to this feature's shape too, without special-casing
"which prior version was this."

**Alternatives considered**:
- *A separate `chunk_size_state` table keyed by upload id* — rejected:
  Chunk-Size State is a 1:1 facet of one upload's row, exactly like
  `bytes_sent`/`session_uri` already are (data-model.md's existing framing
  for Feature 002's own new fields) — a second table would need a join for
  no relational benefit, violating Simplicity (Constitution Principle V).

## 5. `ConfirmRestart` must preserve, not reset, the earned chunk size

**Decision**: `ResetUploadForRestart`'s `UPDATE` — which currently zeroes
`bytes_sent`/`session_uri`/`content_hash_state` on an `awaiting_confirmation
→ in_progress` restart — is **not** extended to touch `chunk_size_bytes` or
`consecutive_chunk_successes`. Both `UploadGetRecoverable`'s silent-resume
path and `UploadConfirmRestart`'s explicit-restart path in `app.go` read
the upload's current `chunk_size_bytes`/`consecutive_chunk_successes` and
carry them into the new `ResumeState` passed to `UploadFile`, rather than
passing a zero-value `ResumeState{}`.

This applies uniformly to both `awaiting_confirmation` reasons —
`session_expired` and `file_changed` — since `ConfirmRestart` is one code
path serving both (data-model.md, Feature 002). The spec's clarification
was stated in terms of session expiry specifically, but FR-009's own
wording ("even when the prior Drive session has expired... is not treated
as a new upload for chunk-size purposes") and the Key Entity's framing (the
chunk-size state is carried "alongside an upload's other resume
information") give no basis to treat a file-identity-triggered restart
differently from a session-expiry-triggered one — both are the same
"restart the byte stream, keep the logical upload" operation from the
chunk-sizing policy's point of view. Splitting them into two behaviors
would be an unrequested distinction (Constitution Principle V).

**Rationale**: This is the concrete mechanism behind the spec's
Clarifications answer ("keep the earned size" on a session-expiry restart).
Since `ConfirmRestart` already resets everything else needed for a
byte-0 restart (session URI, offset, hash), the only change needed is to
*exclude* the two new columns from that reset — a smaller, more surgical
change than adding new restart-specific logic.

**Alternatives considered**:
- *Reset chunk size on `file_changed` restarts only, keep it on
  `session_expired` restarts* — rejected per above: the spec draws no such
  distinction, and introducing one would be speculative scope the spec
  never asked for.

## 6. Test harness extension

**Decision**: `fakeResumableServer` (research.md §6 of Feature 002) gains a
recorded slice of each accepted chunk's byte length (in addition to the
existing scalar `wireBytes` total), exposed via a new
`acceptedChunkSizes() []int64` accessor. This lets a Go test assert the
exact AIMD sequence a run produced (e.g. `8, 8, 8, 16, 16, 16, 32, ...`
MiB on an all-success run, or a size roughly halving on injected
`network-fail`/`429` outcomes) and that every recorded size is a 256 KiB
multiple except the last.

**Rationale**: `wireBytes`'s existing scalar total is enough to verify "no
re-transmission of acknowledged bytes" (Feature 002's concern) but cannot
distinguish *how* that total was split into chunks, which is exactly what
this feature's acceptance scenarios need to verify (growth up to ceiling,
shrink down to floor, asymmetric recovery after a shrink).

**Alternatives considered**:
- *Only test `ChunkSizePolicy` in isolation, not through the fake server*
  — rejected as the sole approach: unit tests on the policy type alone
  (research.md §1) are necessary but not sufficient — they don't prove the
  hooks in §3 are wired into the real chunk loop correctly (e.g. that a
  shrink actually causes a smaller `Content-Length` on the wire); both
  levels of test are needed, not one instead of the other.

## 7. Rejected: exposing chunk size to the frontend

**Decision**: No new Wails-bound method, event, or DTO field surfaces the
current or historical chunk size to the UI in any form (not even
read-only/diagnostic).

**Rationale**: FR-011 rules out chunk size as something a user configures,
and the spec's Assumptions state the feature introduces no new UI and that
upload speed is the *only* user-visible effect. Adding a read-only display
would be scope the spec never asked for (Constitution Principle V) and
would create a second thing to keep in sync with the backend's internal
state for zero requested benefit.

**Alternatives considered**:
- *Add a diagnostic-only field to `upload:progress` or `UploadStatus`
  "for future debugging"* — rejected: speculative, unrequested surface;
  if observability into chunk-size behavior is ever needed, it belongs in
  structured logs (`internal/logging`), not a user-facing contract, and
  isn't required by anything in this spec.
