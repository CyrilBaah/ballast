# Data Model: Automatic Video Captions

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

Everything lives in Ballast's existing SQLite file. Two new tables are
added with `CREATE TABLE IF NOT EXISTS`, following the same additive
pattern earlier features used. The `upload` table is **not** changed.

## Entity: CaptionJob (`caption_job` table)

The work of producing and delivering captions for one uploaded video
(spec Key Entities). Created when a video upload is started while captions
are on and available on this machine.

| Column | Type | Rules |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | |
| `upload_id` | INTEGER NOT NULL UNIQUE | References `upload.id`. One job per upload, at most. Deleting an upload row from history deletes its job. |
| `status` | TEXT NOT NULL | One of `waiting`, `in_progress`, `done`, `failed`, `cancelled` (see state machine). |
| `phase` | TEXT | Only while `waiting` / `in_progress`: `awaiting_consent`, `downloading_model`, `extracting_audio`, `transcribing`, `waiting_for_video`, `uploading_captions`. NULL otherwise. |
| `language` | TEXT NOT NULL | `en` or `auto`, copied from the language setting when the job is created, so a later setting change doesn't alter a job in flight (FR-020). |
| `progress_percent` | INTEGER NOT NULL DEFAULT 0 | 0–100 within the current phase. Shown to the user (FR-008). |
| `audio_duration_ms` | INTEGER | Set after extraction. |
| `piece_count` | INTEGER | Number of ~10-minute pieces (research.md §4). Set after extraction. |
| `pieces_done` | INTEGER NOT NULL DEFAULT 0 | Checkpoint: pieces whose transcript has been appended to the work folder. A resumed job continues from here (FR-012). |
| `drive_file_id` | TEXT | Set when `done` with a caption file. |
| `drive_file_link` | TEXT | Set with `drive_file_id`; opened from the transfer's details (FR-016). |
| `drive_file_name` | TEXT | The name actually used, e.g. `Sermon (2).srt` (FR-014). |
| `note` | TEXT | Plain-language outcome shown to the user: the failure reason when `failed`, or "No speech found in this video" when `done` without a file. |
| `created_at` | DATETIME NOT NULL | |
| `updated_at` | DATETIME NOT NULL | |
| `ended_at` | DATETIME | Set on entering `done`, `failed`, or `cancelled`. |

**Work folder** (not in the database): `<app data>/captions/<job id>/`
holds `audio.wav`, the current piece's WAV, and `transcript.srt`, the
merged transcript so far. Its path is derived from the job ID, not
stored. It is deleted when the job reaches `done`, `failed`, or
`cancelled` (FR-013), and any leftover folder whose job has ended is
removed at startup.

### Validation rules

- `drive_file_id` and `drive_file_link` are both set or both NULL. They
  may only be set when `status = done`.
- `pieces_done <= piece_count` once `piece_count` is set.
- `phase` is NULL exactly when `status` is `done`, `failed`, or `cancelled`.
- A `failed` job always has a non-empty `note` (FR-007).

### State machine

```text
                       ┌───────────────── upload cancelled / upload failed ─────────────────┐
                       │                  (any non-ended state → cancelled, FR-011)          ▼
created ──► waiting ──► in_progress ──────────────────────────────────────────────► done
            │  phase:     phase: downloading_model → extracting_audio →              (file uploaded,
            │  awaiting_  transcribing → waiting_for_video → uploading_captions       or "no speech")
            │  consent           │
            │                    └──── unrecoverable error ───────────────────────► failed (note = reason)
            └── user declines the model download ─────────────────────────────────► cancelled
                                                                                      (note = "Captions
                                                                                      were turned off")
```

- **created → waiting / awaiting_consent**: the model is not downloaded
  and download consent has never been given (FR-019).
- **created → in_progress**: otherwise, starting with
  `downloading_model` if the model is missing, else `extracting_audio`.
- **Unsupported format** (research.md §10): created straight into
  `failed` with "Captions aren't supported for .mkv files yet".
- **transcribing → waiting_for_video**: the transcript is complete but the
  video upload hasn't succeeded yet. Captions are uploaded only after the
  video is confirmed in Drive (spec Assumptions, FR-011).
- **waiting_for_video → uploading_captions**: when the upload reaches
  `succeeded`.
- **Network errors while downloading the model or uploading captions**:
  retried with backoff, staying in the same phase. Being signed out or a
  full Drive keeps the job in `uploading_captions` and it retries later;
  it never touches the video upload's status (spec edge case).
- **App restart**: a job found in `in_progress` resumes its phase. For
  `transcribing` it continues at `pieces_done`. If `audio.wav` is missing
  it goes back to `extracting_audio` (FR-012).
- **Captions turned off while a job runs**: no transition; the job
  finishes (spec Assumptions).

## Entity: Caption settings (`setting` table)

A small key/value table for app-wide preferences, all of which survive
restarts (FR-009, FR-020).

| Column | Type | Rules |
|---|---|---|
| `key` | TEXT PK | |
| `value` | TEXT NOT NULL | |

Keys used by this feature (a missing key means the default):

| Key | Values | Default |
|---|---|---|
| `captions_enabled` | `true` / `false` | `true` (FR-009) |
| `caption_language` | `en` / `auto` | `en` (FR-020) |
| `caption_model_consent` | `unasked` / `accepted` / `declined` | `unasked` (FR-019) |

Declining sets `caption_model_consent = declined` **and**
`captions_enabled = false`. Turning captions back on in Settings sets
consent to `accepted`, which starts the download (FR-019).

## Entity: Speech model (file, not a table)

`<app data>/models/ggml-large-v3-turbo.bin`, about 1.5 GB. While
downloading it is named `….bin.part`. It counts as present only once it
has passed the size and SHA-256 check and been renamed (research.md §7).
It is shared by all jobs and never deleted by job cleanup.

## Relationships

```text
upload 1 ──── 0..1 caption_job        (caption_job.upload_id UNIQUE)
setting  (app-wide, no relationships)
speech model file (app-wide, shared by all caption jobs)
```
