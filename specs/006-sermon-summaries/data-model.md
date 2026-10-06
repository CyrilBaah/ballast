# Data Model: Automatic Sermon Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

Same SQLite file. One new table, `summary_job`, plus two keys in Feature
005's `setting` table. The `upload` and `caption_job` tables are not
changed. The API key is **not** in the database (research.md §6).

## Entity: SummaryJob (`summary_job` table)

| Column | Type | Rules |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `upload_id` | INTEGER NOT NULL UNIQUE | References `upload.id`; at most one per upload. Deleted with the upload's history row. |
| `status` | TEXT NOT NULL | `waiting`, `in_progress`, `done`, `failed`, or `cancelled` |
| `phase` | TEXT | While not ended: `waiting_for_captions`, `summarising`, `waiting_for_video`, `uploading_summary`. NULL once ended. |
| `provider` | TEXT NOT NULL | `anthropic` (only value in this version, FR-019) |
| `model` | TEXT NOT NULL | e.g. `claude-opus-5-5`, recorded for later cost and quality review |
| `attempts` | INTEGER NOT NULL DEFAULT 0 | Summarise attempts made (research.md §8 caps automatic ones at 3) |
| `next_attempt_at` | DATETIME | When the next automatic retry may run; NULL otherwise |
| `input_tokens` | INTEGER | From the response's `usage`, for cost display and checking §9 |
| `output_tokens` | INTEGER | Same |
| `unverified_quotes` | INTEGER NOT NULL DEFAULT 0 | Quotes dropped by the transcript check (research.md §4) |
| `local_copy_path` | TEXT | The `.md` saved next to the original video (FR-007) |
| `drive_file_id` | TEXT | The Google Doc, set when `done` |
| `drive_file_link` | TEXT | Set together with `drive_file_id` |
| `drive_file_name` | TEXT | e.g. `Sermon — Summary (2)` |
| `note` | TEXT | Failure reason when `failed`; "No transcript to summarise" when cancelled for that reason |
| `created_at`, `updated_at`, `ended_at` | DATETIME | As in `caption_job` |

**Work folder** (not in the DB): `<app data>/summaries/<job id>/` holds
`transcript.txt` (copied from the caption transcript when it is ready, so
the job does not depend on Feature 005's work folder, which is deleted) and
`summary.json` (the validated model output). This lets "Try again" and
restart recovery run without redoing captions (FR-009, FR-017). It is
deleted when the job ends.

### Validation rules

- `drive_file_id` and `drive_file_link` are both set or both NULL, and only
  when `done`.
- `failed` always has a `note`.
- `phase` is NULL exactly when ended.
- `attempts <= 3` for automatic retries. "Try again" resets `attempts` to 0.

### State machine

```text
created (video upload starts, summaries on and set up, captions on and available)
   │
   ▼
waiting / waiting_for_captions ── caption job ends without a transcript ──► cancelled (note: "No transcript to summarise")
   │  transcript ready (copied to work folder)
   ▼
in_progress / summarising ── temporary error ──► (attempts < 3) wait next_attempt_at, retry
   │                     └── 401/402/403/refusal/3rd failure ──► failed (note) ──"Try again"──► in_progress / summarising
   │  summary.json validated, local .md saved
   ▼
in_progress / waiting_for_video ── video upload succeeded ──► uploading_summary ──► done
   │
   └── upload cancelled or failed at any point ──► cancelled (local .md kept if already saved — FR-016)
```

- **App restart**: `summarising` restarts from `transcript.txt`. If
  `summary.json` already exists, it continues at `waiting_for_video`
  (FR-017).
- **Summaries turned off mid-job**: the job finishes, as with captions.

## Settings (`setting` table, from Feature 005)

| Key | Values | Default |
|---|---|---|
| `summaries_enabled` | `true` / `false` | `false` (FR-012) |
| `summary_provider` | `anthropic` | `anthropic` |

**Keychain** (not the DB): service `ballast`, account `anthropic-api-key`
holds the API key. "Set up" means `summaries_enabled = true` **and** a key
exists in the keychain.

## Relationships

```text
upload 1 ── 0..1 caption_job (Feature 005)
upload 1 ── 0..1 summary_job          (summary_job waits on caption_job's transcript)
```
