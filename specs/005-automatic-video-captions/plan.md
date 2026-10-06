# Implementation Plan: Automatic Video Captions

**Branch**: `005-automatic-video-captions` | **Date**: 2026-10-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-automatic-video-captions/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

When a video (`.mp4`, `.mov`, `.m4v`) is uploaded while captions are on,
Ballast does five things alongside the upload, leaving the video upload
itself unchanged:
1. Pulls the audio out with macOS's built-in `afconvert`.
2. Transcribes it in about 10-minute pieces with a bundled `whisper-cli`
   (whisper.cpp), using the `large-v3-turbo` model downloaded once on
   first use after a one-time consent prompt.
3. Merges and cleans the pieces into one SubRip (`.srt`) file, and
   saves a copy next to the original video on the Mac straight away, so
   the transcript can be used while the video is still uploading.
4. Once the video is confirmed in Drive, uploads the `.srt` next to it
   with a non-clashing name, tagged so a crash can never create a
   duplicate.
5. Shows its progress as a caption line on the transfer, separate from
   the upload's own status.

Captioning runs one video at a time, in a separate worker, at lower CPU
priority. It survives restarts piece by piece, and it is cancelled
together with its upload. It is available on Apple-silicon Macs only;
everywhere else Settings says so plainly. The embedded-caption-track idea
(Story 4) is an investigation with a written finding, not shipped code.

## Technical Context

**Language/Version**: Go 1.26 (backend, per `go.mod`/toolchain on the maintainer's machine); TypeScript (Wails "vanilla-ts" frontend)

**Primary Dependencies**: Existing: Wails v2, `google.golang.org/api/drive/v3`, `golang.org/x/oauth2`, `modernc.org/sqlite`. **New**: whisper.cpp `whisper-cli` (MIT), built from a pinned tag as a static arm64 binary with Metal and run as a child process (research.md §1, §8); the `ggml-large-v3-turbo` model (about 1.5 GB, MIT), downloaded at runtime and checked against a pinned hash (§2, §7). macOS's built-in `/usr/bin/afconvert` for audio extraction, which is not a new dependency (§3). No new Go module dependency.

**Storage**: Same SQLite file. Two new tables, `caption_job` and `setting` (data-model.md). The `upload` table is unchanged. Files: the model under `<app data>/models/`, and per-job work folders under `<app data>/captions/<job id>/`, deleted when the job ends.

**Testing**: Go `testing`: a new `internal/captions` suite (WAV splitting, SRT merge and cleanup, name choice, model download resume and verify via `httptest`, job state machine with a fake helper program), `storage` tests for the new tables, and `drive` tests for the caption upload and adopt calls. Playwright is extended only for UI states (consent prompt, caption line, Settings), driven by E2E-mock caption events. The real engine is never run in CI. Manual end-to-end validation follows quickstart.md.

**Target Platform**: macOS on Apple silicon (arm64) for captioning. Intel macOS, Windows, and Linux build and run as today, with captions reported unavailable (FR-017, research.md §12).

**Project Type**: desktop-app (single Wails project, unchanged structure)

**Performance Goals**: 3-hour+ video transcribed in no more than its running time on an Apple-silicon Mac (SC-003; to be confirmed in quickstart Scenario 3, fallback in research.md §2). Audio extraction measured at about 1.2 s per 30 minutes of video (research.md §3). Video upload duration within 5% of captions-off (SC-004).

**Constraints**: The uploaded video is byte-for-byte unchanged (FR-001). Nothing but the model download and the final `.srt` upload touches the network (FR-003, SC-007). Memory stays flat regardless of length: about 38 MB of audio per piece, plus the model (FR-013, SC-006). Temporary disk use is about 115 MB per hour of video, plus 1.5 GB once for the model. The upload engine is unchanged (FR-006).

**Scale/Scope**: Single user, one caption job at a time, videos up to multi-hour and tens of GB.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Stack Discipline | **PASS with justification** | No new language, no second datastore (new tables go in the same SQLite file), no new Go module. whisper.cpp plus its model **is** a new major dependency, justified in Complexity Tracking below. `afconvert` ships with macOS. |
| II. Protocol Correctness Over Cleverness | **PASS, N/A** | The resumable upload protocol is untouched. The caption file uses one small, non-resumable `files.create`, and its tag-then-adopt check (research.md §6) prevents duplicates without any parallel or out-of-order sending. |
| III. Test-First for the Upload Engine | **PASS** | `internal/drive`'s upload engine is not modified. The new captions code is still test-first at the task level, and its tunables (10-minute pieces, ±5 s quiet search, 3-repeat collapse, nice 10) are labelled hypotheses to confirm against a real sermon (quickstart Scenario 3). |
| IV. Security by Default | **PASS** | No new credentials. The caption upload reuses the existing encrypted-at-rest OAuth session. The model is downloaded over HTTPS and run only after a pinned SHA-256 check (§7). Transcript text is never logged, so logs hold only IDs, phases, counts, and reasons. |
| V. Simplicity & Bounded Scope | **PASS** | Only spec-required settings: on/off and English/Automatic. No caption editing, translation, backfill, ffmpeg, or format conversion. One worker, one job at a time. Story 4 is a written investigation only. |
| VI. Reliability Gates as Acceptance Criteria | **PASS, to verify** | The upload path is affected only by `app.go` notifying the captions worker. Quickstart Scenarios 2 and 4 verify the upload's resume behaviour and speed are unchanged with captions on. Flat memory (SC-006) is checked on a 3-hour file. |
| VII. Cross-Platform Parity | **PASS** | Captioning is explicitly Apple-silicon-only (FR-017, updated spec Platforms assumption). Other platforms show an explicit "not available on this system yet" message and behave exactly as with captions off, with no silent degradation. Build tags keep non-macOS builds compiling with a stub. |

**Post-design re-check (after Phase 1)**: unchanged. The data model adds
tables in the existing SQLite file, the contract adds a captions
namespace without changing any existing method signature, and nothing in
the design touches `internal/drive`'s resumable loop. Gate passes.

## Project Structure

### Documentation (this feature)

```text
specs/005-automatic-video-captions/
├── plan.md                            # This file
├── research.md                        # Phase 0 output
├── data-model.md                      # Phase 1 output
├── quickstart.md                      # Phase 1 output
├── contracts/
│   └── wails-bindings.md              # Phase 1 output
├── checklists/requirements.md         # from /speckit-specify
├── findings-embedded-captions.md      # Story 4 outcome (written during implementation)
└── tasks.md                           # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
internal/
├── captions/                    # NEW package — everything captions-specific
│   ├── engine.go                # locate whisper-cli (env var, next to executable); availability + reason
│   ├── engine_darwin_arm64.go   # Apple-silicon: available when the binary exists
│   ├── engine_other.go          # every other platform: "not available on this system yet"
│   ├── model.go                 # model download: resume via Range, size + SHA-256 check, rename (§7)
│   ├── extract.go               # run afconvert at low priority; "no audio track" detection (§3)
│   ├── wav.go                   # read WAV header, split at quiet points into ~10-min pieces (§4)
│   ├── transcribe.go            # run whisper-cli per piece, parse -pp progress (§1)
│   ├── srt.go                   # parse/shift/merge SRT, drop repeats, empty check (§5)
│   ├── localcopy.go             # save the .srt next to the original video (fallback: Downloads), free-name choice (FR-022)
│   ├── formats.go               # captionable vs unsupported video extensions (§10)
│   ├── worker.go                # one-at-a-time job runner, state machine, restart recovery
│   └── *_test.go
├── drive/
│   └── captionfile.go           # NEW: upload .srt (multipart), free-name choice, find-by-tag adopt (§6)
├── storage/
│   ├── schema.go                # + caption_job and setting tables
│   ├── caption.go               # NEW: CaptionJob CRUD + transitions
│   └── setting.go               # NEW: get/set with defaults
└── events/events.go             # + captions:consent-needed, captions:updated

app.go                           # + Captions* bound methods; notify worker on upload start/succeed/fail/cancel/delete
mock_e2e.go                      # + scripted caption events for Playwright

frontend/src/
├── api/captions.ts              # NEW: wrappers for the Captions* bindings
├── ui/live.ts                   # + caption state per upload, event wiring, consent prompt trigger
├── ui/components.ts             # + caption status line, consent prompt
├── ui/views/settings.ts         # + captions switch, language choice, notice / unavailable reason
└── ui/views/transfers.ts        # + caption line, "Open captions in Drive"

scripts/build-whisper.sh         # NEW: build pinned whisper.cpp → build/whisper/whisper-cli, copy into .app
.gitignore                       # + /build/whisper/
```

**Structure Decision**: Keep the single Wails project. All captions logic
goes in one new `internal/captions` package, so the upload engine
(`internal/drive`) gains only one small, separate file for the caption
file call. `app.go` stays the only place that joins uploads and captions.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New major dependency: whisper.cpp `whisper-cli` binary plus a ~1.5 GB speech model (Principle I) | The spec requires free, offline, unlimited speech-to-text with high accuracy on accented English (FR-003, FR-021); nothing already in the stack does speech recognition. | Cloud speech APIs send audio off the machine and have free-tier caps (FR-003). Apple's on-device speech framework would need Swift/Objective-C bridging and is untested for multi-hour audio. cgo bindings to whisper.cpp would put a C++ toolchain into every build and let an engine crash take down an in-flight upload. A child process keeps the dependency at the edge, as one replaceable executable. |
