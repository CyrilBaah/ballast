# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends Feature 005's contract. Existing methods, events, and DTOs keep
their shapes; `UploadListItemDTO` gains one optional field.

## DTOs

```ts
interface SummarySettingsDTO {
  enabled: boolean;                    // FR-012, default true
  available: boolean;                  // false when captions are off/unavailable, or the engine is missing
  unavailableReason?: string;          // e.g. "Summaries need captions turned on"
  modelConsent: "unasked" | "accepted" | "declined";   // FR-013
  modelDownloaded: boolean;
  modelSizeBytes: number;              // shown in the download prompt
}

interface SummaryJobDTO {
  uploadId: number;
  status: "waiting" | "in_progress" | "done" | "failed" | "cancelled";
  phase?: "waiting_for_captions" | "awaiting_consent" | "downloading_model" | "waiting_for_engine"
        | "summarising" | "waiting_for_video" | "uploading_summary";
  progressPercent: number;
  driveFileName?: string;
  driveFileLink?: string;              // FR-018
  localCopyPath?: string;              // FR-007, FR-018
  note?: string;                       // failure/cancel reason (FR-009)
  canRetry: boolean;
}

interface UploadListItemDTO {
  /* …existing fields, plus Feature 005's caption… */
  summary?: SummaryJobDTO;
}
```

## Methods

| Method | Behaviour |
|---|---|
| `SummariesGetSettings() -> SummarySettingsDTO` | Read-only. |
| `SummariesSetEnabled(enabled: boolean) -> SummarySettingsDTO` | Saves the setting. Turning on while consent is `declined` or `unasked` sets it to `accepted` and starts the model download. Errors if `available` is false. |
| `SummariesAnswerModelDownload(accept: boolean) -> SummarySettingsDTO` | The answer to the one-time prompt. Accept: download starts and jobs in `awaiting_consent` continue. Decline: summaries off, and those jobs are cancelled with "Summaries were turned off". The video and captions are never touched. |
| `SummariesGetJob(uploadId: number) -> SummaryJobDTO \| null` | For the transfer details. |
| `SummariesRetry(uploadId: number) -> SummaryJobDTO` | "Try again" on a failed job (FR-009). |
| `SummariesShowLocalCopy(uploadId: number) -> void` | Reveals the `.md` in Finder. |

**Added to existing methods** (signatures unchanged): `UploadStart` and
`UploadRetry` create a SummaryJob alongside the CaptionJob when summaries
are on. `UploadCancel` and upload failure cancel it, keeping any local
`.md`. `UploadDelete` removes the row only. `UploadListRecent` fills
`summary`.

## Events

| Event | Payload | When |
|---|---|---|
| `summaries:consent-needed` | `{ modelSizeBytes: number }` | Once, when the first summary needs the model and consent is `unasked` |
| `summaries:updated` | `SummaryJobDTO` | Every status or phase change; progress throttled to at most once a second |

## UI contract

- **Settings → "Video summaries"**: an on-by-default switch and the
  FR-014 notice: "Summaries are written on this Mac for free. Nothing but
  the finished summary leaves your computer. Summaries of long or unclear
  recordings may be less detailed." When `available` is false, the switch
  is disabled and `unavailableReason` is shown.
- **Download prompt**: shows the model size, with "Download and summarise"
  and "Not now". It doesn't block anything already running.
- **Transfer rows**: a summary line under the caption line, e.g.
  - "Summary: waiting for captions"
  - "Summary: waiting for captions to finish on this Mac" (`waiting_for_engine`)
  - "Summary: writing — 40%"
  - "Summary ready on this Mac — waiting for the video"
  - "Summary ready"
  - "Summary couldn't be made — <reason> [Try again]"
- **Transfer details**: "Open summary in Drive" and "Show summary in
  Finder".
