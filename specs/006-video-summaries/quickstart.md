# Quickstart: Validating Automatic Video Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

## Prerequisites

- Feature 005 (captions) working on an Apple-silicon Mac.
- `export BALLAST_LLAMA_SERVER=/opt/homebrew/bin/llama-server` for
  `wails dev` (Homebrew llama.cpp; research.md §15).
- Test videos of different kinds:
  - the real 43-minute sermon `Paster Joseph Ayertey.mp4`;
  - the 13-minute non-religious `Aksum.mp4`;
  - a lecture or meeting recording of your choice;
  - a 3-hour+ recording.
- The reference summaries in `internal/summaries/testdata/`:
  `reference-sermon-2026-10-04.md` and `reference-aksum.md`.

**Warning**: `wails dev` uses your real Ballast database and keychain. Use
a test Drive folder.

## Automated tests

```sh
go test ./...
cd frontend && npx tsc --noEmit
```

The Go suite uses a fake `Summarizer` and a fake `llama-server` (an
`httptest` server that speaks the chat-completions shape). It never loads
a real model. It covers:
- SRT → transcript text, and splitting into parts on cue boundaries (§4, §10);
- the combine flow and restart from `parts_done`;
- quote verification: kept, dropped, none (§5);
- uncertain and invalid-book references going to "Check before sharing",
  never into the share message (§6);
- share-message enforcement: emojis and timestamps stripped, a "transcript"
  mention rejected (§12);
- failure mapping and retries: bad JSON, timeout, load failure → one
  re-download, out of memory (§8);
- the heavy-work lock: a summary waits while captions run, and captions
  always go first (§7);
- the state machine, cancel, consent decline, Try again;
- HTML/Markdown rendering, and Google Doc naming and adopt-by-tag (§11).

## Scenario 0: Pick the model (research.md §2) — before pinning

First, write `reference-aksum.md` by hand from `Aksum.mp4`'s transcript.
Then, for each candidate model, run `llama-server` by hand on **both**
transcripts with the schema and prompt, and record:
- peak memory;
- time taken;
- whether the JSON is valid;
- how many quotes pass the check;
- any invented references;
- a side-by-side read of each result against its reference summary;
- that `Aksum.mp4`'s summary has no Bible section (SC-010).

Choose the best one that meets SC-004 on the 8 GB M2, get the
maintainer's approval, and pin it.

## Scenario 1: First summaries — a sermon and a non-religious video (Stories 1 and 3)

1. Fresh state. Settings shows summaries **on** with the notice.
2. Upload `Paster Joseph Ayertey.mp4`.
3. **Expect**: captions run first. The summary line reads "waiting for
   captions", then the download prompt appears once with the model size.
   Accept.
4. **Expect**: "downloading model", then "writing — N%", then
   `Paster Joseph Ayertey — Summary.md` next to the video before the upload
   finishes, then the Google Doc in Drive after the upload.
5. **Review against the reference**: all seven parts are present. Main
   points cover the Joseph story and the five actions. Matthew 28:18–20 and
   Hebrews 11:24–26 are under "Check before sharing". Quotes match
   transcript passages within 30 s (SC-002).
6. Paste the share message into WhatsApp. **Expect**: no emojis, no
   timestamps, no announcements, ready to send (SC-009).
7. **Expect**: the summary is ready within 10 minutes of captions finishing
   (SC-004). Record the time.
8. Upload `Aksum.mp4`. **Expect**:
   - `Aksum — Summary` with an overview, main points, quotes, title,
     description, and share message;
   - **no** Bible section and no invented references;
   - a takeaways section only if the narrator actually asks the viewer to
     do something (FR-002b, SC-010).
9. Upload the lecture or meeting recording. **Expect**: a sensible summary
   with no sermon-style wording.

## Scenario 2: Failures never affect the upload or captions (Story 2)

1. Corrupt the model file and upload a video. **Expect**: one automatic
   re-download, then a summary. If the re-download is blocked (Wi-Fi off),
   the summary fails with a reason, and the upload and captions succeed.
2. Point `BALLAST_LLAMA_SERVER` at a bad path. **Expect**: Settings shows
   summaries unavailable; uploads and captions behave normally.
3. Cancel an upload after the local summary exists. **Expect**: nothing in
   Drive, and the `.md` stays.
4. Upload a video with no speech. **Expect**: "No speech to summarise".
5. Quit mid-summary and reopen. **Expect**: it resumes (FR-017).
6. Decline the download prompt (fresh state). **Expect**: summaries switch
   off, and the video and captions are unaffected.

## Scenario 3: Long video (SC-003, SC-004)

Upload the 3-hour+ recording. **Expect**: it is processed in parts, the
main points include at least one point from each hour, and it is ready
within 30 minutes of captions finishing.

## Scenario 4: Zero cost and nothing leaves the Mac (SC-006, SC-007)

1. While summarising, watch network traffic. **Expect**: `llama-server`
   talks only to `127.0.0.1`, and there are no outside connections except
   the final Google Doc upload to Drive.
2. Upload two videos back to back and watch processes. **Expect**:
   `whisper-cli` and `llama-server` never run at the same time.
