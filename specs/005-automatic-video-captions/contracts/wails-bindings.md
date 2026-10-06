# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends the bindings recorded by Features 001–003. Every existing method,
event, and DTO keeps its shape, except that `UploadListItemDTO` gains one
optional field. Go method names follow the existing `Namespace` + `Verb`
convention (`UploadStart`, `DriveListFolders`, …). TypeScript names are
the generated ones in `frontend/wailsjs/go/main/App.d.ts`.

## DTOs

```ts
// Caption settings and whether captions can run on this machine.
interface CaptionSettingsDTO {
  enabled: boolean;                 // FR-009, default true
  language: "en" | "auto";          // FR-020, default "en"
  available: boolean;               // false on Intel Macs, Windows, Linux, or when the
                                    // speech engine is missing (research.md §8, §12)
  unavailableReason?: string;       // e.g. "Captions aren't available on this system yet" (FR-017)
  modelConsent: "unasked" | "accepted" | "declined";   // FR-019
  modelDownloaded: boolean;
  modelSizeBytes: number;           // shown in the consent prompt (FR-019)
}

// One video's captioning state (data-model.md: CaptionJob).
interface CaptionJobDTO {
  uploadId: number;
  status: "waiting" | "in_progress" | "done" | "failed" | "cancelled";
  phase?: "awaiting_consent" | "downloading_model" | "extracting_audio"
        | "transcribing" | "waiting_for_video" | "uploading_captions";
  progressPercent: number;          // 0–100 within the current phase (FR-008)
  language: "en" | "auto";
  driveFileName?: string;           // e.g. "Sermon (2).srt" (FR-014)
  driveFileLink?: string;           // opened from the transfer's details (FR-016)
  localCopyPath?: string;           // the caption file saved on this Mac (FR-022)
  note?: string;                    // failure reason, or "No speech found in this video" (FR-007)
}

// Existing DTO, one new optional field.
interface UploadListItemDTO {
  /* …all existing fields unchanged… */
  caption?: CaptionJobDTO;          // absent when the upload has no caption job (FR-010)
}
```

## Methods

### `CaptionsGetSettings() -> CaptionSettingsDTO`

Read-only. Works signed out. Used by the Settings view and to decide
whether to show caption status at all.

### `CaptionsSetEnabled(enabled: boolean) -> CaptionSettingsDTO`

Saves `captions_enabled`. Applies to uploads started afterwards. A job
already running is left to finish (spec Assumptions).
- Turning on while `modelConsent` is `declined` or `unasked` sets it to
  `accepted` and starts the one-time model download in the background
  (FR-019). Switching the setting on counts as consent.
- Rejected with an error when `available` is false.

### `CaptionsSetLanguage(language: "en" | "auto") -> CaptionSettingsDTO`

Saves `caption_language`. Applies to jobs created afterwards (FR-020).
Any other value is rejected.

### `CaptionsAnswerModelDownload(accept: boolean) -> CaptionSettingsDTO`

The user's answer to the one-time consent prompt (FR-019).
- `true`: consent becomes `accepted`, the download starts, and every job
  in `waiting/awaiting_consent` moves to `in_progress/downloading_model`.
- `false`: consent becomes `declined`, `captions_enabled` becomes `false`,
  and every job in `waiting/awaiting_consent` becomes `cancelled` with the
  note "Captions were turned off".

The video uploads are never touched (FR-006).

### `CaptionsGetJob(uploadId: number) -> CaptionJobDTO | null`

Current state of one video's captioning, or `null` if it has none. Used
when the transfer details open, to catch up before events arrive.

### `CaptionsShowLocalCopy(uploadId: number) -> void`

Reveals the job's local caption file in Finder (FR-022). Returns an error
if the job has no `localCopyPath` or the file has since been moved or
deleted.

### Existing methods: behaviour added, signatures unchanged

| Method | Added behaviour |
|---|---|
| `UploadStart`, `UploadRetry` | When the file is a captionable video, captions are enabled, and they are available, creates its CaptionJob (data-model.md). The upload itself starts exactly as before, never waiting on captions (FR-006). |
| `UploadCancel` | Also cancels the upload's caption job: stops any running helper process and deletes the work folder (FR-011). |
| `UploadDelete` | Also deletes the upload's caption job row. Does **not** delete a caption file already in Drive. |
| `UploadListRecent` | Fills `caption` for uploads that have a job. |

## Events (Go → frontend)

| Event | Payload | When |
|---|---|---|
| `captions:consent-needed` | `{ modelSizeBytes: number }` | Once, when the first video job is created while consent is `unasked`. The frontend shows the consent prompt (spec Story 1, scenario 1). |
| `captions:updated` | `CaptionJobDTO` | Whenever a job's status, phase, or progress changes. Progress updates are throttled to at most one per second per job. |

No other event is added. `captions:updated` carries a full `CaptionJobDTO`,
so the frontend never has to combine partial updates, and done or failed
states arrive the same way.

## Frontend surfaces (UI contract)

- **Settings view**: a "Automatic captions" switch and a "Caption
  language" choice (English / Automatic). Next to them, the FR-015 notice:
  captions are generated on this computer; accuracy may be lower for
  languages other than English, including Twi and Ga; captions are saved
  as a separate `.srt` file next to the video, which can be attached in
  Drive's player. When `available` is false, both controls are disabled
  and `unavailableReason` is shown (FR-017).
- **Consent prompt**: shown on `captions:consent-needed`, stating the
  download size, with "Download and caption" and "Not now". It does not
  block the upload, which is already running (FR-019).
- **Transfers / Home rows**: for a row whose `caption` is present, a
  secondary line under the upload's own status, e.g.
  "Captions: transcribing — 42%", "Captions: waiting for the video to
  finish", "Captions ready", or "Captions couldn't be made — This video
  has no audio track". It is always visually separate from the upload's
  status (FR-008).
- **Transfer details**: when `driveFileLink` is set, an "Open captions in
  Drive" action (FR-016); when `localCopyPath` is set, a "Show in Finder"
  action (FR-022). The caption line reads "Captions ready on this Mac —
  waiting for the video to finish" while in `waiting_for_video`.
