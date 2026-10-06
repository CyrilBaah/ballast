# Research: Automatic Sermon Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-06

Model names, prices, and API behaviour were checked against the bundled
Claude API reference (cached 2026-09-25), not recalled. Items marked
**measured** come from running the real 43-minute sermon
(`Paster Joseph Ayertey.mp4`) through the Feature 005 pipeline and
summarising it by hand on 2026-10-06.

## §1 Talking to Claude: the official Go SDK

**Decision**: Use the official Anthropic Go SDK,
`github.com/anthropics/anthropic-sdk-go`, with the API key passed
explicitly through `option.WithAPIKey(...)` from the keychain (§6). Never
read it from the environment.

**Rationale**: An official SDK exists for Go. It gives typed errors
(`*anthropic.Error` with `StatusCode` and `Type()`), built-in retries for
429/5xx, streaming, and request types that keep up with API changes.
Hand-written HTTP would duplicate all of that.

**Cost to the project**: one new Go module dependency, justified in
plan.md Complexity Tracking.

## §2 Model and settings

**Decision**:
- Model `claude-opus-5-5` (the current default Opus): 1M-token context,
  $4 / $20 per million input / output tokens.
- Effort is set **explicitly** to `medium`. This model defaults to `medium`,
  but the code states it so changes are deliberate. It is raised only if
  the quickstart review (Scenario 1) finds summaries missing content.
- Thinking is left at the model's default (adaptive). It can't be disabled
  on this model.
- The request is streamed and the final message collected, as recommended
  for long inputs. `max_tokens` is 16,000.
- **Server-side refusal fallback** is on (`fallbacks: "default"` with beta
  `server-side-fallback-2026-07-01`): if a safety classifier declines a
  sermon, another model serves it in the same call. Sermon text is unlikely
  to trigger this, but a refusal must never silently produce no summary.
  Any `refusal` that still comes back is reported as "The AI service
  declined to summarise this sermon".
- No prompt caching: each sermon is a single one-off request, so there is
  no repeated prefix worth caching.

The exact Go field names for `OutputConfig.Effort`, `fallbacks: "default"`,
and the output format are confirmed against the installed SDK at
implementation time (compile and fix), not guessed here.

## §3 Getting structured output back

**Decision**: Request structured outputs (`output_config.format` with a
JSON schema) so the response is always valid JSON matching one schema:

```text
overview: string
main_points: [{ title: string, detail: string }]
actions: [string]                         // "What we must do"
bible_references: [{ reference: string, heard_as: string, certain: boolean }]
quotes: [{ text: string, start_seconds: number, source_text: string }]
title: string
description: string
share_message: string                     // the ready-to-share WhatsApp text (FR-002a)
```

Go then builds every output from this JSON: the Google Doc, the local
Markdown copy, and the "Check before sharing" note.

**Rationale**: The model never has to format three documents correctly at
once, and Ballast can check the result (§4, §5) before anything is saved.

## §4 Keeping quotes honest (FR-003, SC-002)

**Decision**: Each quote comes with `source_text` (the exact transcript
words it came from) and `start_seconds`. Before saving, Go checks that
`source_text` matches the transcript near `start_seconds`:
- it normalises both (lower-case, punctuation removed);
- it requires at least 80% of `source_text`'s words to appear, in order,
  within transcript cues from 30 s before to 30 s after.

A quote that fails is **dropped**, not "fixed". If every quote fails, the
summary still saves with "No quotes could be verified".

**Rationale**: The clarification allows tidied quotes, so `text` can't be
checked word for word. `source_text` can be, and that is what makes "each
quote traces back to the transcript" testable by code, not only by a
reviewer.

**Measured**: the hand summary's four tidied quotes each map to a clear
source passage (e.g. "Even in prison, the Lord was with him." ← `[33:xx]
"Even in prison. The Lord was with him."`), so the rule is achievable on
real sermons.

## §5 Bible references and the "Check before sharing" note (FR-003a)

**Decision**: The model returns each reference with `heard_as` (what the
transcript actually says) and `certain`. References with `certain: false`
(e.g. heard "Hebrews chapter 24, verse 26", likely Hebrews 11:24–26) appear
in the summary under their likely reference **and** in a separate "Check
before sharing" list showing `heard_as`. That list goes in the Google Doc
and the local copy, never inside `share_message`.

**Measured**: the real sermon had exactly this case. "Matthew 28, verse 30"
and "Genesis… magic chapter 28" both meant Matthew 28:18–20, and the
Hebrews reference above was garbled. Without the flag, the hand summary
would have shared wrong references.

## §6 Where the API key lives

**Decision**: Store the key in the OS keychain, through the same
`go-keyring` library `internal/keychain` already uses for the database
encryption key, under service `ballast` and account `anthropic-api-key`.
Ballast's database stores only whether summaries are on and which
provider is used, never the key. The UI receives only a masked form
(`sk-ant-…a1b2`).

**Rationale**: This is the same protection as the Google sign-in
(Constitution IV: secrets in the OS keychain, not next to the database).

## §7 Checking a key (FR-014)

**Decision**: Validate a newly entered key by listing models
(`GET /v1/models` via the SDK's `Models.List`). It is free (no tokens), and
a 401 means the key is rejected. A successful listing does not prove the
account has credit, so the user is told "Key works" and a later 402 is
reported per §8.

## §8 Errors and retries

| Response | Meaning | Ballast does |
|---|---|---|
| 401 `authentication_error` | key rejected or revoked | fail: "Your AI key was rejected or removed", and flag Settings |
| 402 `billing_error` | out of credit or payment problem | fail: "Your AI account is out of credit" |
| 403 `permission_error` | key not allowed to use this | fail: "Your AI key isn't allowed to do this" |
| 429, 5xx, 529 `overloaded_error`, network errors | temporary | the SDK's own retries (2), then Ballast retries the whole request up to 3 times with 1, 5, then 15 minutes between, then fails "The AI service is unavailable; try again later" (FR-010) |
| `stop_reason: refusal` after fallback | declined | fail with the decline reason (§2) |
| `stop_reason: max_tokens` | output cut off | one retry with `max_tokens` 32,000, then fail |

Errors are classified with `errors.As` into `*anthropic.Error` and switched
on `StatusCode`, per the SDK's documented Go pattern. The retry cap keeps
a failing service from running up repeated charges (spec Assumptions).

## §9 Cost per sermon (FR-015)

**Measured transcript size**: the 43-minute sermon's transcript is about
5,400 words, roughly 7,000 tokens. A 3-hour sermon is about 30,000 words,
roughly 40,000 tokens.

**Estimate** at $4 / $20 per million tokens, assuming 3,000–8,000 output
tokens including thinking:

| Sermon | Input | Output | Total |
|---|---|---|---|
| 45 minutes | ~$0.03 | $0.06–0.16 | **~$0.10–0.20** |
| 3 hours | ~$0.16 | $0.06–0.16 | **~$0.20–0.35** |

Settings shows "about $0.10–$0.40 per sermon". The real `usage` figures
from quickstart Scenario 1 replace the estimate before release.

## §10 Transcript input format

**Decision**: Send the transcript as plain text with one line per
caption cue, prefixed `[mm:ss]` or `[h:mm:ss]`, built from Feature 005's
`.srt`. The system prompt tells the model that the transcript is data to
summarise, never instructions to follow. Cues that Feature 005 collapsed
as repeats appear once.

**Rationale**: Timestamps are what make quotes checkable (§4). Dropping
SRT numbering and arrows saves about 30% of tokens.

## §11 The summary in Drive: a Google Doc

**Decision**: Build simple HTML (headings, paragraphs, lists, a bold
"Check before sharing" box) from the JSON and upload it with
`files.create`, `mimeType: application/vnd.google-apps.document`, and HTML
as the media, so Drive converts it to a Google Doc. It is named
"<video base> — Summary", with `(2)` and so on if taken (same rule as
Feature 005), and tagged `appProperties.ballastSummaryFor=<upload id>` so a
crash never creates a duplicate (adopt before create, as for caption
files). The local copy is the same content as Markdown,
`"<video base> — Summary.md"`, saved next to the original video.

**Rationale**: Drive reliably converts HTML into a Google Doc, keeping
headings and lists. Markdown import is less predictable.

## §12 Prompt content (the summary's shape)

The system prompt fixes what the hand-made summary showed works for this
church:
- English only, sermon only, no announcements;
- an overview of 3–5 sentences;
- 4–7 main points;
- the "What we must do" actions the preacher gave;
- 3–6 quotes;
- a title under 70 characters;
- a description under 600 characters;
- a share message in WhatsApp format: `*bold*` headings, numbered points,
  dash lists, **no emojis**, no timestamps, no notes about the transcript,
  closing with a one-line blessing.

The hand-made version (shared to the church group on 2026-10-06) is kept as
the reference example in `internal/summaries/testdata/` for the prompt's
example and for review in quickstart Scenario 1.

## §13 Provider can change later (FR-019)

**Decision**: A small Go interface
`Summarizer { Validate(ctx) error; Summarize(ctx, transcript) (Summary, error) }`
with one implementation, `anthropicSummarizer`. The `Summary` struct is
the JSON schema in §3, so a later Gemini implementation only has to return
the same struct. No second provider is built now (Constitution V).
