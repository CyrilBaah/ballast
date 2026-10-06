# Research: Automatic Video Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-06 (revised for the free, on-Mac design)

Items marked **measured** come from running the real 43-minute sermon
(`Paster Joseph Ayertey.mp4`) through Feature 005's pipeline and
summarising it by hand on 2026-10-06. The hand-made summary shared to the
church group that day is the quality reference for sermons. Summaries are
for **any** video with speech (spec Clarifications), so a non-religious
reference is added too: the 13-minute `Aksum.mp4` in the maintainer's
Downloads folder, summarised by hand during Scenario 0.

## §1 Engine: llama.cpp's `llama-server`, run as a helper program

**Decision**: Summarise with [llama.cpp](https://github.com/ggml-org/llama.cpp)'s
`llama-server` (MIT), run as a child process exactly like Feature 005 runs
`whisper-cli`:
- Ballast starts it for a summary job, bound to `127.0.0.1` on a free
  port, with the model file and a context size.
- It sends one or more requests to the server's OpenAI-compatible
  `/v1/chat/completions` endpoint.
- It stops the process when the job ends.

Nothing listens beyond the Mac itself, and nothing leaves it (FR-005).

**Confirmed locally**: Homebrew's llama.cpp 0.6.0 (build 11429) is already
installed on the maintainer's Mac and provides `llama-server`. Its
documentation shows the chat endpoint accepts `response_format` of type
`json_schema` and enforces the schema with a grammar.

**Rationale**: It is free with no account or limit, runs on Apple's GPU,
and the output can be forced into the exact JSON shape needed (§3). It
follows the same child-process pattern as captions: no cgo, a crash can't
take down Ballast or an upload, a clean kill on cancel, and lower CPU
priority.

**Alternatives considered**:
- *Ollama*: the same engine underneath, but installed and run as a
  separate background service the user has to manage. That is more moving
  parts for no gain.
- *Apple's on-device Foundation Models framework*: free and built in, but
  it needs Swift bridging, requires Apple Intelligence to be enabled, and
  has a context window of only a few thousand tokens, too small for a
  long video transcript.
- *A paid cloud model*: rejected by the user ("cost 0"). The engine
  interface (§13) leaves room to add one later as an option.

## §2 Choosing the model: tested, not guessed

**Decision**: Pick the model by testing it during implementation (task
list), not now. Candidates are open, instruction-tuned models of about
3–8 billion parameters, 4-bit quantised GGUF files of at most about 5 GB,
whose licence allows free use. Each candidate summarises both reference
videos' transcripts (the sermon and `Aksum.mp4`) and is scored on:

1. **Fits**: peak memory of `llama-server` stays under about 5.5 GB on the
   8 GB M2, run after the speech model has exited (§7).
2. **Follows the shape**: schema-valid JSON every time (§3).
3. **Honest**: at least 3 quotes pass the transcript check (§5); no
   invented references, and no Bible section at all for `Aksum.mp4`
   (SC-010).
4. **Useful**: a reviewer judges the main points and share message close
   to the reference summary (SC-009, SC-002).
5. **Fast enough**: meets SC-004 (≤10 min for a 1-hour video).

The winner's download URL is pinned to a specific repository revision,
with its size and SHA-256 compiled in, the same discipline as the speech
model (Feature 005, research.md §7). The maintainer approves the choice
before it is pinned.

## §3 Getting structured output back

**Decision**: Every request uses `response_format: {type: "json_schema", json_schema: {schema: …}}`,
so the server can only produce JSON in this shape:

```text
overview: string
main_points: [{ title: string, detail: string }]
actions: [string]                         // "Key takeaways / actions" — empty → section omitted (FR-002b)
references: [{ kind: "bible" | "book" | "source" | "other",
               reference: string, heard_as: string, certain: boolean }]   // empty → section omitted
quotes: [{ text: string, start_seconds: number, source_text: string }]
title: string
description: string
share_message: string                     // the WhatsApp-ready text (FR-002a)
```

Go builds everything from this JSON: the Google Doc, the local Markdown,
and the "Check before sharing" note.

**Rationale**: With small models, free-form formatting is where quality
breaks down first. Enforcing the shape with a grammar removes that failure
mode entirely, and Go then applies the share-message rules itself (§12).

## §4 Long transcripts: summarise in parts, then combine

**Decision**: Run the server with a 16,384-token context.
- Transcripts of up to about 9,000 tokens (roughly 60 minutes) are
  summarised in **one pass**.
- Longer ones are split on cue boundaries into parts of about 20 minutes.
  Each part produces **notes** in a smaller schema: points, references,
  and candidate quotes with `source_text` and `start_seconds`. A final
  **combine** pass turns all the notes into the full schema.

**Measured**: the 43-minute sermon transcript is about 5,400 words,
roughly 7,000 tokens, so it fits one pass. A 3-hour video (about 40,000 tokens)
becomes about 9 parts and one combine pass.

**Rationale**: Small models lose track of very long inputs, and a large
context costs memory on an 8 GB Mac. Parts plus a combine pass cover the
whole video (FR-004, SC-003) within fixed memory, and each finished part
is saved, so a restart resumes from the next part (FR-017).

## §5 Keeping quotes honest (FR-003, SC-002)

**Decision**: Each quote carries `source_text` (the exact transcript words)
and `start_seconds`. Go checks that `source_text` matches the transcript
near `start_seconds`:
- it normalises both (lower-case, no punctuation);
- it requires at least 80% of `source_text`'s words to appear, in order,
  within cues from 30 s before to 30 s after.

Quotes that fail are **dropped**. If none survive, the summary says "No
quotes could be verified".

**Rationale**: Small models paraphrase and occasionally invent. This
check is what makes "nothing is invented" hold regardless of model
quality.

## §6 References and "Check before sharing" (FR-002b, FR-003a)

**Decision**: The model returns each reference with a `kind` (`bible`,
`book`, `source`, `other`), `heard_as`, and `certain`.
- Uncertain ones appear under their likely reference, and in a separate
  "Check before sharing" list with what was heard. That list is never
  inside `share_message`.
- For `kind: bible`, Go checks that the book name is a real Bible book (a
  fixed list of 66). A reference that fails is moved to "Check before
  sharing" with `certain: false`.
- Go also drops any reference whose `heard_as` doesn't appear in the
  transcript at all. This catches invented references on any kind of
  video, for example a model adding scripture to a documentary (SC-010).
- An empty references list, or an empty actions list, means that section
  is left out of every output (FR-002b).

**Measured**: the real sermon had "Matthew 28, verse 30" (for Matthew
28:20) and "Hebrews chapter 24, verse 26" (for Hebrews 11:24–26). These
are exactly the cases this note exists for.

## §7 One model at a time

**Decision**: Feature 005's captions worker and this feature's summaries
worker share one **heavy-work lock**, a small `internal/heavywork` package
holding a single mutex. Whichever model is running holds it, so the
speech model and the summary model never run together (FR-008, SC-007).
Summary jobs wait at the `waiting_for_engine` phase while captions run.
Captions of the next video always come first, because a summary waiting a
little is harmless.

**Rationale**: On 8 GB, the speech model (measured peak 2.3 GB) plus a
4–5 GB summary model plus Ballast and the OS would push the Mac into heavy
swapping and slow everything, including the upload. Running them in turn
is simple and safe.

## §8 Failures and retries

| Failure | Ballast does |
|---|---|
| Server won't start, or fails to load the model | delete the model and re-download once (it's "damaged", FR-013); if it fails again, fail: "The summary model couldn't be loaded" |
| Not enough memory (process killed, or load error naming memory) | fail: "Not enough free memory to write the summary — close other apps and try again" |
| Response not schema-valid, or empty | retry up to 3 attempts in total, then fail: "The summary model couldn't produce a usable summary" |
| A request takes more than 20 minutes | kill, count as an attempt |
| Model download errors | the same resume-and-retry as Feature 005's model download |

Every failure leaves the upload and captions untouched and offers "Try
again" (FR-009).

## §9 Cost

**Zero.** No account, key, or usage charge. The only costs are a
one-time download (the chosen model, about 2–5 GB, pinned in §2) and the
Mac's own time and power. Time per video is measured in quickstart
Scenario 1 against SC-004.

## §10 Transcript input format

Plain text, one line per caption cue, prefixed `[mm:ss]` or `[h:mm:ss]`,
built from Feature 005's `.srt`, with repeats Feature 005 collapsed shown
once. The system prompt says the transcript is content to summarise,
never instructions. This also keeps the input small, saving about 30%
against raw SRT.

## §11 The summary in Drive: a Google Doc

Build simple HTML from the JSON and create it with `files.create`
(`mimeType: application/vnd.google-apps.document`, HTML media) so Drive
converts it to a Google Doc named "<video base> — Summary" (or `(2)` and
so on), tagged `appProperties.ballastSummaryFor=<upload id>` and adopted
instead of duplicated after a crash. The local copy is the same content
as Markdown, `"<video base> — Summary.md"`, next to the original video.
This uses the existing Drive client; no new dependency.

## §12 Prompt and post-processing

The system prompt is **general-purpose** (any video with speech) and
fixes the shape that worked in the hand-made summary:
- English only, main content only, no housekeeping;
- include references and actions **only** if the speaker actually gave
  them; never add Bible verses or sources that weren't mentioned;
- keep summaries of short or sparse videos in proportion;
- an overview of 3–5 sentences;
- 4–7 main points;
- the speaker's calls to action, if any;
- 3–6 quotes;
- a title under 70 characters;
- a description under 600 characters;
- a share message: `*bold*` headings, numbered points, dash lists, no
  emojis, no timestamps, no notes about the transcript, closing with a
  one-line blessing.

The prompt carries **no** worked example, so the model doesn't copy
sermon phrasing into unrelated videos. The two reference summaries
(`internal/summaries/testdata/reference-sermon-2026-10-04.md` and
`reference-aksum.md`) are used only in tests and Scenario 0 reviews.

Because small models drift, Go **enforces** the share-message rules after
generation rather than trusting the prompt: it strips emoji characters,
strips `[mm:ss]`-style timestamps, and rejects a share message mentioning
"transcript". A rejected message counts as an unusable answer (§8).

## §13 Engine interface (FR-019)

`Summarizer { Ready() error; Summarize(ctx, transcript) (Summary, error) }`
with one implementation, `localSummarizer` (llama-server). The `Summary`
struct is the §3 schema, so an optional paid cloud engine can be added
later without changing the output, the job flow, or the UI. None is built
now.

## §14 Shared model download

Feature 005's model downloader (resume via `Range`, size and SHA-256
check, rename, disk-space precheck, progress) moves into a shared
`internal/modelfetch` package used by both features, rather than being
copied. The summary model is stored at
`<app data>/models/<pinned file name>.gguf`.

## §15 Shipping the engine

`llama-server` is built from a pinned llama.cpp release as a static arm64
binary with Metal, by the same build script as `whisper-cli`
(`scripts/build-whisper.sh` becomes `scripts/build-engines.sh`), and
copied into `Ballast.app/Contents/MacOS/`. During development,
`BALLAST_LLAMA_SERVER` points at Homebrew's
`/opt/homebrew/bin/llama-server`.
