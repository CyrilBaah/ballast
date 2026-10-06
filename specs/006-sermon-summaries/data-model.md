# Data Model: Automatic Sermon Summaries

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

Same SQLite file. One new table, `summary_job`, plus keys in Feature 005's
`setting` table. The `upload` and `caption_job` tables are not changed.
There are no secrets: the engine is local and needs no key.

## Entity: SummaryJob (`summary_job` table)

| Column | Type | Rules |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `upload_id` | INTEGER NOT NULL UNIQUE | References `upload.id`; at most one per upload. Deleted with the upload's history row. |
| `status` | TEXT NOT NULL | `waiting`, `in_progress`, `done`, `failed`, or `cancelled` |
| `phase` | TEXT | While not ended: `waiting_for_captions`, `awaiting_consent`, `downloading_model`, `waiting_for_engine`, `summarising`, `waiting_for_video`, `uploading_summary`. NULL once ended. |
| `engine` | TEXT NOT NULL | `local` (only value in this version, FR-019) |
| `model` | TEXT NOT NULL | The pinned model file name, for later quality review |
| `part_count` | INTEGER | Number of transcript parts (research.md §4); 1 for a single pass |
| `parts_done` | INTEGER NOT NULL DEFAULT 0 | Checkpoint: part notes saved. A restart resumes here (FR-017). |
| `progress_percent` | INTEGER NOT NULL DEFAULT 0 | Within the current phase |
| `attempts` | INTEGER NOT NULL DEFAULT 0 | Unusable-answer attempts (research.md §8 caps at 3). "Try again" resets it. |
| `unverified_quotes` | INTEGER NOT NULL DEFAULT 0 | Quotes dropped by the transcript check (§5) |
| `local_copy_path` | TEXT | The `.md` next to the original video (FR-007) |
| `drive_file_id` | TEXT | The Google Doc, when `done` |
| `drive_file_link` | TEXT | Set together with `drive_file_id` |
| `drive_file_name` | TEXT | e.g. `Sermon — Summary (2)` |
| `note` | TEXT | Failure reason when `failed`; reason when `cancelled` |
| `created_at`, `updated_at`, `ended_at` | DATETIME | As in `caption_job` |

**Work folder** (not in the DB): `<app data>/summaries/<job id>/` holds
`transcript.txt` (copied from the caption transcript when it is ready),
`part-N.json` (notes per part), and `summary.json` (the validated final
output). This lets "Try again" and restart recovery skip work already
done. It is deleted when the job ends.

### Validation rules

- `drive_file_id` and `drive_file_link` are both set or both NULL, and only
  when `done`.
- `failed` always has a `note`.
- `phase` is NULL exactly when ended.
- `parts_done <= part_count`; `attempts <= 3`.

### State machine

```text
created (video upload starts, summaries on, captions on and available)
  │
  ▼
waiting / waiting_for_captions ── caption job ends without transcript ──► cancelled ("No transcript to summarise")
  │ transcript ready (copied to work folder)
  ├─ consent unasked ──► waiting / awaiting_consent ── declined ──► cancelled ("Summaries were turned off")
  ├─ model missing   ──► in_progress / downloading_model
  ▼
waiting / waiting_for_engine   (holds until the heavy-work lock is free — research.md §7)
  ▼
in_progress / summarising      (parts → combine; unusable answer → retry ≤3 → failed)
  │ summary.json validated, local .md saved
  ▼
in_progress / waiting_for_video ── video succeeded ──► uploading_summary ──► done
  │
  └── upload cancelled or failed at any point ──► cancelled (local .md kept if already saved — FR-016)
failed ──"Try again"──► waiting_for_engine (attempts reset, transcript and saved parts reused)
```

- **App restart**: a job in `summarising` resumes at `parts_done`. If
  `summary.json` exists, it goes to `waiting_for_video` (FR-017).
- **Summaries turned off mid-job**: the job finishes.

## Settings (`setting` table, from Feature 005)

| Key | Values | Default |
|---|---|---|
| `summaries_enabled` | `true` / `false` | `true` (FR-012) |
| `summary_model_consent` | `unasked` / `accepted` / `declined` | `unasked` (FR-013) |

Declining sets consent to `declined` **and** `summaries_enabled = false`.
Turning summaries back on sets consent to `accepted` and starts the
download.

## Summary model (file, not a table)

`<app data>/models/<pinned name>.gguf`, about 2–5 GB, `.part` while
downloading. It counts as present only once verified (research.md §14).
It is shared by all jobs and never deleted by job cleanup.

## Relationships

```text
upload 1 ── 0..1 caption_job (Feature 005)
upload 1 ── 0..1 summary_job          (waits on caption_job's transcript)
heavy-work lock: shared by the caption and summary workers (one model at a time)
```
