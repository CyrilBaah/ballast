# Implementation Plan: Google Sign-In, File/Folder Picker & Basic Upload

**Branch**: `001-auth-picker-upload` | **Date**: 2026-08-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-auth-picker-upload/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Deliver the smallest end-to-end vertical slice of Ballast: a signed-in user
picks one local file, browses/selects a Google Drive destination folder, and
sends the file via a single, basic (non-resumable) Drive API upload with
visible progress and a clear success/failure outcome. Authentication uses the
Google OAuth 2.0 desktop (installed-app) loopback-redirect flow with PKCE;
tokens are encrypted at rest in SQLite with the encryption key held in the OS
keychain. The Drive destination browser is purpose-built (Drive API
`files.list` calls surfaced through Wails-bound Go methods) rather than
Google's embedded Picker widget, to avoid a second JS SDK dependency and an
awkward authorized-origin story inside a native webview. This feature
deliberately does not touch the resumable/adaptive upload engine — that is a
later feature this slice exists to unblock.

## Technical Context

**Language/Version**: Go 1.22+ (backend/engine, per Constitution Principle I); TypeScript (Wails "vanilla-ts" frontend template — no React/Vue/Svelte, per Principle V's bias against unneeded generality for a 3-screen UI)

**Primary Dependencies**: Wails v2 (desktop shell); `google.golang.org/api/drive/v3` + `golang.org/x/oauth2`/`golang.org/x/oauth2/google` (Drive access, OAuth); `modernc.org/sqlite` (pure-Go, cgo-free SQLite driver — chosen over `mattn/go-sqlite3` to avoid per-OS cgo cross-compilation risk, per Principle VII); `zalando/go-keyring` (cross-platform OS keychain access: macOS Keychain / Windows Credential Manager / Linux Secret Service)

**Storage**: SQLite (single local file in the OS app-data dir) via `modernc.org/sqlite`. OAuth access/refresh tokens stored as AES-256-GCM ciphertext blobs; the encryption key itself lives only in the OS keychain (never in the SQLite file or an adjacent file), per Principle IV.

**Testing**: Go `testing` package for backend unit tests (auth flow, token encryption, Drive client wrapper, upload success/failure classification); Playwright driven against `wails dev`'s local dev server for the three user-story flows; GitHub Actions CI across macOS/Windows/Linux.

**Target Platform**: Desktop — macOS, Windows, Linux (Constitution Principle VII: cross-platform parity, explicit fallback required where a primary mechanism is OS-specific)

**Project Type**: desktop-app (single Wails project: one Go backend process + embedded web frontend, no separate client/server split)

**Performance Goals**: Not throughput-sensitive (single file, one request at a time). Progress indicator must visibly update at least every 5s (SC-004) — the only latency-adjacent goal this feature carries.

**Constraints**: Sign-in completable in <30s active interaction (SC-001); file+destination selection through upload start in <1min active interaction (SC-002); ≥95% of launches within a valid session skip re-auth (SC-005); no resumable/session-recovery behavior (explicitly deferred); no artificial file-size cap, but the local file and the outgoing HTTP body must be streamed (`io.Reader`) rather than fully buffered in memory, since that costs nothing extra to get right now and avoids a foreseeable large-file regression.

**Scale/Scope**: Single user, single Google account per installation, one file/one upload at a time, three screens (sign-in, pick file + destination, upload progress/result). No concurrency, no multi-account, no resumable state — all explicitly out of scope per the spec's Assumptions.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS** | Go + Wails + SQLite only. No second backend language, no second datastore. `modernc.org/sqlite`, `go-keyring`, `golang.org/x/oauth2`, and `google.golang.org/api` are additive libraries within the existing stack, not new platforms. |
| II. Protocol Correctness Over Cleverness | **N/A (not exercised)** | This feature uses Drive's simple/non-resumable media upload (`Files.Create().Media(...)`), not the resumable byte-stream protocol Principle II governs. No chunking, ordering, or concurrency-within-a-file logic exists here to violate. |
| III. Test-First for the Upload Engine | **N/A (not exercised)** | No session/offset/resume/retry-classification/adaptive-chunking logic exists in this feature — that logic is explicitly deferred to the later resumable-engine feature this slice unblocks. This feature's own logic (auth, encryption, upload outcome detection) still gets Go tests, just not under this NON-NEGOTIABLE gate, which is scoped specifically to the resumable engine. |
| IV. Security by Default | **PASS by design** | Tokens encrypted at rest (AES-256-GCM) in SQLite; key held only in OS keychain via `go-keyring`, never on disk beside the DB; tokens excluded from all log output at every level (see research.md logging note). |
| V. Simplicity & Bounded Scope | **PASS** | Custom minimal Drive folder browser instead of embedding Google's Picker JS widget (avoids a second JS SDK and an authorized-origin workaround for no requirement-level benefit — spec's own Assumptions treat both as equivalent). Vanilla-ts frontend instead of a component framework. No retry/resume logic beyond FR-010's explicit "fail and let the user retry manually." |
| VI. Reliability Gates as Acceptance Criteria | **N/A (not exercised)** | The Reliability Gates (resume timing, crash recovery, packet-loss simulation, memory-bounded huge files) are defined against the resumable upload path, which this feature does not implement. Not a gap — a scope boundary. |
| VII. Cross-Platform Parity | **PASS, with an explicit fallback documented** | `go-keyring` covers all three OS keychains, but Linux Secret Service depends on a running keyring daemon that isn't guaranteed on every distro/session. Fallback: if the OS keychain is unavailable, sign-in fails closed with a clear, actionable error — the app MUST NOT fall back to storing the encryption key unencrypted next to the database (that would violate Principle IV). See research.md. |

No violations requiring Complexity Tracking justification.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
main.go                   # Wails entrypoint (app bootstrap, window config)
app.go                    # App struct: Wails-bound methods callable from the frontend

internal/
├── auth/                 # OAuth loopback+PKCE flow, session persistence, sign-out
├── drive/                # Drive API client wrapper: folder listing, simple (non-resumable) upload
├── storage/              # SQLite access (modernc.org/sqlite) + AES-256-GCM token encryption
├── keychain/             # go-keyring wrapper: fetch/create the encryption key, fail-closed fallback
└── events/                # Wails runtime.EventsEmit helpers (auth:changed, upload:progress, upload:complete, upload:failed)

frontend/
├── src/
│   ├── screens/          # sign-in, file+folder picker, upload progress/result
│   └── wailsjs/          # generated Go↔TS bindings (Wails codegen, not hand-written)
└── tests/                # Playwright specs, one per user story

build/                    # Wails-generated platform build config (unmodified from Wails defaults)
```

**Structure Decision**: Single Wails project (Option 1 shape, Wails-flavored) —
one Go backend process plus one embedded vanilla-ts frontend, no separate
client/server split. Internal Go packages are organized by capability
(`auth`, `drive`, `storage`, `keychain`) rather than by generic
model/service/controller layers, since each capability maps directly to one
of the spec's key entities or a cross-cutting constitutional requirement
(encryption, keychain access). Playwright specs live under `frontend/tests/`
because they exercise the built frontend against `wails dev`'s local server,
not the Go backend directly.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Not applicable — see Constitution Check above.

## Post-Design Constitution Check (re-evaluated after Phase 1)

Per the constitution's Compliance Review requirement, the gate above was
re-checked against the concrete design in data-model.md and
contracts/wails-bindings.md:

- **I. Stack Discipline**: Still Go + Wails + SQLite only; no library added
  during design beyond those already named in Technical Context. **PASS**.
- **IV. Security by Default**: data-model.md's Account entity confirms
  tokens are stored only as ciphertext + nonce columns, with sign-out
  revoking the OAuth grant server-side and deleting the row outright (no
  lingering plaintext-adjacent state, no lingering server-side grant).
  **PASS**.
- **V. Simplicity & Bounded Scope**: contracts/wails-bindings.md exposes
  exactly the methods/events the three user stories need (auth, file pick,
  folder list, upload start/status) — no speculative endpoints (e.g. no
  multi-file batch upload, no folder create/rename) crept in during design.
  **PASS**.
- **VII. Cross-Platform Parity**: The keychain fail-closed fallback
  (research.md §4) is carried through unchanged into `Auth.SignIn()`'s
  documented rejection behavior in the contract. **PASS**.
- Principles II, III, VI remain **N/A** — the design introduces no
  resumable-protocol, chunking, or retry-classification logic.

No new violations surfaced during design; Complexity Tracking remains empty.
