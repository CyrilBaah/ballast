# Contract: Frontend ↔ Backend Interface (Wails Bindings)

Extends Feature 005's contract. Existing methods, events, and DTOs keep
their shapes; `UploadListItemDTO` gains one optional field.

## DTOs

```ts
interface SummarySettingsDTO {
  enabled: boolean;                    // FR-012, default false
  provider: "anthropic";               // FR-019 — one option in this version
  providerName: string;                // "Claude (Anthropic)"
  keySet: boolean;
  keyMasked?: string;                  // e.g. "sk-ant-…a1b2" — the full key is never sent to the UI (FR-013)
  keyStatus: "unknown" | "ok" | "rejected";
  available: boolean;                  // false when captions are off or unavailable on this machine
  unavailableReason?: string;          // e.g. "Summaries need captions turned on"
  costEstimate: string;                // "about $0.10–$0.40 per sermon" (FR-015, research.md §9)
}

interface SummaryJobDTO {
  uploadId: number;
  status: "waiting" | "in_progress" | "done" | "failed" | "cancelled";
  phase?: "waiting_for_captions" | "summarising" | "waiting_for_video" | "uploading_summary";
  attempts: number;
  driveFileName?: string;
  driveFileLink?: string;              // FR-018
  localCopyPath?: string;              // FR-007, FR-018
  note?: string;                       // failure reason (FR-009)
  canRetry: boolean;                   // true when failed and the transcript is still available
}

interface UploadListItemDTO {
  /* …existing fields, plus Feature 005's caption… */
  summary?: SummaryJobDTO;             // absent when the upload has no summary job
}
```

## Methods

| Method | Behaviour |
|---|---|
| `SummariesGetSettings() -> SummarySettingsDTO` | Read-only. |
| `SummariesSetKey(key: string) -> SummarySettingsDTO` | Validates the key with a free model listing (research.md §7). If valid, stores it in the keychain and returns `keyStatus: "ok"`. If rejected, nothing is stored and an error says why. The key is never logged or echoed back (FR-013, FR-014). |
| `SummariesRemoveKey() -> SummarySettingsDTO` | Deletes the key from the keychain and turns summaries off. |
| `SummariesSetEnabled(enabled: boolean) -> SummarySettingsDTO` | Turning on requires a stored key that works, and `available`; otherwise it errors. Applies to videos whose captions finish afterwards (FR-012). |
| `SummariesGetJob(uploadId: number) -> SummaryJobDTO \| null` | Current state for the transfer details. |
| `SummariesRetry(uploadId: number) -> SummaryJobDTO` | "Try again" on a failed job: resets `attempts`, goes back to `summarising`, reuses the saved transcript (FR-009). |
| `SummariesShowLocalCopy(uploadId: number) -> void` | Reveals the `.md` in Finder (FR-018). |

**Added to existing methods** (signatures unchanged):

| Method | Added behaviour |
|---|---|
| `UploadStart`, `UploadRetry` | When a caption job is created and summaries are set up, also create a SummaryJob in `waiting_for_captions`. |
| `UploadCancel`, upload failure | Cancel the summary job; a local `.md` already saved is kept (FR-016). |
| `UploadDelete` | Deletes the job row; does not touch files in Drive or on disk. |
| `UploadListRecent` | Fills `summary`. |

## Events

| Event | Payload | When |
|---|---|---|
| `summaries:updated` | `SummaryJobDTO` | Every status or phase change |

## UI contract

- **Settings → "Sermon summaries"**: an off-by-default switch; an API-key
  field (masked once saved, with "Remove key"); a "Get a key" link to the
  Anthropic console; and the FR-015 notice: "Only the transcript text is
  sent to Claude (Anthropic) — never the audio or video. Anthropic charges
  your account, about $0.10–$0.40 per sermon." When `available` is false,
  the controls are disabled and `unavailableReason` is shown.
- **Transfer rows**: a summary line under the caption line, separate from
  both the upload and caption status (FR-011), e.g.
  - "Summary: waiting for captions"
  - "Summary: writing…"
  - "Summary ready on this Mac — waiting for the video"
  - "Summary ready"
  - "Summary couldn't be made — Your AI account is out of credit [Try again]"
- **Transfer details**: "Open summary in Drive" and "Show summary in
  Finder" (FR-018).
