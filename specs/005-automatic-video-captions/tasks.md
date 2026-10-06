---

description: "Task list for Automatic Video Captions"
---

# Tasks: Automatic Video Captions

**Input**: Design documents from `/specs/005-automatic-video-captions/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, quickstart.md (all present)

**Tests**: Included. The spec does not ask for them explicitly, but plan.md's
Testing section commits to a Go test suite for `internal/captions`, and
Constitution Principle III treats new tuning constants (piece length, quiet
search window, repeat collapse) as hypotheses that need tests. Write each
test task before its implementation task and confirm it fails first.

**Organization**: Tasks are grouped by user story:
- **US1 (P1)**: a video arrives in Drive with a caption file next to it, plus an early local copy.
- **US2 (P1)**: captioning never puts the upload at risk.
- **US3 (P2)**: turn captions on or off and choose the language.
- **US4 (P3)**: investigate an embedded caption track.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on unfinished tasks)
- **[Story]**: Which user story this task belongs to (US1–US4)
- Paths follow plan.md's Project Structure

## Path Conventions

- Go backend: `app.go`, `mock_e2e.go`, `internal/captions/`, `internal/drive/`, `internal/storage/`, `internal/events/`
- Frontend: `frontend/src/`, `frontend/tests/`, generated bindings in `frontend/wailsjs/`
- Build tooling: `scripts/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Build tooling and test scaffolding every later phase needs.

- [X] T001 Create `scripts/build-engines.sh` (shared with Feature 006, which adds `llama-server` to it). For now it clones whisper.cpp at a pinned release tag (record the tag in the script) and builds `whisper-cli` for arm64 with `-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DBUILD_SHARED_LIBS=OFF` into `build/engines/whisper-cli`. With `--bundle <path to Ballast.app>`, it copies every built engine into `Contents/MacOS/` (research.md §8). It exits with a clear message if `cmake` or Xcode command-line tools are missing.
- [X] T002 [P] Add `/build/engines/` to `.gitignore`.
- [X] T003 [P] Create the `internal/captions` package with a `doc.go` describing its job: extract audio, transcribe in pieces, merge to SRT, and hand the result to the worker (plan.md Structure).
- [X] T004 [P] Create a fake speech engine for tests in `internal/captions/testdata/fakewhisper/main.go`. It accepts the same flags `transcribe.go` will pass (`-m -f -l -osrt -of -pp -sns -t`), prints `progress = N%` lines, and writes a deterministic `.srt` for the given audio length. A `FAKEWHISPER_MODE` environment variable selects `ok`, `empty`, `repeat` (a 5× repeated cue), `crash`, or `hang`. Tests build it with `go build` into a temp folder.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Storage, availability, format detection, and events. Every story depends on these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T005 Add the `caption_job` and `setting` tables (columns, CHECK constraints for `status`/`phase`, `UNIQUE(upload_id)`) to `internal/storage/schema.go` exactly as specified in data-model.md, created with `CREATE TABLE IF NOT EXISTS` in `ensureSchema`. Leave the `upload` table unchanged.
- [X] T006 [P] Write `internal/storage/setting_test.go`: missing keys return defaults (`captions_enabled=true`, `caption_language=en`, `caption_model_consent=unasked`); values round-trip; values survive reopening the DB.
- [X] T007 Implement `internal/storage/setting.go`: `GetSetting(key) (string, error)` with the defaults table from data-model.md, `SetSetting(key, value)`, plus typed helpers `CaptionsEnabled()`, `CaptionLanguage()`, `CaptionModelConsent()` and their setters (depends on T005, T006).
- [X] T008 [P] Write `internal/storage/caption_test.go`: create a job for an upload (a second one for the same upload is rejected); every allowed transition in data-model.md's state machine; validation rules (phase NULL exactly when ended, `drive_file_id` and `link` set together and only when done, failed requires a note, `pieces_done <= piece_count`); `ListActiveCaptionJobs`; deleting an upload via `DeleteUpload` also deletes its job.
- [X] T009 Implement `internal/storage/caption.go`: a `CaptionJob` struct mirroring data-model.md, `CreateCaptionJob`, `GetCaptionJobByUpload`, `ListActiveCaptionJobs`, transition setters (`SetCaptionPhase`, `SetCaptionProgress`, `SetCaptionPieces`, `SetCaptionLocalCopy`, `SetCaptionDone`, `SetCaptionFailed`, `SetCaptionCancelled`) that set `updated_at`/`ended_at`, and the cascade in `internal/storage/upload.go`'s `DeleteUpload` (depends on T005, T008).
- [X] T010 [P] Write `internal/captions/formats_test.go` and implement `internal/captions/formats.go`: `Classify(path) (captionable bool, unsupportedExt string)`, case-insensitive. `.mp4/.mov/.m4v` are captionable; `.mkv .avi .webm .wmv .flv .mpg .mpeg .3gp` are unsupported video; anything else is not a video (research.md §10).
- [X] T011 [P] Write `internal/captions/engine_test.go` and implement `internal/captions/engine.go`, `engine_darwin_arm64.go`, and `engine_other.go`. `Availability() (ok bool, reason string, binPath string)` checks `BALLAST_WHISPER_CLI`, then the folder next to `os.Executable()`. On non-darwin/arm64 builds it always returns "Captions aren't available on this system yet". A missing binary returns "The speech engine is missing from this copy of Ballast" (research.md §8, §12; FR-017).
- [X] T011a [P] Create the shared `internal/heavywork` package (Feature 006 research.md §7): a single process-wide lock with `Acquire(ctx, owner string) (release func(), err error)` that waits until free or until ctx is cancelled, and `Holder() string` for status display. Write `internal/heavywork/heavywork_test.go`: only one holder at a time; a waiter gets it once released; a cancelled waiter returns `ctx.Err()` without acquiring.
- [X] T012 [P] Add event names `CaptionsConsentNeeded = "captions:consent-needed"` and `CaptionsUpdated = "captions:updated"`, plus emit helpers and payload structs matching `CaptionJobDTO`, to `internal/events/events.go` (contracts/wails-bindings.md).

**Checkpoint**: Foundation ready. `go test ./internal/storage ./internal/captions` passes.

---

## Phase 3: User Story 1 - A video arrives in Drive with a caption file next to it (Priority: P1) 🎯 MVP

**Goal**: Uploading a `.mp4/.mov/.m4v` with captions on produces a `.srt`:
saved next to the local video as soon as it's ready, and placed in the
video's Drive folder once the video has arrived. Progress is visible, and
the one-time model download is gated by a consent prompt.

**Independent Test**: quickstart.md Scenario 1. Drive's video is
byte-identical to the local file, `Sermon.srt` sits next to it in Drive and
next to the local video, timings are within 1 s, and a second upload creates
`Sermon (2).srt`.

### Tests for User Story 1 ⚠️ write first, confirm they fail

- [X] T013 [P] [US1] Write `internal/modelfetch/modelfetch_test.go` (the shared downloader also used by Feature 006, research.md §14 there) with an `httptest` server: a full download is verified and renamed; an interrupted download resumes with a `Range` header from the `.part` size; a size or SHA-256 mismatch deletes the `.part` file and never produces the final file; a final-named file passes a size-only startup check; insufficient free disk space fails before downloading.
- [X] T014 [P] [US1] Write `internal/captions/wav_test.go`: parse a 16 kHz mono 16-bit header; split a generated 25-minute WAV into pieces of about 10 minutes, each cut landing in the quietest 100 ms window within ±5 s of the mark (place silence at known offsets); piece start offsets are reported exactly; reads use a fixed buffer regardless of file length.
- [X] T015 [P] [US1] Write `internal/captions/srt_test.go`: parse an SRT; shift by a piece offset; merge pieces with continuous renumbering; collapse 3 or more consecutive identical cues into one spanning start-to-end; collapse a phrase repeated 3+ times within one cue (the measured 38:58 loop); an all-empty transcript reports "no speech"; output is valid SubRip (`HH:MM:SS,mmm --> HH:MM:SS,mmm`).
- [ ] T016 [P] [US1] Write `internal/captions/localcopy_test.go`: saves `<video base>.srt` next to the video; an existing name gets `(2)`, `(3)`, …; an unwritable folder falls back to a given Downloads folder; an existing file is never overwritten (FR-022, FR-014).
- [ ] T017 [P] [US1] Write `internal/drive/captionfile_test.go` with an `httptest` server: `FindCaptionFile` finds a non-trashed file tagged `ballastCaptionFor=<id>` in the folder; `ChooseCaptionName` returns `Sermon.srt` or the next free `Sermon (N).srt` from a name query; `UploadCaptionFile` sends a multipart create with `parents`, `name`, `mimeType=application/x-subrip`, and the `appProperties` tag, and returns id and `webViewLink` (research.md §6).
- [ ] T018 [P] [US1] Write `internal/captions/worker_test.go` for the happy path, using the fake engine (T004), a temp DB, a fake extractor that writes a WAV, and a fake Drive uploader:
  - a job runs `downloading_model → extracting_audio → transcribing → waiting_for_video`;
  - the local copy is saved before `waiting_for_video`;
  - it moves to `uploading_captions → done` only after `VideoSucceeded(uploadID)`;
  - `captions:updated` fires on every phase change and at most once per second for progress;
  - a job with consent `unasked` stays in `waiting/awaiting_consent` and emits `captions:consent-needed` once;
  - two jobs run one at a time.

### Implementation for User Story 1

- [X] T019 [P] [US1] Implement the generic downloader in `internal/modelfetch/modelfetch.go` (`Spec{URL, Size, SHA256, FileName}`, `Present(dir, spec)`, `Fetch(ctx, dir, spec, progress)`), and `internal/captions/model.go` holding only the speech model's `modelfetch.Spec`: pinned model URL (a Hugging Face `resolve/<revision>/ggml-large-v3-turbo.bin`), with size and SHA-256 constants recorded by doing one real download and hashing it (never made up). Also: `.part` resume via `Range`, verify then rename, size-only `ModelPresent()`, a disk-space precheck, retries using `drive.NewBackoffPolicy`, and a progress callback (research.md §2, §7; makes T013 pass).
- [ ] T020 [P] [US1] Implement `internal/captions/extract.go`: run `/usr/bin/afconvert -f WAVE -d LEI16@16000 -c 1 <video> <out.wav>` under a context, lower its priority with `syscall.Setpriority` (nice 10) after start, and map the "Couldn't open input file" failure to "This video has no audio track" (research.md §3, §9).
- [X] T021 [P] [US1] Implement `internal/captions/wav.go`: header parsing, quiet-point splitting into piece WAV files in the job's work folder, and piece offsets (research.md §4; makes T014 pass).
- [X] T022 [P] [US1] Implement `internal/captions/srt.go`: parse, shift, merge, repeat collapse, and the "no speech" check (research.md §5; makes T015 pass).
- [ ] T023 [P] [US1] Implement `internal/captions/localcopy.go`: `SaveLocalCopy(videoPath, srtPath, downloadsDir) (savedPath, error)` (makes T016 pass).
- [ ] T024 [P] [US1] Implement `internal/captions/transcribe.go`: run `whisper-cli -m <model> -f <piece.wav> -l <en|auto> -osrt -of <out> -pp -sns -mc 0 -t <threads>` (`-mc 0` per research.md §5's measured loop) under a context (killing the process on cancel), lower its priority, parse `progress = N%` from its output into a callback, and return the piece's SRT path. A non-zero exit becomes an error with the last stderr line.
- [ ] T025 [P] [US1] Implement `internal/drive/captionfile.go`: `FindCaptionFile`, `ChooseCaptionName`, and `UploadCaptionFile` with the existing `drivev3` service, including the `ballastCaptionFor` app property (makes T017 pass).
- [ ] T026 [US1] Implement `internal/captions/worker.go`: a `Worker` with one goroutine taking jobs from the DB one at a time; it calls T019–T025 and records each phase and the `pieces_done` checkpoint after every piece.
  - In order: consent gate, model download, extract, split, transcribe pieces, merge and clean, local copy, wait for the video, adopt-or-upload to Drive, done.
  - On finishing, failing, or cancelling, it deletes the work folder.
  - It emits throttled `captions:updated` events.
  - It exposes `Enqueue(uploadID)`, `VideoSucceeded(uploadID)`, `AnswerConsent(bool)`, and `Cancel(uploadID)`.
  - It holds the `heavywork` lock (T011a) only while `whisper-cli` pieces run, not while downloading, extracting, or uploading, so Feature 006's summary model never runs alongside it.
  - It calls optional hooks `OnTranscriptReady(uploadID, srtPath)` once the merged transcript is saved, and `OnNoTranscript(uploadID, reason)` when a job ends without one (failed, cancelled, or no speech). Feature 006 subscribes to these; with no subscriber they do nothing.
  - Makes T018 pass (depends on T009, T012, T019–T025).
- [ ] T027 [US1] Wire captions into `app.go`:
  - Start the `captions.Worker` in `startup`.
  - In `startNewUpload` (used by `UploadStart`/`UploadRetry`), create the CaptionJob when the file is captionable, `CaptionsEnabled()`, and `Availability()` is ok, with `language` copied from settings. For unsupported extensions, create it straight into `failed` ("Captions aren't supported for .mkv files yet").
  - Call `worker.VideoSucceeded` after `SetUploadSucceeded` in `runUpload` and in `adoptLandedUpload`.
  - Add the bound methods `CaptionsGetSettings`, `CaptionsAnswerModelDownload`, `CaptionsGetJob`, and `CaptionsShowLocalCopy` (`open -R <path>`).
  - Add the optional `caption` field to `UploadListItemDTO` in `UploadListRecent`.
  - Follow contracts/wails-bindings.md exactly. The upload itself must not wait on anything captions-related (FR-006).
- [ ] T028 [US1] Regenerate the Wails bindings (`wails generate module`) so `frontend/wailsjs/go/main/App.d.ts`/`App.js` and the models include the new methods and DTOs.
- [ ] T029 [P] [US1] Create `frontend/src/api/captions.ts`, wrapping the new bindings and typing `CaptionSettingsDTO` and `CaptionJobDTO`.
- [ ] T030 [US1] Extend `frontend/src/ui/live.ts`: keep caption state per upload (from `UploadListRecent`'s `caption` and `captions:updated` events), and on `captions:consent-needed` open the consent prompt.
- [ ] T031 [US1] Add to `frontend/src/ui/components.ts`:
  - a caption status line, with copy per the contract's UI section, for example "Captions: transcribing — 42%", "Captions ready on this Mac — waiting for the video to finish", "Captions ready";
  - the consent prompt ("Download and caption" / "Not now"), showing the model size.
- [ ] T032 [US1] Show the caption line under each video row in `frontend/src/ui/views/transfers.ts` and `frontend/src/ui/views/home.ts`. Add "Open captions in Drive" (`driveFileLink`) and "Show in Finder" (`CaptionsShowLocalCopy`) to the transfer details.
- [ ] T033 [US1] Extend `mock_e2e.go` with an outcome that scripts a caption job through every phase (consent-needed, progress, done). Write `frontend/tests/captions.spec.ts` covering the consent prompt appearing once, the caption line changing per phase, and both detail actions appearing when done.

**Checkpoint**: US1 is fully functional. Run quickstart Scenario 1 on a real Apple-silicon Mac.

---

## Phase 4: User Story 2 - Captioning never puts the video upload at risk (Priority: P1)

**Goal**: Every captioning failure is isolated and explained. Cancelling an
upload stops its captioning. Captioning survives restarts, and the upload
behaves identically with captions on or off.

**Independent Test**: quickstart.md Scenario 2. No audio, `.mkv`, a PDF,
Wi-Fi drop, cancel mid-transcription, cancel after the transcript, and a
missing engine all leave the video upload unaffected, with a specific
reason shown.

### Tests for User Story 2 ⚠️ write first, confirm they fail

- [ ] T034 [P] [US2] Extend `internal/captions/worker_test.go` with failure cases:
  - fake-engine `crash`: job `failed` with a note, and the work folder is deleted;
  - extractor "no audio": failed with "This video has no audio track";
  - `empty`: done with the note "No speech found in this video", with no local copy and no Drive upload;
  - `Cancel` during `hang`: the helper process is killed within 2 s, the job is `cancelled`, the folder is deleted, and nothing is uploaded;
  - `Cancel` in `waiting_for_video`: `cancelled`, the local copy is kept, and nothing is uploaded;
  - an upload that fails: the job becomes `cancelled`;
  - Drive upload errors (signed out, quota): the job stays `uploading_captions` and retries.
- [ ] T035 [P] [US2] Write `internal/captions/recovery_test.go`: a job left `in_progress/transcribing` with `pieces_done=2` resumes at piece 3 after a new `Worker` starts. If `audio.wav` is missing, it goes back to `extracting_audio`. Leftover work folders of ended jobs are removed at startup (FR-012, FR-013).

### Implementation for User Story 2

- [ ] T036 [US2] Implement the failure paths, `Cancel`, and Drive-error retry in `internal/captions/worker.go`, making T034 pass. Every `failed` transition carries a plain-language note (FR-007). A cancel kills any running helper process via its context.
- [ ] T037 [US2] Implement startup recovery and orphan-folder cleanup in `internal/captions/worker.go` (makes T035 pass).
- [ ] T038 [US2] Wire cancellation into `app.go`:
  - `UploadCancel` calls `worker.Cancel(id)` after `stopUpload`;
  - a terminal upload failure in `runUpload` (`SetUploadFailed`) calls `worker.Cancel(id)`;
  - `UploadDelete` relies on the storage cascade (T009) and also calls `worker.Cancel(id)`.
  None of this may change the upload's own status handling.
- [ ] T039 [P] [US2] Add a free-disk-space check before extraction to `internal/captions/extract.go`. It needs the audio duration × 32,000 bytes/s plus 20%, estimated from the file size before `afconvert` runs. Failing it gives "Not enough free disk space to make captions". Test it in `internal/captions/extract_test.go` with an injectable free-space function.
- [ ] T040 [US2] Extend `frontend/tests/captions.spec.ts` via `mock_e2e.go` outcomes: a failed caption job shows "Captions couldn't be made — <reason>" while the upload row still shows its own succeeded status, and a non-video upload shows no caption line.

**Checkpoint**: US1 and US2 together are the shippable MVP. Run quickstart Scenarios 2 and 4.

---

## Phase 5: User Story 3 - Turn automatic captions on or off (Priority: P2)

**Goal**: A Settings switch (default on) and an English/Automatic language
choice, both remembered. Explicit "not available" behaviour on Intel Macs,
Windows, and Linux.

**Independent Test**: quickstart.md Scenario 5.

### Tests for User Story 3 ⚠️ write first, confirm they fail

- [ ] T041 [P] [US3] Write `app_captions_test.go` (package `main`, temp DB): `CaptionsSetEnabled(false)` stops new jobs from being created on `UploadStart` (no job row is created); `CaptionsSetEnabled(true)` with consent `declined` flips it to `accepted`; `CaptionsSetLanguage("fr")` is rejected; a job copies the language at creation and keeps it after the setting changes; `CaptionsAnswerModelDownload(false)` cancels every `awaiting_consent` job with the note "Captions were turned off" and turns captions off.

### Implementation for User Story 3

- [ ] T042 [US3] Add `CaptionsSetEnabled` and `CaptionsSetLanguage` to `app.go`, with the consent rules from contracts/wails-bindings.md. Both reject when `Availability()` is not ok. Makes T041 pass.
- [ ] T043 [US3] Regenerate the Wails bindings (as in T028) and add the two wrappers to `frontend/src/api/captions.ts`.
- [ ] T044 [US3] Add an "Automatic captions" section to `frontend/src/ui/views/settings.ts` containing:
  - the switch and the "Caption language" choice (English / Automatic);
  - the FR-015 notice: made on this computer, lower accuracy for languages other than English including Twi and Ga, saved as a separate `.srt` next to the video that can be attached in Drive's player.
  When `available` is false, disable both controls and show `unavailableReason` (FR-017).
- [ ] T045 [US3] Extend `frontend/tests/captions.spec.ts`: the settings persist across `DebugRestart`, and a mock outcome with `available=false` shows the unavailable message with disabled controls.

**Checkpoint**: US1–US3 complete.

---

## Phase 6: User Story 4 - Embedded caption track investigation (Priority: P3)

**Goal**: A written yes/no finding on whether Drive's player shows a caption track embedded without re-encoding (SC-008). No shipped code.

**Independent Test**: The findings file exists and answers the question with evidence.

- [ ] T046 [US4] Carry out research.md §13 on one short and one multi-hour video (copy streams without re-encoding with `-c:s mov_text`, compare per-stream MD5s, upload, and check Drive's player), then write `specs/005-automatic-video-captions/findings-embedded-captions.md` with the result, screenshots, the remux time and extra disk needed for the long file, and a recommendation.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T047 [P] Update `README.md`: what captions do, Apple-silicon-only availability, `scripts/build-engines.sh`, `BALLAST_WHISPER_CLI` for `wails dev`, and where the model and work folders live.
- [ ] T048 [P] Add the captions feature to the `[Unreleased]` section of `CHANGELOG.md`.
- [ ] T049 [P] Logging audit: make sure no transcript text, SRT content, or model URL query string is logged anywhere in `internal/captions` or `app.go` (grep `logging.` calls). Logs carry only IDs, phases, counts, durations, and reasons (Constitution IV).
- [ ] T050 Run quickstart.md Scenarios 1–6 on the maintainer's Apple-silicon Mac with a real 3-hour sermon. Record transcription time against SC-003, peak memory at 10 minutes vs 2 hours (SC-006), upload time with captions off vs on (SC-004), and observations on music sections and Twi/Ga passages. If SC-003 fails on 8 GB, report it and propose the `large-v3-turbo-q8_0` fallback for the maintainer's decision (research.md §2) instead of switching silently.
- [ ] T051 Final checks: `gofmt -l .`, `go vet ./...`, `go test ./...`, `cd frontend && npx tsc --noEmit`, and the Playwright suite against `wails dev` with `BALLAST_E2E_MOCK=1`, using a separate test account and data folder.

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: none. T001 is needed only for real-engine runs (T050), not for unit tests.
- **Foundational (Phase 2)**: after Setup. Blocks every story.
- **US1 (Phase 3)**: after Foundational.
- **US2 (Phase 4)**: after US1's worker (T026) and app wiring (T027), because it hardens the same worker.
- **US3 (Phase 5)**: after Foundational and T027 (it extends `app.go`'s captions methods). It can run in parallel with US2.
- **US4 (Phase 6)**: independent of all code. Only needs one finished `.srt` (any source), so it can run at any time.
- **Polish (Phase 7)**: after the stories being shipped.

### Within each story

Tests first (and failing) → helpers (`model`, `extract`, `wav`, `srt`, `localcopy`, `transcribe`, `captionfile`) → `worker.go` → `app.go` wiring → bindings → frontend → Playwright.

### Story completion order

```text
Setup → Foundational → US1 ─┬─► US2 ─┐
                            └─► US3 ─┴─► Polish
US4 (anytime)
```

---

## Parallel Opportunities

- **Setup**: T002, T003, T004 together.
- **Foundational**: T006, T008, T010, T011, T012 together. Then T007 and T009.
- **US1 tests**: T013–T018 together (separate files).
- **US1 helpers**: T019–T025 together (separate files, each paired with its own test). Only T026 joins them.
- **US2**: T034, T035, T039 together.
- **US3**: T041 can be written while US2 is in progress.
- **Polish**: T047, T048, T049 together.

### Example: US1 helpers in parallel

```text
Task: "Implement internal/captions/model.go (T019)"
Task: "Implement internal/captions/extract.go (T020)"
Task: "Implement internal/captions/wav.go (T021)"
Task: "Implement internal/captions/srt.go (T022)"
Task: "Implement internal/captions/localcopy.go (T023)"
Task: "Implement internal/captions/transcribe.go (T024)"
Task: "Implement internal/drive/captionfile.go (T025)"
```

---

## Implementation Strategy

### MVP first (US1 + US2)

1. Phase 1 Setup, then Phase 2 Foundational.
2. Phase 3 (US1): stop and run quickstart Scenario 1 on the real Mac.
3. Phase 4 (US2): stop and run Scenarios 2 and 4.
4. US1 and US2 together are the shippable MVP. US2 shares US1's P1 priority because the spec makes upload safety a precondition for shipping captions at all.

### Incremental delivery

1. MVP (US1 + US2): captions on by default with consent, safe failure handling.
2. US3: user control over on/off and language.
3. US4: the finding that decides whether a follow-up feature can remove the manual "attach captions in Drive" step.
4. Then Feature 006 (sermon summaries) builds on the transcript this feature produces.

### Notes

- [P] = different files, no dependencies on unfinished tasks.
- Commit after each task or logical group (one sentence, no co-author trailer, per the maintainer's preference).
- Never change `internal/drive`'s resumable upload loop in this feature (plan.md Constitution Check, Principle II/VI).
- The model's revision, size, and SHA-256 (T019) and the whisper.cpp tag (T001) must come from the real artifacts, never invented.
