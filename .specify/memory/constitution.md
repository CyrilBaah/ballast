<!--
Sync Impact Report
==================
Version change: [TEMPLATE] → 1.0.0
Rationale: Initial ratification. Constitution was previously an unfilled template
(all placeholder tokens); this is the first concrete version, hence MAJOR (1.0.0)
per template versioning convention for initial adoption.

Modified principles: N/A (first ratification, no prior named principles)

Added sections:
- Core Principles I–VII (Stack Discipline, Protocol Correctness, Test-First for the
  Upload Engine, Security by Default, Simplicity & Bounded Scope, Reliability Gates
  as Acceptance Criteria, Cross-Platform Parity)
- Additional Constraints (technology stack, non-goals as hard constraints)
- Development Workflow (spec-driven development, solo+AI-agent PR discipline)
- Governance

Removed sections: N/A (template placeholders only)

Templates requiring updates:
- ✅ .specify/templates/plan-template.md — Constitution Check section is generic
  ("[Gates determined based on constitution file]"), no edit needed; gates will be
  filled per-feature by /speckit-plan referencing this file.
- ✅ .specify/templates/spec-template.md — generic, no constitution-specific
  references to update.
- ✅ .specify/templates/tasks-template.md — generic, no constitution-specific
  references to update.
- ✅ No CLAUDE-only or agent-specific naming found in installed speckit commands
  requiring correction.

Follow-up TODOs: none — all placeholders resolved.
-->

# Ballast Constitution

## Core Principles

### I. Stack Discipline

Go is the implementation language for the upload engine and all backend logic.
Wails (Go backend + web frontend, single process) is the desktop shell.
SQLite is the only local persistence layer. Introducing a second backend
language (e.g., a Rust/Tauri split), a second local datastore, or a new major
dependency requires a written justification in the relevant plan.md's
Complexity Tracking section — it is not a default-allowed choice.

**Rationale**: The project's own technology review already identified that a
Rust-frontend/Go-backend split adds a second language and an IPC boundary for
no offsetting benefit versus Wails. Stack fragmentation is the most common way
a solo-plus-AI-agents project quietly becomes unmaintainable.

### II. Protocol Correctness Over Cleverness

Google Drive's resumable upload API is a single sequential byte stream per
file: chunks MUST be sent in order, in multiples of 256 KiB (except the final
chunk), and two chunks of the same file MUST NOT be sent concurrently.
Concurrency is achieved only across different files' independent sessions,
never within one file's stream. Any design, optimization, or "clever" transfer
trick that assumes otherwise (parallel chunk upload, out-of-order chunks,
speculative multi-stream tricks for one file) MUST be rejected at review,
regardless of claimed performance benefit.

**Rationale**: This is a hard constraint of Google's API, not a design
preference. Code that violates it will fail in production in ways that are
easy to miss in a happy-path test and catastrophic on a real large upload.

### III. Test-First for the Upload Engine (NON-NEGOTIABLE)

Session/offset/resume logic, retry classification (retryable vs. terminal
errors), and adaptive chunk-sizing/concurrency algorithms MUST have tests
written before or alongside implementation, and those tests MUST fail before
the implementation exists. Empirically-tuned constants (chunk-size steps,
concurrency caps, backoff intervals) MUST be validated against the
network-simulation test harness before being treated as settled — they are
starting hypotheses, not trusted defaults, until a benchmark run confirms
them.

**Rationale**: A silent bug in this specific code path doesn't just fail a
test — it corrupts or loses a real user's in-flight multi-hour upload of a
large file. This is the one part of the system where "fix it after" is not an
acceptable failure mode.

### IV. Security by Default

OAuth tokens and resumable session URIs MUST be encrypted at rest in SQLite.
The encryption key MUST be held in the OS keychain (macOS Keychain / Windows
Credential Manager / Linux Secret Service) and MUST NOT be stored in a file
alongside the database. Credentials MUST NOT appear in logs at any log level,
including debug.

**Rationale**: An app-managed key sitting next to the encrypted database is
not real encryption at rest — it just moves the secret, it doesn't protect
it. Google account access is high-value; this is a boundary the project does
not compromise on for convenience.

### V. Simplicity & Bounded Scope

Do not add abstractions, configuration options, or generality beyond what the
current feature's spec requires. The project's stated non-goals — no bypassing
ISP bandwidth limits, no bandwidth bonding/MPTCP/BitTorrent-style piece
scheduling, no modification of Google's backend, no parallelizing a single
file's byte stream — are hard constraints on design, not deprioritized ideas
that can resurface as "nice to have." A proposal that reintroduces a
documented non-goal requires an explicit constitution amendment, not a quiet
exception in one PR.

**Rationale**: Matches the project's existing engineering norm (three similar
lines beats a premature abstraction) and prevents scope creep from
re-litigating settled architecture decisions one feature at a time.

### VI. Reliability Gates as Acceptance Criteria

A feature touching the upload path is not "done" merely because it passes unit
tests. It must be evaluated against the project's Success Criteria where
applicable: resume within 2 seconds of connectivity returning with zero
re-transmission of acknowledged bytes; full recovery after process crash,
power loss, or OS reboot; successful completion under a simulated 5% packet
loss / 500ms RTT network; progress reporting accurate to within 1% of actual
bytes transferred; no unbounded memory growth on files in the hundreds-of-GB
range. These are acceptance gates for the relevant milestone, not aspirational
targets.

**Rationale**: Qualitative goals ("should be reliable") are not verifiable and
invite scope drift. Measurable gates keep a solo-plus-AI-agents team honest
about what "done" means without a second human reviewer to catch it.

### VII. Cross-Platform Parity

Any OS-specific integration (keychain access, network reachability
detection, code signing) MUST ship with an explicit fallback path for
platforms where the primary mechanism isn't yet implemented, rather than
silently degrading (e.g., failing to detect reconnection, or storing secrets
unencrypted) on unsupported platforms. A feature that only works correctly on
one OS MUST say so explicitly in its spec and plan, not ship silently
partial.

**Rationale**: The project targets macOS, Windows, and Linux. Silent
per-platform gaps are the kind of bug that is invisible in the developer's
own testing environment and only discovered by a user on a different OS.

## Additional Constraints

- **Technology stack**: Go (backend/engine), Wails (desktop shell), SQLite
  (local session/state store, encrypted at rest per Principle IV). Testing
  stack: Go's standard testing package, Playwright for UI flows, GitHub
  Actions for CI.
- **Non-goals are binding**: see Principle V. The project does not attempt to
  exceed physical network bandwidth limits, does not modify or circumvent
  Google Drive's backend or policies, and does not parallelize a single
  file's upload stream.
- **Differentiation vs. rclone**: any claim of outperforming rclone (the
  project's stated competitive benchmark) MUST name a specific, independently
  benchmarkable mechanism (e.g., adaptive chunk sizing, faster reconnect
  detection, warm-daemon vs. cold-CLI start, smarter cross-file concurrency)
  before implementation work targeting that claim begins.

## Development Workflow

- This is a solo-developer project executed primarily through AI coding
  agents, structured with open-source discipline: a public GitHub repository
  (github.com/CyrilBaah/ballast), spec-driven development via Spec Kit
  (constitution → specify → clarify → plan → tasks → analyze →
  taskstoissues → implement), and one pull request per task, even though
  there is a single human maintainer.
- The human maintainer is the sole approver of merges and the final authority
  on scope and architecture tradeoffs (e.g., the AIMD tuning constants in
  Principle III, or resolving open design questions) — these are not
  delegated to an agent's judgment.
- Every pull request touching the upload path MUST be checked against the
  Reliability Gates (Principle VI) relevant to the change before merge.
- Features are sliced into independently testable, independently shippable
  vertical increments (per Spec Kit's user-story prioritization model) rather
  than specified as one monolithic engine build.

## Governance

This constitution supersedes any conflicting practice, template default, or
prior informal convention. All plans, specs, and task lists produced by Spec
Kit commands MUST be checked against these principles; a "Constitution Check"
gate that would fail MUST either change the design or be justified in the
relevant plan's Complexity Tracking section before proceeding.

**Amendment procedure**: A change to this document requires the human
maintainer's explicit approval, a version bump per the policy below, and an
update to the Sync Impact Report at the top of this file. Amendments that
remove or redefine a principle MUST also identify any specs/plans/tasks
already in flight that relied on the old principle.

**Versioning policy** (semantic versioning applied to governance):
- **MAJOR**: Backward-incompatible principle removal or redefinition.
- **MINOR**: A new principle or materially expanded section added.
- **PATCH**: Wording clarifications, typo fixes, non-semantic edits.

**Compliance review**: Every `/speckit-plan` invocation MUST re-check its
Constitution Check gate after Phase 1 design, not only before Phase 0.

**Version**: 1.0.0 | **Ratified**: 2026-08-02 | **Last Amended**: 2026-08-02
