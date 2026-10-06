# Implementation Plan: Automatic Sermon Summaries

**Branch**: `006-sermon-summaries` | **Date**: 2026-10-06 (revised: free, on-Mac engine) | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-sermon-summaries/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

When a video's captions finish (Feature 005), Ballast copies the
transcript into a summary work folder. It waits until the speech model has
finished, using a shared heavy-work lock so only one model runs at a
time. Then it:
1. Starts a bundled `llama-server` (llama.cpp) on `127.0.0.1`, with a free
   open model downloaded once after a consent prompt.
2. Asks for a summary in a fixed JSON shape. Sermons up to about an hour
   are done in one pass; longer ones in ~20-minute parts that are then
   combined.
3. Checks every quote against the transcript, routes doubtful Bible
   references to a "Check before sharing" note, and enforces the
   WhatsApp-message rules in code.
4. Saves a Markdown copy next to the original video straight away, and
   once the video has arrived, puts a Google Doc next to it in Drive,
   tagged so a crash can't duplicate it.

The cost is zero. Nothing except the finished summary leaves the Mac. The
upload and captions are never affected.

## Technical Context

**Language/Version**: Go 1.26; TypeScript (Wails vanilla-ts frontend)

**Primary Dependencies**: Existing: Wails v2, `google.golang.org/api/drive/v3`, `golang.org/x/oauth2`, `modernc.org/sqlite`. **New (not a Go module)**: llama.cpp's `llama-server` (MIT), shipped as a helper binary like Feature 005's `whisper-cli` (research.md §1, §15), and one free open GGUF model downloaded at runtime and pinned by hash (§2). It is called over a plain local HTTP request with `net/http`; no new Go module. Depends on Feature 005's transcript, `setting` table, local-copy rules, and model downloader (moved into `internal/modelfetch`, §14).

**Storage**: Same SQLite file: a new `summary_job` table and two `setting` keys (data-model.md). Model file under `<app data>/models/`. Per-job work folders under `<app data>/summaries/<job id>/`. No secrets.

**Testing**: Go `testing` with a fake `Summarizer` and a fake chat-completions `httptest` server. No real model is loaded in tests or CI. Playwright covers Settings, the download prompt, and the summary line through E2E-mock events. Model choice and quality are validated in quickstart Scenarios 0–3.

**Target Platform**: macOS on Apple silicon (wherever Feature 005 runs). Elsewhere it shows "Summaries need captions, which aren't available on this system yet".

**Project Type**: desktop-app (single Wails project)

**Performance Goals**: ≤10 min after captions for a 1-hour sermon, ≤30 min for 3 hours, on an 8 GB Apple-silicon Mac (SC-004; measured in quickstart).

**Constraints**: $0 cost. No outside network use except the model download and the Drive upload (FR-005, SC-006). Only one model in memory at a time (FR-008, SC-007). `llama-server` peak memory about 5.5 GB or less (§2). Fixed 16k context, so memory stays flat for any sermon length (§4). The upload and caption code paths are unchanged except for the transcript hand-off hook and the shared lock.

**Scale/Scope**: Single user, one summary job at a time, one engine.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS with justification** | No new language, Go module, or datastore. `llama-server` plus a model file is a new major dependency, justified below. It is the same kind as Feature 005's already-justified `whisper-cli`, and it reuses its packaging and download code. |
| II. Protocol Correctness | **PASS, N/A** | The upload protocol is untouched. The Google Doc is one small create with tag-then-adopt (§11). |
| III. Test-First | **PASS** | The upload engine is unchanged. Summary logic (parts and combine, quote check, reference check, share-message enforcement, retries, lock) is written test-first against fakes. The model choice is benchmarked (Scenario 0), not assumed. |
| IV. Security by Default | **PASS** | No credentials are added. The engine listens on `127.0.0.1` only. The model is verified by pinned SHA-256 before use. Transcript text is never logged. |
| V. Simplicity & Bounded Scope | **PASS** | One engine behind a two-method interface. No cloud option, no model picker, no settings beyond on/off. |
| VI. Reliability Gates | **PASS, to verify** | The upload path is unchanged. The heavy-work lock keeps memory pressure off the Mac during uploads. Quickstart Scenario 2 and Scenario 4 step 2 verify both. |
| VII. Cross-Platform Parity | **PASS** | Available exactly where Feature 005 is, with an explicit message elsewhere. |

**Post-design re-check**: unchanged. Gate passes.

## Project Structure

### Documentation (this feature)

```text
specs/006-sermon-summaries/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/wails-bindings.md
├── checklists/requirements.md
└── tasks.md            # /speckit-tasks
```

### Source Code (repository root)

```text
internal/
├── modelfetch/                   # MOVED from internal/captions/model.go (research.md §14): resume, verify, rename, progress
├── heavywork/                    # NEW: one-model-at-a-time lock shared by captions + summaries (§7)
├── summaries/                    # NEW package
│   ├── summary.go                # Summary + PartNotes structs (= JSON schemas, §3–§4); Summarizer interface (§13)
│   ├── server.go                 # start/stop llama-server on 127.0.0.1:<free port>, wait-ready, nice 10, kill on cancel (§1)
│   ├── local.go                  # localSummarizer: single pass or parts→combine via /v1/chat/completions + json_schema
│   ├── prompt.go                 # system prompts (part notes, combine, single pass) + reference example (§12)
│   ├── transcript.go             # SRT → "[mm:ss] text"; split into ~20-min parts on cue boundaries (§4, §10)
│   ├── verify.go                 # quote check; reference certainty + 66-book check (§5, §6)
│   ├── share.go                  # enforce share-message rules: strip emoji/timestamps, reject "transcript" (§12)
│   ├── render.go                 # Summary → HTML (Google Doc) and Markdown (local copy) (§11)
│   ├── worker.go                 # job runner + state machine + retries + restart recovery
│   ├── testdata/reference-2026-10-04.md
│   └── *_test.go
├── captions/worker.go            # + TranscriptReady/NoTranscript hooks; take heavywork lock around transcription
├── drive/summarydoc.go           # NEW: find-by-tag, free name, create Google Doc from HTML
├── storage/summary.go            # NEW: SummaryJob CRUD + transitions
├── storage/schema.go             # + summary_job table
└── events/events.go              # + summaries:consent-needed, summaries:updated

app.go                            # + Summaries* bound methods; create/cancel summary jobs alongside caption jobs
mock_e2e.go                       # + scripted summary events
scripts/build-engines.sh          # RENAMED from build-whisper.sh: + pinned llama.cpp → build/engines/llama-server
frontend/src/api/summaries.ts     # NEW
frontend/src/ui/live.ts, components.ts, views/settings.ts, views/transfers.ts, views/home.ts
```

**Structure Decision**: A new `internal/summaries` package mirrors
`internal/captions`. Two tiny shared packages (`modelfetch`, `heavywork`)
replace copying code between the two features. `app.go` remains the only
place joining uploads, captions, and summaries.

**Effect on Feature 005's plan/tasks**: its model downloader is written
directly in `internal/modelfetch`, its worker takes the `heavywork` lock,
its build script is named `build-engines.sh`, and it exposes the
transcript hand-off hook. These are small edits to 005's tasks.md, made
when 006's tasks are generated.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New major dependency: llama.cpp `llama-server` binary + a ~2–5 GB open model (Principle I) | The user requires summaries at zero cost with nothing leaving the Mac; nothing in the stack can write text summaries. | Paid cloud APIs cost money (rejected by the user). Ollama adds a separately managed background service. Apple's on-device model has too small a context for a sermon transcript and needs Swift bridging. cgo bindings would put a C++ toolchain into every build and let a crash take down an upload. Running it as a child process keeps it at the edge as one replaceable executable, sharing Feature 005's packaging and download code. |
