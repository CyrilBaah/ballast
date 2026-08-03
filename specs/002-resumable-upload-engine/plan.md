# Implementation Plan: Resumable, Crash-Safe Upload Engine

**Branch**: `002-resumable-upload-engine` | **Date**: 2026-08-03 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-resumable-upload-engine/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Replace Feature 001's single-request, non-resumable Drive upload with
Google Drive's resumable upload session protocol: initiate a session, send
the file in ordered 256 KiB-multiple chunks, and persist the session URI,
acknowledged byte offset, and source-file identity to SQLite after every
chunk so an interrupted upload — whether from a dropped connection or a
process crash/power loss/OS restart — resumes from the last acknowledged
byte instead of restarting. Errors are classified as retryable (network
blips, rate limits, 5xx — auto-retried with capped backoff, upload stays
visibly "in progress") or terminal (quota exceeded, permission revoked,
expired session, changed source file, deleted destination folder — retries
stop and the user is told a specific reason within 5 seconds); the two
recoverable-but-not-silently-resumable terminal cases (expired session,
changed file) route to an explicit user confirmation before restarting from
byte 0, per the spec's Clarifications, while a deleted destination folder
is not recoverable at all (the user must start a new upload). The user can
also explicitly cancel a paused or awaiting-confirmation upload at any time
rather than being forced to either resolve it or leave it stuck (FR-014).
The transfer itself streams the file in bounded-size chunks throughout, so
memory use stays flat regardless of file size (FR-012/SC-006).

## Technical Context

**Language/Version**: Go 1.22+ (backend/engine, per Constitution Principle I); TypeScript (Wails "vanilla-ts" frontend, unchanged from Feature 001)

**Primary Dependencies**: Wails v2 (desktop shell, unchanged); `google.golang.org/api/drive/v3` + `golang.org/x/oauth2` (reused for auth/token-refresh and non-upload Drive calls, unchanged from Feature 001); the resumable upload byte stream itself is implemented with the Go standard library's `net/http` directly against Drive's resumable endpoints (research.md §1) rather than the SDK's resumable wrapper; `modernc.org/sqlite` (unchanged); `zalando/go-keyring` (unchanged, no new keychain usage — this feature adds no new secrets beyond the session URI, which is not treated as a credential and does not require keychain-level protection, only the same SQLite-column-encryption Feature 001 already applies to auth tokens)

**Storage**: SQLite (same single local file as Feature 001), extending the existing `upload` table with resumable-session columns (data-model.md) — session URI, local mtime, checkpointed content-hash state — rather than a new table, since these are all facets of the same one-upload-at-a-time entity Feature 001 already models.

**Testing**: Go `testing` package for session/offset/resume logic, retry classification, and the file-identity check (Constitution Principle III, NON-NEGOTIABLE for this exact code path), backed by a new `httptest`-based fake resumable-upload server extending the existing `mock_e2e.go` pattern (research.md §6) to simulate dropped connections, 429/5xx, and expired sessions; Playwright for the three user-story flows end-to-end, reusing Feature 001's `wails dev` + network-boundary-mocked CI setup; GitHub Actions CI across macOS/Windows/Linux (unchanged).

**Target Platform**: Desktop — macOS, Windows, Linux (unchanged from Feature 001; Constitution Principle VII)

**Project Type**: desktop-app (single Wails project, unchanged structure from Feature 001)

**Performance Goals**: Resume within 2 seconds of connectivity returning for the common brief-drop case (SC-001, research.md §4); terminal-failure reasons surfaced within 5 seconds of detection (SC-004); progress reporting driven by the same throttled-emit pattern Feature 001 already uses (`internal/drive/progress.go`), now anchored to acknowledged (not merely sent) bytes.

**Constraints**: No reading a whole file into memory at any point, including for the file-identity check (FR-012, SC-006 — research.md §5's incremental hash checkpoint is what makes this possible for the identity check specifically); exactly one active upload at a time, matching Feature 001 (FR-013); chunks sent strictly in order, one at a time, never concurrently for a single file (Constitution Principle II — non-negotiable, not merely a performance choice); no adaptive chunk-size/concurrency logic in this feature (spec Assumptions — fixed 8 MiB chunk size, research.md §2).

**Scale/Scope**: Single user, single Google account, one file/one upload at a time (unchanged from Feature 001) — but now files "ranging from small documents to hundreds of gigabytes" (FR-012), which Feature 001 didn't need to handle robustly since its upload was already all-or-nothing per attempt.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS** | No new backend language, no new datastore, no new major dependency — the resumable protocol is implemented with the Go standard library (`net/http`) against the existing oauth2-authenticated client, and persisted in the same SQLite database via the same `modernc.org/sqlite` driver. |
| II. Protocol Correctness Over Cleverness | **PASS, directly exercised** | This is the feature this principle exists for. Chunks are sent strictly in order, one at a time, in 256 KiB multiples (research.md §2); no parallel/out-of-order chunk sending for one file is introduced anywhere in the design. Cross-file concurrency isn't in scope either (FR-013: one upload at a time). |
| III. Test-First for the Upload Engine (NON-NEGOTIABLE) | **PASS, directly exercised** | Session/offset/resume logic, retry classification, and the file-identity check all get Go tests against the new fake resumable-server harness (research.md §6), written before/alongside implementation at the task level. The two empirically-tuned backoff constants (research.md §4) are explicitly flagged as starting hypotheses pending harness validation, not settled defaults — this feature does not implement adaptive chunk-sizing/concurrency, so that half of Principle III's constant-tuning clause is N/A here. |
| IV. Security by Default | **PASS** | No new secret is introduced — the Drive session URI is bearer-authenticated the same way any other Drive API call is (via the existing encrypted OAuth token), and is itself a capability URL scoped to one upload session, not a durable credential. It's stored in the same SQLite database as the (already-encrypted) tokens; it does not need independent keychain-backed encryption, since possessing it alone is useless without also possessing the account's already-encrypted access token. Tokens remain excluded from all log output, unchanged from Feature 001. |
| V. Simplicity & Bounded Scope | **PASS** | No adaptive chunk-sizing/concurrency (explicitly deferred per spec Assumptions), no OS-level network-reachability watcher (research.md §3 — a plain retry loop achieves the same outcome with none of that surface area), no cross-file concurrency, no bypassing bandwidth/quota — none of the constitution's non-goals are reintroduced. |
| VI. Reliability Gates as Acceptance Criteria | **PASS, directly exercised** | This feature's Success Criteria (SC-001–SC-006) *are* a subset of the constitution's Reliability Gates verbatim (resume timing, crash recovery, degraded-network completion, accurate progress, bounded memory on huge files) — quickstart.md's scenarios are written directly against them. |
| VII. Cross-Platform Parity | **PASS, N/A for new OS-specific surface** | This feature adds no new OS-specific integration (research.md §3 — connectivity detection is deliberately protocol-level, not OS-level, specifically to avoid a third per-platform fallback story alongside the keychain one Feature 001 already carries). Crash/power-loss recovery (User Story 2) relies only on SQLite's own durability (unmodified `modernc.org/sqlite` defaults, already cross-platform) and startup-time detection, not any OS crash-reporting mechanism. |

No violations requiring Complexity Tracking justification.

## Project Structure

### Documentation (this feature)

```text
specs/002-resumable-upload-engine/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md         # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
main.go                   # Wails entrypoint (unchanged)
app.go                    # App struct: UploadStart/UploadGetStatus behavior changes internally
                           # to drive the resumable engine; UploadGetRecoverable,
                           # UploadConfirmRestart, UploadCancel are new bound methods
                           # (contracts/wails-bindings.md)

internal/
├── auth/                 # unchanged (Feature 001)
├── drive/
│   ├── folders.go        # unchanged
│   ├── upload.go         # replaced: was the single-request Files.Create(...).Media() call;
│   │                      # becomes the resumable session orchestrator (init → chunk loop → complete)
│   ├── resumable.go       # NEW: raw-HTTP resumable protocol primitives (initiate session,
│   │                      # send one chunk, query offset) — research.md §1
│   ├── retry.go            # NEW: backoff policy + error classification (retryable vs. terminal) — research.md §4
│   ├── identity.go          # NEW: source-file identity check (cheap size/mtime + incremental
│   │                      # hash checkpoint/verify) — research.md §5
│   └── progress.go        # unchanged (countingReader), now driven by acknowledged offset
├── storage/
│   ├── schema.go          # extended: new upload columns + expanded status CHECK constraint
│   └── upload.go          # extended: session/offset/identity persistence, new state transitions
├── keychain/              # unchanged
└── events/                # extended: new upload:paused, upload:awaiting-confirmation events

frontend/
├── src/
│   ├── screens/
│   │   └── progress.ts    # extended: renders paused/awaiting-confirmation states, confirm-restart/cancel actions
│   └── api/upload.ts       # extended: GetRecoverable, ConfirmRestart, Cancel bindings
└── tests/                 # new Playwright specs per user story (network-drop resume,
                           # crash-recovery-on-relaunch, terminal-condition handling)

build/                    # unchanged
```

**Structure Decision**: Same single Wails project as Feature 001 — this
feature is additive within the existing `internal/drive` and
`internal/storage` packages (new files for genuinely new concerns:
`resumable.go` for the raw protocol, `retry.go` for classification/backoff,
`identity.go` for the file-identity check) plus a rewrite of `upload.go`'s
orchestration logic, rather than a new package or module. No new top-level
directory is introduced.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Not applicable — see Constitution Check above.

## Post-Design Constitution Check (re-evaluated after Phase 1)

Per the constitution's Compliance Review requirement, the gate above was
re-checked against the concrete design in data-model.md and
contracts/wails-bindings.md:

- **I. Stack Discipline**: data-model.md's schema changes are additional
  columns on the existing `upload` table, not a new table or datastore; no
  library beyond Technical Context's list was introduced during design.
  **PASS**.
- **II. Protocol Correctness Over Cleverness**: contracts/wails-bindings.md
  and data-model.md's state machine confirm exactly one chunk is ever
  in-flight per upload (the single `bytes_sent` offset plus a single
  `session_uri` per row structurally prevents modeling two concurrent
  chunks of the same file), and FR-013's one-upload-at-a-time constraint is
  unchanged from Feature 001. **PASS**.
- **III. Test-First for the Upload Engine**: data-model.md's Error
  Classification table and state transitions are concrete enough to write
  failing tests against directly (one test per row/transition) before
  implementation — this is deferred to `/speckit-tasks` task breakdown, but
  nothing in the design blocks it. **PASS** (gate satisfied at design level;
  actual test-first sequencing is enforced at task/implementation time).
- **IV. Security by Default**: data-model.md's Upload entity confirms
  `session_uri` is a plain (unencrypted) TEXT column, justified in
  Technical Context — it's a capability URL, not a durable credential, and
  is useless without the already-encrypted OAuth tokens. No new column
  holds a secret. **PASS**.
- **V. Simplicity & Bounded Scope**: contracts/wails-bindings.md adds
  exactly three new bound methods (`Upload.GetRecoverable`,
  `Upload.ConfirmRestart`, `Upload.Cancel`) and two new events beyond
  Feature 001's set — `Upload.Cancel` is a direct, synchronous user action
  with no new event of its own (Simplicity: the caller already knows the
  outcome from the call's return, so no new event is needed) — no
  speculative multi-file, concurrency, or adaptive-sizing surface crept in.
  **PASS**.
- **VI. Reliability Gates as Acceptance Criteria**: quickstart.md's
  scenarios map directly to SC-001 through SC-006. **PASS**.
- **VII. Cross-Platform Parity**: unchanged from the initial gate — no new
  OS-specific code path was introduced during design. **PASS, N/A**.

No new violations surfaced during design; Complexity Tracking remains empty.
