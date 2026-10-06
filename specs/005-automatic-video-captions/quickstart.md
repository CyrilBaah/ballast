# Quickstart: Validating Automatic Video Captions

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

These scenarios prove the feature works from start to finish. They refer
to [data-model.md](./data-model.md) and
[contracts/wails-bindings.md](./contracts/wails-bindings.md) rather than
repeating them.

## Prerequisites

- An Apple-silicon Mac (research.md §12).
- The speech engine built: `scripts/build-whisper.sh` (research.md §8),
  then `export BALLAST_WHISPER_CLI=$PWD/build/whisper/whisper-cli` for
  `wails dev`.
- A Google account and your own OAuth client (see README), signed in.
- Test videos (made with Homebrew `ffmpeg`, which is a test tool only):
  - `speech-short.mp4`: 2–5 minutes of clear English speech. Any
    recording you have the rights to works.
  - `no-audio.mp4`: `ffmpeg -f lavfi -i testsrc -t 10 -c:v libx264 no-audio.mp4`
  - `clip.mkv`: any short `.mkv`.
  - A real multi-hour sermon recording of at least 3 hours and at least
    10 GB, for Scenarios 3 and 4.

**Warning**: `wails dev` uses your real Ballast database and keychain.
Use a test Google account and Drive folder.

## Automated tests

```sh
go test ./...        # includes the new internal/captions package tests
cd frontend && npx tsc --noEmit
```

The Go suite must cover, with fakes and no real model:
- splitting a WAV at quiet points (research.md §4)
- shifting and merging per-piece SRTs, and repeat collapsing (§5)
- resuming a model download from a `.part` file, and rejecting a
  mismatched hash (§7), using `httptest`
- choosing a free caption name: `Sermon.srt` → `Sermon (2).srt` (§6)
- adopting an already-uploaded caption file by its tag (§6)
- the CaptionJob state machine, including cancel and restart recovery
  (data-model.md), with a fake helper program standing in for
  `whisper-cli`

## Scenario 1: First video, consent, captions next to the video (Stories 1 and 3)

1. Fresh state: no `models/` folder in the app data folder, and no
   `setting` rows.
2. Upload `speech-short.mp4` to a test folder.
3. **Expect**: the upload starts at once. The consent prompt shows the
   model size. The row shows "Captions: waiting for your answer".
4. Choose "Download and caption".
5. **Expect**: "Captions: downloading speech model — N%", then extracting,
   then transcribing with rising %, then uploading. Finally
   `speech-short.srt` appears in the same Drive folder and the row shows
   "Captions ready" with "Open captions in Drive".
6. **Check**: Drive's copy of the video has the same MD5 as the local file
   (`md5 speech-short.mp4` against the file's `md5Checksum` in Drive):
   SC-001.
7. **Check**: in Drive's player, attach `speech-short.srt` (⋮ → Manage
   caption tracks) and spot-check that the timing is within 1 s: SC-002.
8. Upload `speech-short.mp4` again to the same folder. **Expect**: no
   prompt this time, and `speech-short (2).srt` is created. The first
   caption file is untouched (FR-014).

## Scenario 2: Captions never harm the upload (Story 2)

1. Upload `no-audio.mp4`. **Expect**: the upload succeeds, and captions show
   "Captions couldn't be made — This video has no audio track".
2. Upload `clip.mkv`. **Expect**: the upload succeeds, and captions show
   "Captions aren't supported for .mkv files yet".
3. Upload a PDF. **Expect**: no caption line at all (FR-010).
4. Start a large video upload, then turn Wi-Fi off for 30 s while it is
   transcribing. **Expect**: the upload pauses and resumes exactly as with
   captions off, and transcription keeps going (FR-006).
5. Start a video upload, wait for transcribing, then pause it and Cancel.
   **Expect**: no `whisper-cli` process remains (`pgrep whisper-cli`), the
   work folder is gone, and no `.srt` appears in Drive (FR-011).
6. Rename `BALLAST_WHISPER_CLI` to a bad path and relaunch. **Expect**:
   Settings shows captions as unavailable with a reason; video uploads work
   normally.

## Scenario 3: Multi-hour sermon (SC-003, SC-006, FR-012)

1. Upload the 3-hour+ sermon with captions on and the model already
   downloaded.
2. Watch peak memory of `whisper-cli` and Ballast in Activity Monitor at
   10 minutes in and at 2 hours in. **Expect**: about the same peak
   (SC-006).
3. About halfway through transcription, quit Ballast and reopen it.
   **Expect**: captioning resumes at its saved piece (`pieces_done`), not
   from 0%.
4. **Expect**: total transcription time (excluding the model download)
   is at most the video's running time (SC-003).
5. Open the `.srt`. **Check**: worship-music sections have no runs of
   repeated lines (research.md §5), and Twi/Ga passages are best effort.
   Note quality here for tuning.

## Scenario 4: Upload speed is unaffected (SC-004)

1. Upload the same large video twice over the same connection: once with
   captions off, once on.
2. **Expect**: the captions-on upload's duration is within 5% of the
   captions-off one.

## Scenario 5: Settings and other platforms (Story 3, FR-017)

1. Turn captions off and upload a video. **Expect**: no caption line, and
   no `afconvert` or `whisper-cli` process runs.
2. Turn them on, set the language to Automatic, and upload a video.
   **Expect**: the job's language is `auto`.
3. Quit and relaunch. **Expect**: both settings are kept.
4. On a Windows or Linux build, or with `GOARCH=amd64` on macOS, open
   Settings. **Expect**: "Captions aren't available on this system yet",
   controls disabled, and uploads behave as before.

## Scenario 6: Privacy (SC-007)

While Scenario 1's transcription runs, after the model has downloaded,
watch network traffic, for example with Little Snitch or
`nettop -p $(pgrep whisper-cli)`. **Expect**: `whisper-cli` and
`afconvert` make no network connections. Ballast's only outgoing traffic
is the video upload and, at the end, the small caption upload to Drive.

## Scenario 7: Embedded caption track investigation (Story 4, SC-008)

Follow research.md §13 and write the outcome to
`findings-embedded-captions.md`, with screenshots. Nothing from this
scenario ships.
