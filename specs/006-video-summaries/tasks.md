---

description: "Task list for Automatic Video Summaries"
---

# Tasks: Automatic Video Summaries

**Input**: Design documents from `/specs/006-video-summaries/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wails-bindings.md, quickstart.md (all present). Feature 005 (captions) is built: its transcript hooks (`OnTranscriptReady` / `OnNoTranscript`), the `heavywork` lock, the `modelfetch` downloader, `scripts/build-engines.sh`, and the `setting` table all exist.

**Tests**: Included, written before each implementation task and confirmed failing first. plan.md commits to a fake chat-completions server and a fake `Summarizer`, so no test ever loads a real model.

**Organization**: Tasks are grouped by user story:
- **US1 (P1)**: every video gets a summary next to it in Drive.
- **US2 (P1)**: summaries never put the upload or captions at risk.
- **US3 (P2)**: summaries can be turned on or off.
- **US4 (P2)**: the summary can be used before the upload finishes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on unfinished tasks)
- **[Story]**: US1–US4

## Path Conventions

- Go: `app.go`, `app_summaries.go`, `internal/summaries/`, `internal/drive/`, `internal/storage/`, `internal/events/`
- Frontend: `frontend/src/`, `frontend/tests/`, generated `frontend/wailsjs/`
- Build: `scripts/build-engines.sh`

---

## Phase 1: Setup and model choice

- [X] T001 Extend `scripts/build-engines.sh` to also build llama.cpp's `llama-server` from a pinned release tag (record it), as a static arm64 binary with Metal (`-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DBUILD_SHARED_LIBS=OFF -DLLAMA_CURL=OFF`), into `build/engines/llama-server`, and copy it into the app bundle with `--bundle` (research.md §15).
- [X] T002 [P] Create the `internal/summaries` package with `doc.go`, and a fake llama-server for tests: an `httptest` server speaking `/health` and `/v1/chat/completions`, returning scripted JSON per request, recording each request body, and able to return invalid JSON, hang, or 500 on demand (`internal/summaries/fake_server_test.go`).
- [X] T003 [P] Add the reference summaries to `internal/summaries/testdata/`: *(Kept in git-ignored `testdata/private/`: the sermon and the documentary narration aren't ours to publish in a public repo. Unit tests use short made-up transcripts instead.)*
  - `reference-sermon-2026-10-04.md`: the summary shared to the church group on 2026-10-06;
  - `transcript-sermon-2026-10-04.srt`: the real 43-minute transcript;
  - `transcript-aksum.srt`: Feature 005's real output for `Aksum.mp4`;
  - `reference-aksum.md`: written by hand from that transcript.
- [ ] T004 **Model choice (quickstart Scenario 0) — maintainer approval required.**
  1. List 2–3 candidate open instruction-tuned GGUF models (3–8B, 4-bit, ≤ about 5 GB, licence allowing free use), checking that each exists on Hugging Face (repository and file names, not recalled from memory).
  2. Run each with Homebrew `llama-server` on both reference transcripts, using the T017 schema and T022 prompt.
  3. Record peak memory, time, JSON validity, quotes passing the transcript check, invented references, no Bible section for Aksum, and a side-by-side read against the references, in research.md §2.
  4. **Stop for the maintainer to pick one.** Then pin the winner's revision URL, size, and SHA-256 in `internal/summaries/model.go` (T025).

---

## Phase 2: Foundational (blocks all stories)

- [X] T005 Add the `summary_job` table to `internal/storage/schema.go` per data-model.md: `upload_id UNIQUE REFERENCES upload(id) ON DELETE CASCADE`, CHECK constraints on `status`/`phase`/`engine`.
- [X] T006 [P] Write `internal/storage/summary_test.go`, mirroring `caption_test.go`: create/one-per-upload; validation (ended means no phase; failed needs a note; drive id and link set together); transitions only while active; `parts_done <= part_count`; `attempts`; `ListActiveSummaryJobs`; cascade delete with the upload.
- [X] T007 Implement `internal/storage/summary.go` (`SummaryJob`, statuses and phases from data-model.md, CRUD, transitions, `ResetSummaryForRetry` which sets attempts to 0 and returns to `waiting_for_engine`) to make T006 pass.
- [X] T008 [P] Add `summaries_enabled` (default `true`) and `summary_model_consent` (default `unasked`) with typed helpers to `internal/storage/setting.go`, with tests in `setting_test.go`.
- [X] T009 [P] Add `SummariesConsentNeeded = "summaries:consent-needed"`, `SummariesUpdated = "summaries:updated"`, a `SummaryJob` payload matching `SummaryJobDTO`, and emit helpers to `internal/events/events.go`.
- [X] T010 [P] Write `internal/summaries/engine_test.go` and implement `engine.go` plus the platform files. `Availability()` finds `llama-server` via `BALLAST_LLAMA_SERVER` or next to the executable, is unavailable on anything but darwin/arm64, and also requires captions to be available ("Summaries need captions, which aren't available on this system yet").

**Checkpoint**: `go test ./internal/storage ./internal/summaries` passes.

---

## Phase 3: User Story 1 — every video gets a summary next to it in Drive (P1) 🎯 MVP

**Independent Test**: quickstart Scenario 1. A sermon and `Aksum.mp4` each get "<name> — Summary" in Drive with every core part. The sermon's summary has its Bible references and "Check before sharing"; Aksum's has no Bible section. The share message pastes cleanly.

### Tests ⚠️ write first, confirm they fail

- [X] T011 [P] [US1] `internal/summaries/transcript_test.go`:
  - SRT becomes `[mm:ss] text` lines (`[h:mm:ss]` past an hour);
  - a transcript under about 9,000 tokens is a single part;
  - a longer one splits into parts of about 20 minutes on cue boundaries;
  - token estimate is about 4 characters per token (research.md §4, §10).
- [X] T012 [P] [US1] `internal/summaries/verify_test.go`:
  - quotes are kept when 80% or more of `source_text` words appear in order within ±30 s of `start_seconds`, and dropped otherwise; "No quotes could be verified" when none survive;
  - references whose `heard_as` isn't in the transcript are dropped;
  - a `bible` reference with an unknown book goes to check-before-sharing;
  - `certain: false` goes to check-before-sharing (research.md §5, §6).
- [X] T013 [P] [US1] `internal/summaries/share_test.go`: emoji are stripped; `[12:03]` and `(1:02:03)` style timestamps are stripped; a share message mentioning "transcript" is rejected as unusable (research.md §12).
- [X] T014 [P] [US1] `internal/summaries/render_test.go`:
  - HTML (for the Google Doc) and Markdown (local copy) contain every core part;
  - the references and takeaways sections are left out entirely when empty (FR-002b);
  - "Check before sharing" appears only when there are guesses, and never inside the share message;
  - quote times are shown as `h:mm:ss`;
  - HTML escapes model text.
- [X] T015 [P] [US1] `internal/summaries/local_test.go`, against the fake server (T002):
  - the request uses `response_format` `json_schema` with the §3 schema;
  - a short transcript makes a single request;
  - a long one makes N part requests and one combine request;
  - a schema-valid but semantically empty answer counts as unusable.
- [X] T016 [P] [US1] `internal/drive/summarydoc_test.go`, with a fake Drive like `captionfile_test.go`:
  - find by `ballastSummaryFor` tag;
  - free name `Sermon — Summary`, then `Sermon — Summary (2)`;
  - create with `mimeType: application/vnd.google-apps.document`, HTML media, and the tag.
- [ ] T017 [P] [US1] `internal/summaries/worker_test.go` happy path, with a fake `Summarizer`, a temp DB, a fake uploader, and a fake model fetch. Ends with a local `.md` next to the video and "<name> — Summary" uploaded. Phases run in this order:
  1. `waiting_for_captions`, then on `TranscriptReady`: `awaiting_consent`
  2. consent, then `downloading_model`
  3. `waiting_for_engine`, which holds until `heavywork` is free
  4. `summarising`
  5. `waiting_for_video`
  6. `uploading_summary`
  7. `done`

### Implementation

- [X] T018 [P] [US1] `internal/summaries/summary.go`: the `Summary`, `Reference`, `Quote`, `MainPoint`, and `PartNotes` structs; the JSON schemas from research.md §3–§4; the `Summarizer` interface (§13).
- [X] T019 [P] [US1] `internal/summaries/transcript.go` (makes T011 pass).
- [X] T020 [P] [US1] `internal/summaries/verify.go`, including the 66-book list (makes T012 pass).
- [X] T021 [P] [US1] `internal/summaries/share.go` (makes T013 pass).
- [X] T022 [P] [US1] `internal/summaries/prompt.go`: general-purpose system prompts for part notes, combine, and single pass, with no worked example (research.md §12).
- [X] T023 [P] [US1] `internal/summaries/render.go` (makes T014 pass).
- [X] T024 [US1] `internal/summaries/server.go` and `local.go`:
  - start `llama-server -m <model> -c 16384 --host 127.0.0.1 --port <free>`, lower its priority, wait for `/health`, and kill it on ctx cancel or when the job ends;
  - `localSummarizer` runs a single pass, or parts then combine, through `/v1/chat/completions` with `json_schema`;
  - makes T015 pass.
- [ ] T025 [US1] `internal/summaries/model.go`: the pinned `modelfetch.Spec` from T004. Until T004 is approved it holds an obviously unset placeholder that makes `Availability()` report "The summary model hasn't been chosen yet".
- [X] T026 [P] [US1] `internal/drive/summarydoc.go` (makes T016 pass).
- [ ] T027 [US1] `internal/summaries/worker.go`:
  - mirrors `internal/captions/worker.go`: one job at a time, the consent gate, the shared `modelfetch` download (with its own fetch lock), `heavywork.Acquire` around the server's lifetime, `parts_done` checkpoints, verify, share-message rules, render, local `.md` via a shared free-name helper, wait for the video, adopt-or-create the Google Doc, and cleanup;
  - exposes `Enqueue(uploadID)`, `TranscriptReady(uploadID, srtPath)` (which copies the transcript into the job folder), `NoTranscript(uploadID, reason)`, `VideoSucceeded`, `AnswerConsent`, `Cancel`, `Retry`;
  - makes T017 pass.
- [ ] T028 [US1] `app_summaries.go`:
  - start the worker beside captions, including in `DebugRestart`;
  - create a summary job wherever a caption job is created;
  - wire captions' `OnTranscriptReady` / `OnNoTranscript` to the summaries worker;
  - call `VideoSucceeded` where captions do;
  - add the bound methods `SummariesGetSettings`, `SummariesAnswerModelDownload`, `SummariesGetJob`, `SummariesShowLocalCopy`;
  - add `summary` to `UploadListItemDTO`.
- [ ] T029 [US1] Regenerate the Wails bindings (`wails generate module`; revert mode-only runtime changes).
- [ ] T030 [P] [US1] `frontend/src/api/summaries.ts` (plain-data types, like `captions.ts`).
- [ ] T031 [US1] `frontend/src/ui/live.ts`: summary state per upload, the `summaries:*` events, and the consent prompt (also derived from saved state on load, as captions does).
- [ ] T032 [US1] `frontend/src/ui/summaries.ts`, plus `transfers.ts` and `home.ts`:
  - a summary line under the caption line, using the contract's wording;
  - the download prompt;
  - "Open summary in Drive" and "Show summary in Finder" detail actions.
- [ ] T033 [US1] `frontend/tests/summaries.spec.ts`: event-driven checks of the prompt, the line per phase, and the detail actions, in the same style as `captions.spec.ts`.

**Checkpoint**: US1 works end to end with the pinned model. Run quickstart Scenario 1.

---

## Phase 4: User Story 2 — summaries never put the upload or captions at risk (P1)

### Tests ⚠️

- [ ] T034 [P] [US2] Extend `internal/summaries/worker_test.go`:
  - an unusable answer is retried up to 3 attempts, then fails with "The summary model couldn't produce a usable summary";
  - a request over 20 minutes counts as an attempt (shortened in tests);
  - a server that won't start or load triggers one re-download, then fails "The summary model couldn't be loaded";
  - the server process exiting with an out-of-memory message fails with the memory note;
  - `Cancel` kills the server;
  - a cancel after the local `.md` keeps it and uploads nothing;
  - `NoTranscript` cancels with "No speech to summarise";
  - an upload failure cancels the job;
  - Drive errors and being signed out retry without failing;
  - `Retry` reuses the transcript.
- [ ] T035 [P] [US2] `internal/summaries/recovery_test.go`: resume at `parts_done`; `summary.json` present means continue at `waiting_for_video`; orphaned work folders are cleaned up.

### Implementation

- [ ] T036 [US2] Failure paths, retry caps, and `Cancel` in `worker.go` (makes T034 pass).
- [ ] T037 [US2] Restart recovery and orphan cleanup in `worker.go` (makes T035 pass).
- [ ] T038 [US2] `app_summaries.go`: cancel summaries wherever `cancelCaptions` runs; add the `SummariesRetry` bound method; add a "Try again" action on failed summary lines in `transfers.ts`.
- [ ] T039 [US2] Extend `summaries.spec.ts`: a failed summary shows its reason and Try again, and the upload and caption lines are unchanged.

---

## Phase 5: User Story 3 — turn summaries on or off (P2)

- [ ] T040 [P] [US3] `app_summaries_test.go`:
  - on by default;
  - off means no summary job is created;
  - declining the download turns summaries off and cancels waiting jobs;
  - turning back on accepts consent and starts a background download (stubbed);
  - turning on is rejected when unavailable.
- [ ] T041 [US3] Add `SummariesSetEnabled` to `app_summaries.go` (makes T040 pass); regenerate bindings.
- [ ] T042 [US3] Add a "Video summaries" section to `frontend/src/ui/views/settings.ts`, with the switch and the FR-014 notice, and the unavailable reason when it applies.
- [ ] T043 [US3] Extend `summaries.spec.ts` for the Settings section, including the unavailable state.

---

## Phase 6: User Story 4 — use the summary before the upload finishes (P2)

- [ ] T044 [US4] Add a test to `worker_test.go`: with the video still uploading, the local `"<name> — Summary.md"` exists as soon as summarising ends; nothing is in Drive until `VideoSucceeded`; a cancel keeps the `.md`. (The behaviour is built in T027; this task proves FR-007 and FR-016 on their own.)

---

## Phase 7: Polish

- [ ] T045 [P] Update `README.md` (summaries, `BALLAST_LLAMA_SERVER`, model location) and `CHANGELOG.md`.
- [ ] T046 [P] Logging audit: no transcript, summary, or share-message text in any log line from `internal/summaries` or `app_summaries.go`.
- [ ] T047 Run quickstart Scenarios 1–4 for real (sermon, `Aksum.mp4`, a lecture, a 3-hour video). Record time against SC-004, memory, and quality against the references. Confirm `whisper-cli` and `llama-server` never run at once (SC-007).
- [ ] T048 Final checks: gofmt, vet, `go test -race ./...`, tsc, Windows/Linux builds.

---

## Dependencies & Execution Order

- **Setup**: T001–T003 have no dependencies. T004 (the model choice) needs T018 and T022 to exist in draft form, and **blocks only real runs** (T025 pin, T047). Unit tests never need it.
- **Foundational** (T005–T010): after Setup; blocks every story.
- **US1**: after Foundational. Tests T011–T017 first, then T018–T026 (mostly parallel), then T027, then T028–T033.
- **US2**: after T027/T028. **US3**: after T028; can run in parallel with US2. **US4**: after T027.
- **Polish**: last.

```text
Setup ─► Foundational ─► US1 ─┬─► US2 ─┐
   └─ T004 (model, approval) ──┤        ├─► Polish (T047 needs T004)
                               ├─► US3 ─┤
                               └─► US4 ─┘
```

## Parallel Opportunities

- T002, T003; T006, T008, T009, T010.
- US1 tests T011–T017 together.
- US1 helpers T018–T023 and T026 together.
- US2 tests T034 and T035 together; US3's T040 alongside US2.

## Implementation Strategy

1. **MVP: US1 + US2**, built against fakes, so everything is ready the moment the model is chosen.
2. **T004 runs in the middle**: it is the only step needing large downloads (about 2–5 GB per candidate) and the maintainer's decision.
3. **Then US3 and US4**, then a real run of the quickstart.
4. Commit after each task (one-sentence messages, no co-author trailer).
