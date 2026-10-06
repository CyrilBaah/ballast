# Quickstart: Validating Automatic Sermon Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

## Prerequisites

- Feature 005 (captions) working on an Apple-silicon Mac (see its
  quickstart).
- An Anthropic API key with a little credit, from the Anthropic console.
- Test videos: the real 43-minute sermon (`Paster Joseph Ayertey.mp4`),
  plus one 3-hour+ recording.
- The reference summary in `internal/summaries/testdata/reference-2026-10-04.md`
  (the version shared to the church group).

**Warning**: `wails dev` uses your real Ballast database and keychain. Use
a test Drive folder.

## Automated tests

```sh
go test ./...
cd frontend && npx tsc --noEmit
```

The Go suite covers the following without spending money; every model
call is a fake `Summarizer`:
- building transcript text from an SRT (research.md §10);
- quote verification: kept, dropped, and none left (§4);
- uncertain references going into the "Check before sharing" list and
  never into `share_message` (§5);
- the share message having no emojis and no `[mm:ss]` markers (FR-002a);
- the error mapping 401/402/403/429/529/refusal/max_tokens to the right
  outcome and retry behaviour (§8), using an `httptest` server that returns
  the API's error bodies;
- the SummaryJob state machine: wait for captions, retry, cancel, restart;
- HTML and Markdown rendering;
- Google Doc naming, `(2)` handling, and adopting an existing doc by tag (§11).

## Scenario 1: Set up and summarise a real sermon (Stories 1 and 3)

1. Settings → Sermon summaries is **off** by default. Upload a short video
   and confirm no summary line and no traffic to `api.anthropic.com`.
2. Enter a wrong key. **Expect**: "Key rejected", nothing stored.
3. Enter a real key. **Expect**: "Key works" within a few seconds, shown
   masked. Turn summaries on (SC-008: under 2 minutes in total).
4. Upload `Paster Joseph Ayertey.mp4`.
5. **Expect**: the summary line moves through waiting for captions →
   writing → "ready on this Mac". `Paster Joseph Ayertey — Summary.md`
   appears next to the video before the upload finishes (FR-007), and after
   the upload a Google Doc `Paster Joseph Ayertey — Summary` appears in the
   Drive folder.
6. **Review against the reference**: all seven parts are present. Main
   points cover the Joseph story and the five actions. Matthew 28:18–20
   and Hebrews 11:24–26 are listed under "Check before sharing" with what
   was heard. Every quote matches a transcript passage within 30 s
   (SC-002).
7. Copy the share message into a WhatsApp chat with yourself. **Expect**:
   bold renders, no emojis, no timestamps, no announcements (thanks, food
   event, retired mothers), ready to send without edits (SC-009).
8. Record `input_tokens` and `output_tokens` from the job. Work out the
   cost at $4 / $20 per million tokens and update the Settings estimate if
   it falls outside $0.10–$0.40 (research.md §9).
9. **Expect**: the summary is ready within 5 minutes of captions finishing
   (SC-004).

## Scenario 2: Failures never affect the upload or captions (Story 2)

1. Revoke the key in the Anthropic console, then upload a video.
   **Expect**: upload and captions succeed, and the summary shows "Your AI
   key was rejected or removed". Settings flags the key.
2. Turn Wi-Fi off just as summarising starts. **Expect**: automatic
   retries (`attempts` rises), then a failure with "Try again". Wi-Fi on,
   then Try again. **Expect**: the summary is made without redoing captions.
3. Cancel an upload after its summary's local copy exists. **Expect**:
   nothing in Drive, and the `.md` stays (FR-016).
4. Upload a video with no speech. **Expect**: "No transcript to summarise".
5. Quit Ballast mid-summary and reopen. **Expect**: it resumes (FR-017).

## Scenario 3: Long sermon (SC-003)

Upload the 3-hour+ recording. **Expect**: a summary whose main points
include at least one point from each hour, and a cost within the Settings
estimate.

## Scenario 4: Privacy and secrets (SC-006, SC-007)

1. While summarising, watch traffic. **Expect**: requests to
   `api.anthropic.com` carry only text, with request sizes about the
   transcript's size (kilobytes, not the gigabytes of the video).
2. After setup, run
   `grep -r "sk-ant" ~/Library/Application\ Support/ballast/` and
   `sqlite3 … "select * from setting"`. **Expect**: no key anywhere. The key
   is only in Keychain Access under `ballast` / `anthropic-api-key`.
