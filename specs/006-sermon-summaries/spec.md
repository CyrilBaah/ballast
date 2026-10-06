# Feature Specification: Automatic Sermon Summaries

**Feature Branch**: `006-sermon-summaries`

**Created**: 2026-10-06

**Status**: Draft

**Input**: User description: "Automatic sermon summaries. After a video's captions are made (Feature 005), Ballast automatically sends the transcript text — never the audio or video — to a cloud AI model and saves a summary file next to the video in the same Google Drive folder, so each sermon folder ends up with the video, its caption file, and its summary with no manual step. The summary contains: a short overview paragraph, the main points, Bible references mentioned, key quotes with timestamps so people can jump to that moment, and a suggested title and description that can be reused for YouTube or social posts. Uploading the video successfully remains the core of the app: summarising must never slow, block, or fail the video upload or the captions, and if it fails (no internet, no API key, provider error) the video and captions are unaffected and the user sees why. The feature is off until the user turns it on and enters their own API key, which is stored securely (OS keychain) like the Google sign-in. Recommended provider is Claude (Anthropic) because it handles very long transcripts (a 3-hour sermon is ~30,000 words) in one go; the provider should be swappable (e.g. Google Gemini). Summaries start only after the transcript exists; a copy is also saved on the user's Mac next to the video like the caption file. Sermons may mix English with Twi/Ga. Context: Google Drive's own AI video summary failed for one 13 GB sermon (likely because Drive could not caption it) but worked for a larger 15 GB one, so relying on Drive is unreliable."

## Background

Google Drive offers its own AI summary of a video, but it is unreliable for the sermons Ballast uploads. It failed for a 13 GB sermon (most likely because Drive could not caption it) yet worked for a larger 15 GB one, so whether a sermon gets a summary is out of the user's hands. Feature 005 makes Ballast produce its own transcript of every video. This feature builds on that transcript to produce a summary of every sermon, every time, without relying on Drive.

**Depends on**: Feature 005 (Automatic Video Captions). A summary is made only from a transcript that Feature 005 produced.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every sermon gets a summary next to it in Drive (Priority: P1)

A user uploads a recorded sermon as usual. Once its captions are made, Ballast produces a summary and places it in the same Drive folder, so the folder holds the video, its caption file, and its summary, with no extra steps. The summary gives a short overview, the main points, the Bible passages referred to, a handful of memorable quotes with the time they were said, and a suggested title and description ready to paste into YouTube or social media.

**Why this priority**: It is the whole point of the feature: a dependable summary for every sermon, including the long ones Drive refuses to summarise.

**Independent Test**: With summaries turned on and a valid key entered, upload a sermon video. Confirm that a summary appears in the same Drive folder, named after the video, containing all six parts, and that every quote in it can be found word-for-word in the caption file at the stated time.

**Acceptance Scenarios**:

1. **Given** summaries are turned on and set up, **When** a video's captions are made, **Then** a summary is produced automatically and, once the video is in Drive, placed in the same folder, named after the video (e.g. `Sermon.mp4` → "Sermon — Summary").
2. **Given** a summary has been produced, **When** the user opens it, **Then** it contains: an overview paragraph, the main points, the Bible references mentioned, key quotes each with its time in the video, a suggested title, and a suggested description.
3. **Given** a sermon of 3 hours or more, **When** its summary is produced, **Then** it covers the whole sermon, including points made in the last hour, and not just the beginning.
4. **Given** a summary is being made, **When** the user looks at the transfer, **Then** they see the summary's status (waiting for captions, in progress, ready, or could not be made) separately from the upload's and the captions' status.
5. **Given** a summary is ready, **When** the user opens the transfer's details, **Then** they can open it in Drive, or show the copy on their Mac.

---

### User Story 2 - Summaries never put the upload or captions at risk (Priority: P1)

Summarising happens after captions and off to the side. If the internet drops, the key is wrong, or the AI service has a problem, the video still uploads and the captions still arrive exactly as they would without this feature. The user is told plainly why the summary could not be made and can try again with one click.

**Why this priority**: A successful upload is the core of Ballast. A summary feature that risks the upload or the captions is a net loss, so this protection is a precondition for shipping Story 1.

**Independent Test**: Enter an invalid key and upload a video. Confirm the video and captions complete exactly as with summaries off, and the transfer shows "Summary couldn't be made — your AI key was rejected". Fix the key, choose Try again, and confirm the summary is produced.

**Acceptance Scenarios**:

1. **Given** summarising fails for any reason (no internet, rejected key, account out of credit, service error), **When** the upload and captions complete, **Then** both are recorded as succeeded, and the summary shows as failed with a plain-language reason.
2. **Given** a summary failed, **When** the user chooses Try again, **Then** it is attempted again from the existing transcript, without re-uploading the video or redoing the captions.
3. **Given** the AI service is temporarily unavailable, **When** summarising is attempted, **Then** it is retried automatically a few times before being reported as failed.
4. **Given** the user cancels a video upload, **When** the cancellation takes effect, **Then** no summary is uploaded to Drive for it.
5. **Given** captions could not be made for a video, or found no speech, **When** captioning ends, **Then** no summary is attempted, and the transfer says why ("No transcript to summarise").

---

### User Story 3 - Turn summaries on with your own AI key (Priority: P1)

The feature is off until the user switches it on in Settings. Turning it on asks for an API key from the AI provider, which Ballast stores securely in the computer's password store (like the Google sign-in) and checks right away. Before they switch it on, the user is told clearly what leaves their computer: the transcript text only, never the audio or video, sent to the named AI provider.

**Why this priority**: Nothing works until it is set up, and sending transcript text to an outside service needs the user's informed choice.

**Independent Test**: In Settings, turn summaries on, enter a key, and confirm Ballast reports the key as working. Quit and reopen Ballast and confirm the key is still set and is not stored in any readable file. Turn summaries off and confirm the next upload makes no summary and sends nothing to the AI provider.

**Acceptance Scenarios**:

1. **Given** a new install, **When** the user uploads a video, **Then** no summary is made and nothing is sent to any AI provider.
2. **Given** the user turns summaries on, **When** they enter a key, **Then** Ballast checks it immediately and says whether it works, before any sermon is sent.
3. **Given** the user is turning summaries on, **When** the setting is shown, **Then** it states that only the transcript text is sent, to which provider, that the provider may charge for it, and roughly what a typical sermon costs.
4. **Given** a key has been entered, **When** Ballast is quit and reopened, **Then** the key is still set, is kept only in the computer's secure password store, and is never shown in full again.
5. **Given** the user turns summaries off or removes the key, **When** later videos are uploaded, **Then** no summaries are made for them.

---

### User Story 4 - Use the summary before the upload finishes (Priority: P2)

A long sermon can take hours to upload. As soon as its summary is ready, a copy is saved on the user's Mac next to the original video, the same way Feature 005 saves the caption file, so the user can use the title and description right away. The Drive copy follows once the video has arrived.

**Why this priority**: Useful, and it mirrors the caption file's behaviour, but the summary in Drive (Story 1) is the main deliverable.

**Independent Test**: Upload a large video and confirm the summary appears next to the local video before the upload completes, and in Drive only after the video arrives.

**Acceptance Scenarios**:

1. **Given** a video is still uploading, **When** its summary is ready, **Then** a copy is saved immediately next to the original video on the Mac.
2. **Given** the video upload is later cancelled, **When** the cancellation takes effect, **Then** the local copy stays on the Mac and nothing is put in Drive.

---

### Edge Cases

- **Mixed English and Twi/Ga sermons**: the summary is written in English. Passages whose transcript is unclear are summarised only as far as the transcript supports; nothing is invented to fill gaps.
- **Very long transcripts (3+ hours, ~30,000+ words)**: summarised as a whole. If a transcript is ever too long for the provider to take in one go, it is summarised in parts that are then combined, and the result still covers the whole sermon.
- **Long stretches of worship music or announcements**: not treated as sermon content; the summary focuses on the preaching.
- **No Bible references in a sermon**: the section says "None mentioned" rather than inventing any.
- **The user's provider account runs out of credit, or hits a usage limit**: the summary fails with that reason and can be retried later; nothing else is affected.
- **The key is removed or rejected after summaries were working**: pending summaries fail with "your AI key was rejected or removed", and Settings flags the key as needing attention.
- **The app quits or the computer restarts mid-summary**: the summary is started again automatically on the next launch, from the saved transcript.
- **A summary with the same name already exists in the Drive folder or next to the local video**: the existing one is not overwritten; the new one gets a distinguishing suffix, the same rule as caption files in Feature 005.
- **Summaries on but captions off**: no transcript exists, so no summary can be made. Settings explains that summaries need captions turned on.
- **Platforms without captions (Feature 005: Intel Macs, Windows, Linux)**: summaries are unavailable for the same reason, and Settings says so.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST produce a summary automatically for every video whose captions Feature 005 completes with speech, when summaries are turned on and set up, with no action from the user beyond starting the upload.
- **FR-002**: Each summary MUST contain six parts: an overview paragraph, the main points, the Bible references mentioned (or "None mentioned"), key quotes each with its time in the video, a suggested title, and a suggested description suitable for YouTube or social media.
- **FR-003**: Every quote in a summary MUST appear word-for-word in the transcript, and its stated time MUST match when it was said; the summary MUST NOT present invented quotes or references.
- **FR-004**: The summary MUST cover the whole sermon, however long, not only its opening portion.
- **FR-005**: Only the transcript text MAY be sent to the AI provider; the audio and video MUST never leave the user's computer for this feature.
- **FR-006**: The summary MUST be placed in the same Drive folder as its video, named after the video, only after the video has been confirmed in Drive; it MUST NOT overwrite an existing file.
- **FR-007**: As soon as a summary is ready, the system MUST save a copy on the user's computer next to the original video, without waiting for the video upload, following the same placement and naming rules as Feature 005's local caption copy.
- **FR-008**: Summarising MUST NOT slow, block, or fail the video upload or the captions; their behaviour MUST be identical whether summaries are on or off.
- **FR-009**: If summarising fails, the system MUST show a plain-language reason and offer "Try again", which reuses the existing transcript.
- **FR-010**: Temporary service failures MUST be retried automatically a limited number of times before the summary is reported as failed.
- **FR-011**: The system MUST show each video's summary status (waiting for captions, in progress, ready, could not be made) separately from its upload and caption status.
- **FR-012**: Summaries MUST be off by default. Users MUST be able to turn them on or off in Settings; the setting MUST persist across restarts and apply to videos whose captions finish after the change.
- **FR-013**: Turning summaries on MUST require the user's own API key for the AI provider. The key MUST be stored only in the operating system's secure password store, MUST never be written to logs or readable files, and MUST never be shown in full after entry.
- **FR-014**: The system MUST check a newly entered key immediately and tell the user whether it works, before any transcript is sent.
- **FR-015**: Before summaries are turned on, the system MUST tell the user that only the transcript text is sent, which provider receives it, that the provider may charge for it, and an approximate cost for a typical sermon.
- **FR-016**: Cancelling a video upload MUST prevent its summary from being uploaded to Drive; a local copy already saved stays on the user's computer.
- **FR-017**: Interrupted summaries MUST be restarted automatically from the saved transcript on the next launch.
- **FR-018**: The user MUST be able to open a finished summary in Drive, or show its local copy, from the transfer's details.
- **FR-019**: The system MUST be designed so that the AI provider can be changed without changing how summaries look or behave; this version MUST ship with one provider.

### Key Entities *(include if feature involves data)*

- **Summary job**: The work of summarising one video. Linked to exactly one upload and its caption job. Has a status (waiting for captions, in progress, ready, failed, cancelled), a failure reason when failed, the number of attempts made, the provider used, and once ready, references to the Drive copy and the local copy. Survives app restarts.
- **Summary**: The produced document with its six parts (FR-002), named after its video, stored in the video's Drive folder and next to the original video on the user's computer.
- **Summary settings**: On/off (default off) and the chosen provider, remembered across restarts. The API key itself is held separately in the secure password store, never with the other settings.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With summaries set up, 100% of sermons whose captions complete with speech get a summary in their Drive folder, including those of 3 hours or more and those Drive's own summary cannot handle.
- **SC-002**: In 100% of tested summaries, every quote appears word-for-word in the transcript within 5 seconds of its stated time, and every Bible reference listed is actually mentioned in the transcript.
- **SC-003**: For a 3-hour sermon, the main points include at least one point from each hour of the sermon.
- **SC-004**: A summary is ready within 5 minutes of its captions finishing, for a sermon of up to 3 hours, on a normal internet connection.
- **SC-005**: In 100% of tested summary failures (no internet, rejected key, out of credit, service outage), the video upload and captions still succeed and the user sees a specific reason.
- **SC-006**: Zero audio or video bytes are sent to the AI provider, confirmed by observing network traffic; the only data sent is transcript text.
- **SC-007**: The API key never appears in Ballast's logs, database, or any file on disk, confirmed by searching them after setup.
- **SC-008**: A user can turn summaries on, enter a key, and have it confirmed working in under 2 minutes.

## Assumptions

- **Provider**: This version ships with Claude (Anthropic), chosen because it can read a whole multi-hour transcript in one go. Supporting other providers, such as Google Gemini, is a later addition the design allows for (FR-019); the provider choice in Settings has one option in this version.
- **The user pays the provider**: the user brings their own key and is billed by the provider. Ballast does not charge, meter, or proxy anything. The approximate per-sermon cost shown (FR-015) is an estimate, worked out during planning from the provider's published prices.
- **Summary format**: In Drive, the summary is a Google Doc, so it can be read, shared, and edited directly in Drive. The local copy is a plain-text Markdown file with the same content.
- **Language**: Summaries are always written in English, whatever languages the sermon mixes.
- **Quote timings**: Quote times come from the caption file's timings and are shown as hours:minutes:seconds, which viewers can use to jump to that point in Drive's player.
- **Order of work**: A summary starts as soon as the transcript is ready, which may be while the video is still uploading. Its local copy is saved immediately; its Drive copy waits for the video, as with caption files.
- **Platforms**: Summaries are available wherever Feature 005's captions are (macOS on Apple silicon), since they need its transcript.
- **Scope**: Only videos captioned after summaries are turned on are summarised. Summarising older videos already in Drive, translating summaries, chat about a sermon, and editing summaries inside Ballast are out of scope.
- **Running cost of retries**: Automatic retries (FR-010) are limited to a small number so a failing service never runs up repeated charges.
