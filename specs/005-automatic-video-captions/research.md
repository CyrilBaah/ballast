# Research: Automatic Video Captions

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-06

Each section records a decision, why it was made, and what else was
considered. Measurements marked **measured** were taken on the
maintainer's machine (Apple M2, 8 GB RAM, macOS Darwin 25.6) while writing
this plan; everything else is a starting hypothesis to be confirmed during
implementation, per Constitution Principle III's "constants are hypotheses
until benchmarked" rule.

## §1 Speech engine: whisper.cpp, run as a separate helper program

**Decision**: Use [whisper.cpp](https://github.com/ggml-org/whisper.cpp)'s
`whisper-cli`, built from a pinned release tag as a single static binary
with Apple's GPU (Metal) support compiled in
(`-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DBUILD_SHARED_LIBS=OFF`),
shipped inside the app bundle and run as a child process. Ballast talks to
it only through command-line arguments, files, and its progress output.

Flags used (confirmed against the whisper.cpp CLI documentation):
`-m <model>`, `-f <wav>`, `-l en|auto`, `-osrt`, `-of <out path without extension>`,
`-pp` (print progress), `-sns` (suppress non-speech tokens), `-t <threads>`.
The GPU is used by default on Apple silicon; nothing extra is needed.

**Rationale**:
- Free, MIT-licensed, runs fully offline, no account or key (FR-003).
- A child process keeps the Go side free of cgo. The project already chose
  a pure-Go SQLite driver to avoid cgo (`internal/storage/db.go`), and cgo
  bindings would complicate cross-compiling the Windows and Linux builds,
  which do not get captions in this version.
- Cancelling is clean: killing the child process stops the work at once
  (FR-011). A crash inside the speech engine cannot take Ballast or an
  in-flight upload down with it (FR-006).
- The child can be run at lower CPU priority (§9).

**Alternatives considered**:
- *whisper.cpp Go bindings (cgo)*: tighter integration, but brings in cgo
  and a C++ toolchain for every build, and a crash would kill the app.
- *Apple's built-in speech recognition (SFSpeechRecognizer)*: free and
  built in, but it needs Objective-C/Swift bridging and has historically
  limited on-device language support and long-audio handling. It was not
  tested here; whisper.cpp is the better-known fit for multi-hour files.
- *Cloud speech services (Google Speech-to-Text, Groq, and similar)*:
  ruled out by FR-003: audio would leave the machine, and free tiers are
  capped.

## §2 Speech model: `large-v3-turbo`, downloaded once

**Decision**: Use whisper.cpp's `ggml-large-v3-turbo.bin` (multilingual,
about 1.5 GB), downloaded on first use (FR-019) from the whisper.cpp model
repository on Hugging Face. The download URL is pinned to a specific
repository revision, not `main`, and the file's exact size and SHA-256 are
compiled into Ballast. A file that does not match is never used.

**Rationale**: It is the highest-accuracy model family whisper.cpp offers
(FR-021) and is designed to run much faster than full `large-v3`. The
multilingual version is required for the "Automatic" language choice
(FR-020); English-only `.en` models cannot detect other languages.

**To confirm during implementation**:
- The pinned revision, size, and SHA-256. They are recorded when the
  download code is written; none are invented here.
- Speed on an Apple-silicon Mac against SC-003 (a 3-hour video captioned
  in at most 3 hours). If an 8 GB machine cannot meet it, the fallback is
  `large-v3-turbo-q8_0` (smaller, near-identical accuracy). Switching to it
  would need the maintainer's approval, since the spec's clarification
  chose the larger model.

**Alternatives considered**: `medium` / `small` (smaller and faster, but
weaker on accented English: rejected in clarification); `large-v3`
(slower for little gain over turbo); bundling the model in the app
(rejected in clarification).

## §3 Getting the audio out of the video: macOS's built-in `afconvert`

**Decision**: Extract audio with `/usr/bin/afconvert -f WAVE -d LEI16@16000 -c 1 <video> <out.wav>`,
which produces 16 kHz mono 16-bit WAV, the input whisper.cpp expects.

**Measured**:

| Input | Result |
|---|---|
| 20 s H.264 + AAC `.mp4` | OK, 16 kHz mono Int16, duration exactly 20.0 s |
| Same file as `.mov` | OK |
| `.mp4` with no audio track | Exits with an error ("Couldn't open input file"). Used as the "this video has no audio" signal |
| `.webm` | Not readable (unsupported container) |
| 30-minute `.mp4` | 1.2 s wall time, 11 MB peak memory, 57.6 MB WAV |

So a 3-hour sermon extracts in seconds with flat memory, producing a
temporary WAV of about 345 MB (16,000 samples/s × 2 bytes × 10,800 s).

**Rationale**: It ships with every Mac, so there is no new dependency, no
licensing question, and nothing to download. It streams, so memory stays
flat (FR-013, SC-006), and it reads only the audio track, so the size of
the video stream (13 GB in the motivating case) does not matter.

**Consequence: supported formats**: captioning covers `.mp4`, `.mov`, and
`.m4v` (formats Apple's audio framework reads). Other video formats
(`.mkv`, `.avi`, `.webm`, …) still upload normally, and their caption job
ends as "Captions aren't supported for .mkv files yet" (§10).

**Alternatives considered**: bundling `ffmpeg` (reads every format, but
adds about 50–80 MB, LGPL/GPL licensing review, and a second helper binary
for a long-tail benefit; can be revisited later if users ask for other
formats); decoding MP4/AAC in pure Go (a large, risky amount of code).

## §4 Long videos: transcribe in 10-minute pieces

**Decision**: Split the extracted WAV into pieces of about 10 minutes and
run `whisper-cli` once per piece. Each cut is moved to the quietest
100 ms window within ±5 s of the 10-minute mark, measured by
loudness over the 16-bit samples, so cuts land between words. Each piece's
SRT is shifted by the piece's start time and appended to the job's
combined transcript.

**Rationale**:
- **Flat memory (FR-013, SC-006)**: whisper.cpp holds a file's whole
  audio in memory as 32-bit floats. For 3 hours that is about 690 MB on
  top of the model; a 10-minute piece is about 38 MB whatever the video's
  length.
- **Restart without starting over (FR-012)**: progress is saved after
  each piece, so a crash or quit loses at most one piece's work.
- **Accurate progress (FR-008)**: progress is pieces finished plus the
  current piece's own `-pp` percentage.
- **Fewer runaway repetitions**: large Whisper models can loop and repeat
  a phrase over long music or silence. Restarting the context each
  piece limits how far a loop can spread (§5).

Splitting WAV is simple in Go: fixed-size header, then raw samples, so a
piece is a header plus a byte range, read with a fixed buffer.

**Alternatives considered**: `whisper-cli -ot/-d` (offset and duration)
on the full file still loads all of it into memory; fixed cuts with no
quiet-point search split words at boundaries.

## §5 Transcript cleanup

**Decision**: After each piece, drop cues that whisper.cpp marks as
non-speech (`-sns`), and collapse runs of 3 or more consecutive cues with
identical text into one (keeping the first cue's start and the last
cue's end). A transcript with no cues left is "no speech found": the job
ends as done with that note, and no caption file is uploaded (spec edge
case).

**Rationale**: Church recordings contain long stretches of worship music,
where large Whisper models are known to produce repeated or invented
lines. These are cheap, testable guards; their thresholds are hypotheses
to check against a real sermon recording during implementation
(quickstart Scenario 3).

## §6 Putting the caption file in Drive

**Decision**: Upload the finished `.srt` with a single small multipart
`files.create` call through the existing Drive client
(`google.golang.org/api/drive/v3`), with:
- `parents` = the video's folder, `mimeType` = `application/x-subrip`.
- `appProperties` = `{ballastCaptionFor: <upload id>}`.
- `name` = the video's base name + `.srt`, or `<base> (2).srt`, `(3)`, …
  when a file of that name already exists in the folder (FR-014). This is
  checked with a name query just before creating.

Before creating, Ballast looks for a non-trashed file in that folder
already tagged `ballastCaptionFor: <upload id>`. If one exists (Ballast
crashed after creating it but before recording it), it is adopted rather
than uploaded again. This is the same "check before re-sending" approach
the duplicate-upload fix (`drive.FindLandedUpload`) introduced for videos.

**Rationale**: Caption files are tiny (a 3-hour sermon's is under 1 MB),
so the resumable chunk engine adds nothing. Tagging and adopting prevents
duplicate caption files, the bug just fixed for videos.

**Not possible (confirmed limitation)**: Drive's API has no call to attach
a caption track to a video; only Drive's web player can. That is why the
caption file sits next to the video (spec Assumptions), and why User
Story 4 exists.

## §7 Model download

**Decision**: Download over HTTPS with a plain `net/http` client (not the
Drive client), to `models/ggml-large-v3-turbo.bin.part` in Ballast's app
data folder. If interrupted, resume from the `.part` file's size with an
HTTP `Range` request. When complete, check the size and SHA-256 against
the compiled-in values, then rename the file to its final name. A model
file is trusted only after passing that check. At startup a final-named
file is re-checked by size only; a full hash check of 1.5 GB is too slow
to run every launch. If the speech engine later fails to load the model,
the file is deleted and downloaded again ("missing or damaged", FR-019).

Before downloading, check free disk space (1.5 GB plus margin). Network
failures are retried with the same capped backoff the upload engine uses
(`drive.NewBackoffPolicy`). A captioning job waiting on the model shows
"Downloading speech model — N%" (FR-019).

**Rationale**: Resume-and-verify is the same discipline the upload engine
already applies; a half-downloaded or tampered model is never run.

## §8 Where the helper binary lives

**Decision**:
- **Packaged app**: `whisper-cli` is copied into `Ballast.app/Contents/MacOS/`
  by a build script (`scripts/build-whisper.sh`). The script builds the
  pinned whisper.cpp tag for arm64 into `build/whisper/`, which is
  gitignored. The app's normal code-signing signs it along with
  everything else.
- **Lookup at runtime**: first the `BALLAST_WHISPER_CLI` environment
  variable (for `wails dev` and tests), then the folder next to Ballast's
  own executable. If neither has it, captions are unavailable with the
  reason "the speech engine is missing from this copy of Ballast".

**Rationale**: `go build` and `wails dev` keep working on any machine
without the binary (it is not `go:embed`-ed, so a missing file never
breaks the build), and the packaged app always carries it.

## §9 Not getting in the upload's way

**Decision**:
- One captioning job runs at a time (spec Assumptions), handled by a
  single background worker separate from the upload goroutines.
- The `afconvert` and `whisper-cli` child processes get a lower CPU
  priority (`setpriority`, nice 10) right after they start.
- Captioning only reads the local video file, and its only Drive call is
  the final small caption upload, so it never competes for the upload's
  Drive session and never touches upload rows or checkpoints (FR-006).
- The upload engine (`internal/drive`) is not changed. `app.go` only
  notifies the captions worker when an upload is created, succeeds, fails,
  or is cancelled.

**Rationale**: Uploading is limited by the network; transcription uses
the GPU and CPU. The spec's ≤5% upload-slowdown target (SC-004) is
verified in quickstart Scenario 4.

## §10 Which files are captioned

**Decision**: Decide by file extension, case-insensitive. `.mp4`, `.mov`,
`.m4v` → captioned. Other common video extensions (`.mkv`, `.avi`, `.webm`,
`.wmv`, `.flv`, `.mpg`, `.mpeg`, `.3gp`) → a job that ends at once with
"Captions aren't supported for .<ext> files yet". Anything else → no job
and no captioning status (FR-010).

**Rationale**: Extension checks are predictable, testable, and match what
`afconvert` can read (§3). A video whose audio cannot be read anyway fails
with a clear reason at extraction.

## §11 Languages offered

**Decision**: The language setting offers exactly two choices: **English**
(default, `-l en`) and **Automatic** (`-l auto`, detected separately for
each 10-minute piece).

**Rationale**: FR-020 requires these two; anything more is extra scope
(Constitution Principle V). Whisper's supported-language list does not
include Twi or Ga, so listing them would promise accuracy the engine
cannot give. FR-015's notice covers this. More languages can be added
later without changing the data model, since the setting stores a
language code.

## §12 Platforms: Apple-silicon Macs only

**Decision**: Captioning is available on macOS **on Apple silicon**
(arm64). Intel Macs, Windows, and Linux show "Captions aren't available on
this system yet" (FR-017) and behave exactly as with captions off.

**Rationale**: Without Apple's GPU, the high-accuracy model runs far
slower than real time, so SC-003 cannot be met on Intel Macs, and shipping
something that slow would mislead users. Saying so plainly satisfies
Constitution Principle VII. The spec's FR-017 and Platforms assumption are
updated alongside this plan to say "macOS on Apple silicon".

## §13 Embedded caption track: investigation method (User Story 4)

**Decision**: An investigation only; nothing ships. Method:
1. Make a captioned copy of a short sample and of a multi-hour recording
   with a developer-only command that copies the streams without
   re-encoding: `ffmpeg -i in.mp4 -i in.srt -map 0 -map 1 -c copy -c:s mov_text out.mp4`.
2. Confirm the video and audio are unchanged by comparing per-stream
   checksums (`ffmpeg -map 0:v -c copy -f md5 -` and the same for audio)
   between the original and the copy.
3. Upload both copies to Drive, open them in Drive's web player, and
   record whether a caption option appears, with screenshots.
4. Record how long step 1 takes on the multi-hour file, and the extra disk
   space it needs (a full copy of the video).

The finding goes in `specs/005-automatic-video-captions/findings-embedded-captions.md`
(SC-008). `ffmpeg` is used only for this investigation, never shipped.
