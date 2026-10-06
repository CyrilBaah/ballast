# Feature Specification: Automatic Video Captions

**Feature Branch**: `005-automatic-video-captions`

**Created**: 2026-10-06

**Status**: Draft

**Input**: User description: "Automatic captions for uploaded videos. When a user uploads a video with Ballast, it should arrive in Google Drive with captions without the user doing anything extra and without any loss of video quality — the original file is uploaded untouched. Ballast extracts the audio locally, transcribes it on the user's own Mac with a speech-to-text engine bundled in the app (free, no account or API key, no usage limits, nothing leaves the machine — e.g. whisper.cpp), and produces a standard caption file (.srt) named after the video, placed in the same Drive folder next to it. Transcription runs alongside or after the upload and must never slow, block, or fail the video upload itself; if captioning fails, the video upload still succeeds and the user sees that captions could not be made and why. Google Drive cannot have a caption track attached through its API, so the spec should also cover an investigation of embedding captions inside the MP4 as a soft subtitle track via a lossless remux (no re-encode) and whether Drive's player shows it. Context: a 13 GB, multi-hour sermon video got "Captions can't be generated for this video" from Drive's own automatic captions, likely due to its size/length. Sermons may mix English with Ghanaian languages (Twi, Ga), which transcription may handle poorly. Users can turn the feature on or off."

## Background

Google Drive's own automatic captions refused a 13 GB, multi-hour sermon recording uploaded through Ballast ("Captions can't be generated for this video"), most likely because of its size or length. Shrinking or re-encoding the video before upload would make Drive more willing to caption it, but it lowers the video's quality, which the user does not accept. This feature makes captions independent of Drive's own captioning: Ballast produces them itself, on the user's computer, from the video's audio, and leaves the video file untouched.

## Clarifications

### Session 2026-10-06

- Q: Should Ballast include the speech-recognition model in the app download, or download it the first time a video is captioned? → A: Download it once, automatically, the first time a video is captioned, with progress shown; later captioning works offline.
- Q: How should Ballast decide which language the speech is in when it makes captions? → A: A caption-language setting next to the on/off switch, defaulting to English, with an "Automatic" choice; it applies to all videos until changed.
- Q: Should the first version of captions work on macOS only, or on Windows and Linux too? → A: macOS only for this version; Windows and Linux show "Captions aren't available on this system yet", and support there is a follow-up feature.
- Q: When captions are made, should Ballast favour accuracy or speed and a smaller download? → A: High accuracy: a single high-accuracy model (about a 1.5 GB one-time download), with no speed/accuracy setting; it must still caption faster than the video plays on an Apple-silicon Mac.
- Q: Should automatic captions be switched on by default for a new Ballast install? → A: On by default, but the first time a video is uploaded Ballast asks once before the one-time model download; after that, every video is captioned with no extra steps.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A video arrives in Drive with a caption file next to it (Priority: P1)

A user uploads a long recorded sermon. They do nothing beyond what they do today: pick the file, pick the folder, start the upload. The full-quality video lands in the chosen Drive folder as before, and shortly afterwards a caption file with the same name sits next to it, ready to attach in Drive's player or to share with anyone who needs it.

**Why this priority**: This is the whole point of the feature — captions for videos that Drive will not caption itself, with no extra effort and no loss of quality. Everything else builds on having a caption file at all.

**Independent Test**: Upload a video with spoken audio, with captions turned on. Confirm the video in Drive is byte-for-byte identical to the local file, and that a caption file named after the video appears in the same folder, with readable text whose timings line up with the speech when played alongside the video.

**Acceptance Scenarios**:

1. **Given** a new install, **When** the user uploads their first video, **Then** the upload starts immediately and Ballast asks once whether to download the speech model (showing its size); agreeing starts captioning for that video and the question is never asked again.
2. **Given** captions are turned on and the model is already downloaded, and the user uploads a video file with spoken audio, **When** the upload and captioning both finish, **Then** the chosen Drive folder holds the original video, unchanged, and a caption file with the same base name (e.g. `Sermon.mp4` → `Sermon.srt`).
3. **Given** a video is being captioned, **When** the user looks at that transfer in Ballast, **Then** they can see the captioning status (waiting, in progress with an indication of progress, done, or could not be made) separately from the video upload's own status.
4. **Given** captions are turned on and the user uploads a file that is not a video (e.g. a PDF or a photo), **When** the upload finishes, **Then** no captioning is attempted and nothing about captions is shown for that file.
5. **Given** captioning has finished for a video, **When** the user opens the transfer's details, **Then** they can open the caption file in Drive directly from Ballast.

---

### User Story 2 - Captioning never puts the video upload at risk (Priority: P1)

A user uploads a multi-hour video over a slow or unreliable connection. Captioning works alongside the upload, but the upload is the priority: it is not slowed down, held back, or failed because of captioning. If captioning fails for any reason, the video still uploads normally, and the user is told plainly that captions could not be made and why.

**Why this priority**: Ballast exists to get large files into Drive reliably. A captioning feature that endangers that is a net loss; this protection is a precondition for shipping Story 1, so it shares P1.

**Independent Test**: Upload a video while forcing captioning to fail (e.g. a video with no audio track, or an unavailable speech engine). Confirm the video upload completes exactly as it would with captions turned off, and that the transfer shows a clear "captions could not be made" reason.

**Acceptance Scenarios**:

1. **Given** captioning fails for a video, **When** the video upload itself completes, **Then** the upload is recorded as succeeded, and captioning is shown separately as failed with a plain-language reason.
2. **Given** a video upload and its captioning are both running, **When** the upload pauses for a network drop, resumes, or is restarted, **Then** the upload behaves exactly as it does with captions turned off — same resume point, same retry behaviour, no extra bytes re-sent.
3. **Given** the user cancels a video upload, **When** the cancellation takes effect, **Then** its captioning also stops, and no caption file is uploaded for a video that never arrived.
4. **Given** the video upload finishes before captioning does, **When** the user looks at the transfer, **Then** the video is shown as uploaded and captions as still in progress — the upload is not held back waiting for captions.

---

### User Story 3 - Turn automatic captions on or off (Priority: P2)

A user who does not want captions — for example, for videos with no speech, or to save battery on a laptop — turns the feature off in Settings. From then on, uploads behave exactly as they did before this feature existed. Turning it back on applies to videos uploaded afterwards.

**Why this priority**: Captioning a multi-hour video uses noticeable computing power and time. The user asked for control over it, but the feature delivers value with a sensible default even before the switch exists.

**Independent Test**: Turn captions off, upload a video, and confirm no caption file is made and no captioning status appears. Turn them back on and confirm the next video upload is captioned. Change the caption language and confirm the next video is captioned in the chosen language.

**Acceptance Scenarios**:

1. **Given** captions are turned off, **When** the user uploads a video, **Then** no audio is processed, no caption file is created, and no captioning status is shown.
2. **Given** the user changes the setting, **When** they quit and reopen Ballast, **Then** the setting is remembered.
3. **Given** the caption language is left at its default, **When** a mostly-English video that opens with Twi singing is captioned, **Then** it is captioned as English rather than in a language guessed from its opening.
4. **Given** a video is currently being captioned, **When** the user turns captions off, **Then** captioning already in progress finishes or stops (see Assumptions) and later uploads are not captioned.

---

### User Story 4 - Find out whether captions can live inside the video itself (Priority: P3)

A caption file next to the video still has to be attached by hand in Drive's player, because Google gives apps no way to attach one. Before committing to a better experience, the team investigates whether captions can instead be carried *inside* the video file as a selectable caption track — without re-encoding, so the picture and sound stay exactly as they were — and whether Google Drive's player actually shows such a track.

**Why this priority**: If it works, captions would appear in Drive with no manual step at all, which is the experience the user actually wants. But it is uncertain, and changing the uploaded file at all touches the "original file is uploaded untouched" promise, so it is an investigation with a recorded finding, not a committed behaviour.

**Independent Test**: Take a sample video, add a caption track to it without re-encoding, upload it to Drive, and record whether Drive's player offers the captions, how long the extra step takes for a multi-hour video, and whether the picture and sound are unchanged.

**Acceptance Scenarios**:

1. **Given** the investigation is complete, **When** the team reviews it, **Then** there is a written finding stating whether Drive's player shows an embedded caption track, tested on at least one short and one multi-hour video, with the evidence (e.g. screenshots).
2. **Given** the finding is positive, **When** the team decides how to proceed, **Then** any follow-up that changes the uploaded file is specified separately and keeps the user's control over whether their original file is altered.

---

### Edge Cases

- **Video with no audio track, or only music/silence**: captioning is skipped or reports "no speech found"; the upload is unaffected and no empty caption file is uploaded.
- **Very long or very large videos (multi-hour, tens of GB)**: captioning must complete without unbounded memory growth and must not depend on reading the whole file into memory.
- **Speech in languages the engine handles poorly (e.g. Twi, Ga mixed with English)**: captions are still produced for what is recognised; the user is told up front that accuracy may be lower for languages other than English, rather than being given silently poor captions.
- **The app quits, crashes, or the computer restarts mid-captioning**: on the next launch captioning resumes or restarts for that video without the user doing anything, and the video upload's own recovery is unaffected.
- **A caption file with the same name already exists in the destination folder**: the existing file is not silently overwritten; the new one gets a distinguishable name (see Assumptions).
- **The video file is moved, changed, or deleted locally before captioning finishes**: captioning stops with a clear reason; the video upload follows its existing file-changed/file-missing rules.
- **The user is signed out, or Drive storage is full, when the caption file is ready**: the caption file waits and is uploaded once possible, or fails with Drive's reason; it never blocks or fails the video upload.
- **Low disk space for temporary audio**: captioning fails with a clear reason rather than filling the disk; temporary files are always cleaned up.
- **Several videos queued**: captioning work is done one video at a time so it does not overwhelm the computer.
- **The one-time model download fails, is interrupted, or there is no internet the first time**: that video's captioning waits and retries the download later (resuming where possible), or fails with a clear reason; the video upload is unaffected, and a damaged or partial model is never used.
- **Windows or Linux**: the caption settings show that captions aren't available on this system yet, and uploads proceed exactly as with captions off (FR-017).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST upload video files byte-for-byte unchanged when captions are turned on; captioning MUST NOT alter, re-encode, or compress the uploaded video.
- **FR-002**: When captions are turned on, the system MUST automatically produce a caption file for each uploaded video that contains speech, without any extra action from the user beyond starting the upload.
- **FR-003**: Speech recognition MUST run entirely on the user's computer: no audio, video, or transcript may be sent to any outside service, and the feature MUST work without an account, API key, payment, or usage limit.
- **FR-004**: The caption file MUST be in a standard, widely supported caption format (SubRip, `.srt`) that Google Drive's player accepts as a caption track.
- **FR-005**: The caption file MUST be placed in the same Drive folder as its video and named after it (same base name, caption extension).
- **FR-006**: Captioning MUST NOT slow, pause, block, or fail the video upload; the upload's resume, retry, and recovery behaviour MUST be identical whether captions are on or off.
- **FR-007**: If captioning fails, the video upload MUST still be able to succeed, and the user MUST be shown that captions could not be made, with a plain-language reason.
- **FR-008**: The system MUST show each video's captioning status (waiting, in progress with progress indication, done, could not be made) separately from its upload status.
- **FR-009**: Automatic captions MUST be on by default for a new install. Users MUST be able to turn them on or off; the setting MUST persist across app restarts and apply to uploads started after the change.
- **FR-010**: The system MUST only attempt captioning for video files; other files MUST be uploaded exactly as today with no captioning status.
- **FR-011**: Cancelling a video upload MUST stop its captioning, and the system MUST NOT upload a caption file for a video that did not arrive in Drive.
- **FR-012**: Captioning MUST survive an app quit, crash, or restart: interrupted captioning MUST resume or restart automatically on the next launch.
- **FR-013**: Captioning MUST use bounded memory regardless of video length or file size, and MUST clean up all temporary files when it finishes, fails, or is cancelled.
- **FR-014**: The system MUST NOT overwrite an existing file in the destination folder when uploading a caption file.
- **FR-015**: The system MUST tell the user, where they turn the feature on, that captions are generated automatically, that accuracy may be lower for languages other than English, and that captions are stored as a separate file they can attach to the video in Drive.
- **FR-016**: The user MUST be able to open a finished caption file in Drive from the transfer's details in Ballast.
- **FR-017**: Captioning MUST work on macOS in this version. On Windows and Linux, the caption settings MUST be shown as unavailable with the message "Captions aren't available on this system yet", no captioning status may appear on transfers, and uploads MUST behave exactly as they do with captions turned off.
- **FR-018**: The team MUST investigate and record, in writing, whether a caption track embedded in the video without re-encoding is shown by Google Drive's player (User Story 4); this feature MUST NOT ship any change to the uploaded video file based on that investigation.
- **FR-019**: The speech-recognition model MUST NOT be part of the app download; it MUST be downloaded once, the first time a video is captioned, with its download progress shown as part of that video's captioning status. Before that first download starts, the system MUST ask the user once, stating the download size; if they agree, the download proceeds and is never asked about again, and if they decline, that video is uploaded without captions and captions are switched off (the user can turn them back on in Settings, which starts the download). The video upload MUST start immediately regardless of the answer and MUST NOT wait for it. Once downloaded, captioning MUST work without an internet connection, and the model MUST NOT be downloaded again unless it is missing or damaged.
- **FR-020**: Users MUST be able to choose the caption language in a setting next to the on/off switch. The choice MUST default to English, MUST offer "Automatic" (the language is detected from the speech), MUST persist across app restarts, and MUST apply to every video captioned after it is changed.
- **FR-021**: Captioning MUST use a single high-accuracy speech model, chosen for handling accented English and recordings made in noisy halls; there is no speed/accuracy setting. The one-time model download (FR-019) MAY be up to about 1.5 GB.

### Key Entities *(include if feature involves data)*

- **Captioning job**: The work of producing captions for one uploaded video. Linked to exactly one upload. Has a status (waiting, in progress, done, failed, cancelled), progress, a failure reason when failed, and, once done, a reference to the caption file in Drive. Survives app restarts.
- **Caption file**: The produced transcript with timings, in SubRip format, named after its video and stored in the same Drive folder.
- **Caption settings**: The user's on/off preference for automatic captions and their caption language (English by default, or "Automatic"), both remembered across restarts. Each captioning job records the language it used.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of videos uploaded with captions on are byte-for-byte identical in Drive to the local file.
- **SC-002**: For an English-language video with clear speech, a caption file appears next to the video in Drive with no user action beyond starting the upload, and its timings stay within 1 second of the speech throughout.
- **SC-003**: A multi-hour (at least 3 hours, at least 10 GB) video is captioned successfully on a recent Apple-silicon Mac using the high-accuracy model (FR-021), with captioning taking no longer than the video's own running time (once the model is downloaded).
- **SC-004**: Upload duration for a video with captions on is within 5% of the same upload with captions off, on the same connection.
- **SC-005**: In 100% of tested captioning failures (no audio, engine unavailable, disk full, file removed), the video upload still succeeds and the user sees a specific reason.
- **SC-006**: Memory used by captioning stays flat regardless of video length — a 10-minute and a 3-hour video use a comparable peak amount.
- **SC-007**: Zero audio, video, or transcript bytes leave the user's computer during captioning, confirmed by observing network traffic.
- **SC-008**: The embedded-caption investigation (User Story 4) produces a written yes/no finding, with evidence, before any follow-up work on changing the uploaded file is planned.

## Assumptions

- **Default**: Automatic captions are on by default for new installs (FR-009), with a one-time confirmation before the first model download (FR-019), so a large download never happens as a surprise.
- **Turning off mid-job**: Switching captions off lets a caption job that is already running finish; it only stops new videos from being captioned. Cancelling an upload is the way to stop its captioning.
- **Name clashes**: If a caption file with the same name already exists in the folder, the new one is uploaded alongside it with a distinguishing suffix (e.g. `Sermon (2).srt`) rather than replacing it.
- **Language**: Captions use the language chosen in settings (FR-020): English by default, so a mostly-English sermon that opens in Twi or Ga is not mis-detected. English is the reference language for accuracy targets; Ghanaian languages (Twi, Ga) are best effort and called out to the user (FR-015). Which other languages the setting lists is a plan decision, limited to those the engine supports.
- **Attaching captions in Drive**: Because Google provides no way for an app to attach a caption track to a Drive video, the user (or anyone they share with) attaches the caption file in Drive's player by hand. This version does not remove that step; User Story 4 investigates whether it can be removed.
- **Order of work**: Captioning may begin while the video is still uploading, but the caption file is uploaded only after the video has been confirmed in Drive.
- **Scope**: Only videos uploaded after the feature is turned on are captioned; captioning videos already in Drive, editing captions, translation, and burned-in (on-picture) captions are out of scope.
- **Speech engine**: A free, open-source speech-to-text engine that runs offline (whisper.cpp is the leading candidate) ships with Ballast, along with whatever is needed to read a video's audio; its speech model is downloaded once on first use (FR-019). Only the model is downloaded — no audio, video, or transcript is ever sent out (FR-003). Adding the engine is a new major dependency, which the plan must justify under Constitution Principle I; the plan picks the specific high-accuracy model (FR-021) and confirms it meets SC-003.
- **Platforms**: This version supports macOS only, where the user runs Ballast. Windows and Linux say clearly that captions are not yet available (FR-017), which satisfies Constitution Principle VII; captioning on those systems is a separate follow-up feature.
- **Computer load**: Captioning a multi-hour video takes noticeable processing time and power; running one captioning job at a time, at a lower priority than the user's other work, is acceptable.
