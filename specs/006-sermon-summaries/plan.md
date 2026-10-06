# Implementation Plan: Automatic Sermon Summaries

**Branch**: `006-sermon-summaries` | **Date**: 2026-10-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-sermon-summaries/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

When a video's captions finish (Feature 005) and summaries are switched on
with the user's own Anthropic API key, Ballast:
1. Copies the transcript into a summary work folder.
2. Sends it as timestamped plain text, and nothing else, to Claude Opus 5.5
   through the official Go SDK, asking for structured JSON output.
3. Checks every quote against the transcript and drops any that don't
   match. It routes uncertain Bible references to a "Check before sharing"
   note.
4. Saves a Markdown copy next to the original video straight away.
5. Once the video has arrived, uploads a Google Doc next to it in Drive,
   tagged so a crash can never duplicate it.

The summary includes a WhatsApp-ready share message, sermon only, with no
emojis. Failures are isolated, retried a limited number of times, and
explained, with a "Try again" that reuses the saved transcript. The upload
and the captions are never touched. The key lives only in the OS keychain.

## Technical Context

**Language/Version**: Go 1.26; TypeScript (Wails vanilla-ts frontend)

**Primary Dependencies**: Existing: Wails v2, `google.golang.org/api/drive/v3`, `golang.org/x/oauth2`, `modernc.org/sqlite`, `github.com/zalando/go-keyring`. **New**: `github.com/anthropics/anthropic-sdk-go`, the official Anthropic SDK (research.md §1). Depends on Feature 005's transcript, `setting` table, and local-copy rules.

**Storage**: Same SQLite file. A new `summary_job` table and two `setting` keys (data-model.md). The API key is in the OS keychain only. Per-job work folders live under `<app data>/summaries/<job id>/`.

**Testing**: Go `testing` with a fake `Summarizer` for all job logic, and an `httptest` server returning real API error bodies for the error mapping. No test spends money. Playwright covers Settings and the summary line through E2E-mock events. The real model is exercised only in quickstart Scenarios 1–3.

**Target Platform**: Wherever Feature 005 captions run (macOS on Apple silicon). Elsewhere, Settings shows "Summaries need captions, which aren't available on this system yet".

**Project Type**: desktop-app (single Wails project)

**Performance Goals**: Summary ready within 5 minutes of captions finishing for sermons up to 3 hours (SC-004). One request per sermon, streamed.

**Constraints**: Only transcript text leaves the machine (FR-005, SC-006). The key is never logged, stored in the DB, or sent to the UI (FR-013, SC-007). At most 3 automatic attempts per job (research.md §8). Cost about $0.10–$0.40 per sermon (§9, confirmed in quickstart). The upload and caption code paths are unchanged (FR-008).

**Scale/Scope**: Single user, one summary job at a time, one provider.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS with justification** | One new Go module (the official Anthropic SDK), justified below. No new language or datastore: the new table goes in the same SQLite file, and the key uses the keychain library already in use. |
| II. Protocol Correctness | **PASS, N/A** | The resumable upload protocol is untouched. The Google Doc is one small `files.create` with the tag-then-adopt check (§11). |
| III. Test-First | **PASS** | The upload engine is unchanged. Summary job logic, quote verification, and error mapping are written test-first against fakes. The prompt's tunables (effort `medium`, retry spacing 1/5/15 min) are hypotheses checked in quickstart. |
| IV. Security by Default | **PASS, directly exercised** | A new credential (the API key) is stored only in the OS keychain, never in SQLite or files, never logged, and masked in the UI. Checked by quickstart Scenario 4 / SC-007. Transcript text is not logged either. |
| V. Simplicity & Bounded Scope | **PASS** | One provider behind a two-method interface; no Gemini build (FR-019). No prompt caching, batches, or tools. One request per sermon. |
| VI. Reliability Gates | **PASS** | No change to the upload path. Quickstart Scenario 2 verifies that the upload and captions are unaffected by every failure mode. |
| VII. Cross-Platform Parity | **PASS** | Available exactly where Feature 005 is. Elsewhere it shows an explicit message, with no silent degradation. |

**Post-design re-check**: unchanged. The design adds one table, one
package, one Drive helper file, and bound methods in a new `Summaries*`
namespace; no existing signature changes. Gate passes.

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
├── summaries/                    # NEW package
│   ├── summary.go                # Summary struct = the JSON schema (research.md §3); Summarizer interface (§13)
│   ├── anthropic.go              # anthropicSummarizer: SDK client from keychain key, Validate via Models.List, Summarize (stream, structured output, effort, fallbacks) (§1–§2, §7)
│   ├── prompt.go                 # system prompt + reference example (§12)
│   ├── errors.go                 # *anthropic.Error → outcome + user message + retryable (§8)
│   ├── transcript.go             # SRT → "[mm:ss] text" lines (§10)
│   ├── verify.go                 # quote check against transcript; split certain/uncertain references (§4, §5)
│   ├── render.go                 # Summary → HTML (Google Doc) and Markdown (local copy) (§11)
│   ├── key.go                    # keychain get/set/delete/mask for "anthropic-api-key" (§6)
│   ├── worker.go                 # one-at-a-time job runner, state machine, retries, restart recovery
│   ├── testdata/reference-2026-10-04.md   # the hand-made summary shared on 2026-10-06
│   └── *_test.go
├── captions/worker.go            # + TranscriptReady(uploadID, srtPath) / NoTranscript(uploadID) hooks
├── drive/summarydoc.go           # NEW: find-by-tag, free name, create Google Doc from HTML (§11)
├── storage/summary.go            # NEW: SummaryJob CRUD + transitions
├── storage/schema.go             # + summary_job table
└── events/events.go              # + summaries:updated

app.go                            # + Summaries* bound methods; create/cancel jobs alongside caption jobs
mock_e2e.go                       # + scripted summary events
frontend/src/api/summaries.ts     # NEW
frontend/src/ui/live.ts, components.ts, views/settings.ts, views/transfers.ts, views/home.ts
```

**Structure Decision**: A new `internal/summaries` package mirrors
`internal/captions`. The only change to Feature 005 is a hook in its
worker that hands over the finished transcript. `app.go` remains the only
place that joins uploads, captions, and summaries.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New Go module: `github.com/anthropics/anthropic-sdk-go` (Principle I) | The feature's core is a call to Claude. The official SDK provides typed errors, retries, streaming, and structured-output types that follow API changes. | Hand-written HTTP would reimplement retries, streaming, and error typing, and drift as the API changes. A local model was rejected in the spec discussion (quality on long, mixed-language sermons; an 8 GB Mac already running the speech model). |
