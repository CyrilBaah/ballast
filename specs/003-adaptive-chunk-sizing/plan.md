# Implementation Plan: Adaptive Chunk-Size Tuning

**Branch**: `003-adaptive-chunk-sizing` | **Date**: 2026-08-04 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-adaptive-chunk-sizing/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Replace Feature 002's fixed 8 MiB chunk size with an AIMD-style adaptive
policy: after 3 consecutive Drive-acknowledged chunks, double the chunk size
(up to a 64 MiB ceiling); on any retried chunk failure, halve it immediately
(down to a 1 MiB floor), resetting the success streak either way. All sizes
stay multiples of 256 KiB. The chunk-size/success-streak state is persisted
alongside the existing session/offset checkpoint so a resumed upload —
whether from a brief pause, a crash, or a Drive session that expired during
the interruption — continues at the size it had earned rather than
re-ramping from baseline (spec Clarifications). No new user-facing surface
is introduced; the only observable effect is upload throughput.

## Technical Context

**Language/Version**: Go 1.22+ (backend/engine, per Constitution Principle I); TypeScript (Wails "vanilla-ts" frontend) is untouched — this feature has no UI surface (spec Assumptions)

**Primary Dependencies**: None beyond Feature 002's existing set (`google.golang.org/api/drive/v3` + `golang.org/x/oauth2`, `modernc.org/sqlite`, Wails v2) — the adaptive policy is plain Go arithmetic on top of the existing resumable-chunk loop, no new library is needed

**Storage**: SQLite (same file), extending the existing `upload` table with two columns — `chunk_size_bytes`, `consecutive_chunk_successes` (data-model.md) — via the same additive-migration pattern Feature 002 used for its own new columns, rather than a new table

**Testing**: Go `testing` package — a new table-driven suite for the growth/shrink policy in isolation (pure arithmetic, no I/O), plus extending the existing `fakeResumableServer` (research.md §6) to record each accepted chunk's byte length so upload-loop tests can assert the exact growth/shrink sequence and the 256 KiB-multiple invariant end-to-end; Playwright is not extended (no UI surface to test)

**Target Platform**: Desktop — macOS, Windows, Linux (unchanged; Constitution Principle VII)

**Project Type**: desktop-app (single Wails project, unchanged structure)

**Performance Goals**: ≥15% faster completion than a fixed-8-MiB-chunk upload for a 1 GB+ file over a stable connection (SC-001); no more than 5% of transferred bytes re-sent under a degraded (5% loss / 500ms RTT) connection (SC-002) — both validated against the same network-simulation harness Feature 002 established (Constitution Principle III)

**Constraints**: Chunk size never leaves `[1 MiB, 64 MiB]` and always stays a 256 KiB multiple except the file's final chunk (FR-004/FR-006/FR-007); memory use stays flat regardless of how large the chunk size has grown (SC-004) — achieved by sizing the reusable read buffer to the 64 MiB ceiling once per upload rather than reallocating on every resize; chunks remain strictly sequential, one in flight at a time, never concurrent for one file (Constitution Principle II, unchanged); no user-facing chunk-size configuration (FR-011)

**Scale/Scope**: Single user, single Google account, one upload at a time (unchanged) — this feature only changes how that one upload's chunk size varies over time, not the concurrency model

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS** | No new language, datastore, or major dependency — the AIMD policy is a small pure-Go type alongside the existing `BackoffPolicy`, and its two new fields are additional columns on the same SQLite `upload` table. |
| II. Protocol Correctness Over Cleverness | **PASS, directly exercised** | Chunk size changes between chunks, never within one — chunks are still sent strictly in order, one at a time, in 256 KiB multiples (FR-007), and cross-file concurrency remains out of scope (spec Assumptions). No design here introduces parallel or out-of-order sends. |
| III. Test-First for the Upload Engine (NON-NEGOTIABLE) | **PASS, directly exercised** | This is the feature Principle III's "adaptive chunk-sizing algorithms" clause explicitly anticipates. The growth/shrink constants (3-success threshold, ×2/÷2, 1 MiB/64 MiB bounds) are the same starting-hypothesis numbers Feature 002's research.md already flagged as pending harness validation — this feature is what validates them. Tests are written against the policy type and the fake server before/alongside implementation at the task level. |
| IV. Security by Default | **PASS, N/A** | No new secret or credential surface — the two new columns hold plain integers, not session/credential data. |
| V. Simplicity & Bounded Scope | **PASS** | No user-facing chunk-size setting (FR-011), no cross-file concurrency, no new Wails-bound methods or events, no UI change — chunk size is not even exposed for display (research.md §7). The only new production type is one small policy struct mirroring the existing `BackoffPolicy` shape. |
| VI. Reliability Gates as Acceptance Criteria | **PASS, directly exercised** | SC-001/SC-002 are drawn directly from the constitution's throughput/degraded-network reliability gates; SC-004's flat-memory requirement is the same "no unbounded memory growth" gate Feature 002 already satisfies, now re-verified against a chunk size that can grow 8× larger. |
| VII. Cross-Platform Parity | **PASS, N/A** | No OS-specific integration is introduced. |

No violations requiring Complexity Tracking justification.

## Project Structure

### Documentation (this feature)

```text
specs/003-adaptive-chunk-sizing/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── drive/
│   ├── chunksize.go       # NEW: ChunkSizePolicy (growth/shrink/reset) + the
│   │                       # baseline/floor/ceiling/threshold constants — research.md §1
│   ├── resumable.go        # unchanged protocol primitives; loses the old fixed
│   │                       # ChunkSize constant (replaced by chunksize.go's baseline)
│   ├── upload.go            # extended: reusable read buffer sized to the ceiling
│   │                       # (research.md §2), ResumeState carries ChunkSize/
│   │                       # ConsecutiveSuccesses, growth hook on each ack,
│   │                       # shrink hook on each retryable chunk failure only
│   │                       # (research.md §3), OnChunkAcked callback extended
│   └── testharness_test.go  # extended: fakeResumableServer records each
│                           # accepted chunk's length (research.md §6)
├── storage/
│   ├── schema.go            # extended: two new upload columns via a second,
│   │                       # chained additive migration (research.md §4)
│   └── upload.go            # extended: Upload struct + CreateUpload default +
│                           # UpdateUploadProgress + ResetUploadForRestart
│                           # (which now preserves the two new columns —
│                           # research.md §5)

app.go                     # extended: runUpload's OnChunkAcked persists the new
                           # state; UploadGetRecoverable and UploadConfirmRestart
                           # carry ChunkSize/ConsecutiveSuccesses into ResumeState
                           # instead of always starting fresh
```

**Structure Decision**: Same single Wails project as Features 001/002 — this
feature is additive within the existing `internal/drive` and
`internal/storage` packages (one new file, `chunksize.go`, for the genuinely
new policy concern) plus targeted edits to `upload.go` in both packages and
`app.go`'s orchestration. No new package, table, or top-level directory; no
frontend change at all (no UI surface, per spec Assumptions).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Not applicable — see Constitution Check above.

## Post-Design Constitution Check (re-evaluated after Phase 1)

Per the constitution's Compliance Review requirement, the gate above was
re-checked against the concrete design in data-model.md and
contracts/wails-bindings.md:

- **I. Stack Discipline**: data-model.md's schema change is two additional
  columns on the existing `upload` table via the same migration mechanism
  Feature 002 introduced; no new library surfaced during design. **PASS**.
- **II. Protocol Correctness Over Cleverness**: data-model.md's state
  machine confirms chunk size is read once per chunk-send attempt and never
  changes mid-flight; the single-session, single-offset structure that
  already prevented concurrent chunks in Feature 002 is unchanged. **PASS**.
- **III. Test-First for the Upload Engine**: research.md §1's policy is
  concrete enough (exact thresholds, bounds, and reset rule) to write
  failing table-driven tests against directly before implementation,
  deferred to `/speckit-tasks` task breakdown. **PASS** (gate satisfied at
  design level; sequencing enforced at task/implementation time).
- **IV. Security by Default**: confirmed — data-model.md's two new columns
  are plain `INTEGER`s, no secret material. **PASS**.
- **V. Simplicity & Bounded Scope**: contracts/wails-bindings.md confirms
  zero new bound methods, events, or DTO fields — chunk size stays fully
  internal to the backend. **PASS**.
- **VI. Reliability Gates as Acceptance Criteria**: quickstart.md's
  scenarios map directly to SC-001 through SC-005. **PASS**.
- **VII. Cross-Platform Parity**: unchanged — no OS-specific surface added
  during design. **PASS, N/A**.

No new violations surfaced during design; Complexity Tracking remains empty.
