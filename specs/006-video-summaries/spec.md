# Feature Specification: Automatic Video Summaries

**Feature Branch**: `006-video-summaries`

**Created**: 2026-10-06

**Status**: Draft. Revised twice on 2026-10-06:
1. Summaries are made on the user's Mac at no cost, replacing the original cloud-AI design.
2. Summaries cover any video with speech, not only sermons.

**Input**: User description: "Automatic sermon summaries. After a video's captions are made (Feature 005), Ballast automatically sends the transcript text — never the audio or video — to a cloud AI model and saves a summary file next to the video in the same Google Drive folder, so each sermon folder ends up with the video, its caption file, and its summary with no manual step. The summary contains: a short overview paragraph, the main points, Bible references mentioned, key quotes with timestamps so people can jump to that moment, and a suggested title and description that can be reused for YouTube or social posts. Uploading the video successfully remains the core of the app: summarising must never slow, block, or fail the video upload or the captions, and if it fails (no internet, no API key, provider error) the video and captions are unaffected and the user sees why. […] Context: Google Drive's own AI video summary failed for one 13 GB sermon (likely because Drive could not caption it) but worked for a larger 15 GB one, so relying on Drive is unreliable." Revised by the user: "let make so it cost 0" → summaries run on an AI model on the user's own Mac. Revised again by the user: "it should be the summary of the video not sermon — mean it can any video ok" → summaries work for any video with speech.

## Background

Google Drive offers its own AI summary of a video, but it is unreliable for the videos Ballast uploads. It failed for a 13 GB sermon (most likely because Drive could not caption it) yet worked for a larger 15 GB one, so whether a video gets a summary is out of the user's hands. Feature 005 makes Ballast produce its own transcript of every video, on the user's Mac. This feature builds on that transcript to produce a summary of every video that contains speech: sermons, talks, lessons, meetings, interviews, documentaries. It is made on the user's Mac, free, and nothing is sent anywhere except the finished summary to the user's own Drive.

**Depends on**: Feature 005 (Automatic Video Captions). A summary is made only from a transcript that Feature 005 produced.

## Clarifications

### Session 2026-10-06

- Q: Should the quotes in a summary be exactly as transcribed, or lightly tidied into clear English? → A: Lightly tidied into clear English with the meaning and time kept; each quote must still trace back to the passage of the transcript it came from, so nothing is invented.
- Q: Should each summary also include a ready-to-share version for a group chat? → A: Yes, a second, shorter section written for the people who will receive it: plain text with WhatsApp bold formatting and no emojis, ready to copy and paste.
- Q: When a reference in the recording is unclear (e.g. a Bible verse), how should the summary handle it? → A: Use the most likely correct reference in the summary and share-ready text, and list those guessed references separately for the user to check before sharing.
- Q: Should a summary include housekeeping such as announcements, or only the main content? → A: Main content only; announcements, thanks, notices, and other housekeeping are left out.
- Q: Should summaries cost anything, and where should they be made? → A: They must cost nothing. Summaries are made by a free AI model running on the user's own Mac, the same way captions are, with a one-time model download. There is no API key, no account, and no usage charge, and the transcript never leaves the Mac. A paid cloud provider may be offered later as an optional "better quality" setting, but not in this version.
- Q: Are summaries only for sermons? → A: No. Any video with speech gets a summary. Content that only applies to some videos, such as Bible references for a sermon or the speaker's calls to action, appears only when the video actually has it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every video gets a summary next to it in Drive (Priority: P1)

A user uploads a video with speech as usual: a sermon, a lecture, a team meeting, a documentary. Once its captions are made, Ballast produces a summary on the user's Mac and places it in the same Drive folder, so the folder holds the video, its caption file, and its summary, with no extra steps and at no cost. The summary gives:
- a short overview;
- the main points;
- key quotes, with the time each was said;
- a suggested title and description;
- a message ready to paste into a group chat.

When the video contains them, it also lists the references mentioned (for example Bible verses in a sermon, or books and sources in a talk) and the speaker's calls to action.

**Why this priority**: It is the whole point of the feature: a dependable, free summary of every video with speech, including the ones Drive refuses to summarise.

**Independent Test**: With summaries on and the summary model downloaded, upload one sermon and one non-religious video (e.g. a short documentary). For each, confirm that:
- a summary appears in the same Drive folder, named after the video;
- every quote matches the meaning of the transcript at the stated time;
- the ready-to-share message can be pasted into WhatsApp unchanged;
- the sermon's summary lists its Bible references and the documentary's summary has no Bible section;
- nothing but the finished summary left the Mac.

**Acceptance Scenarios**:

1. **Given** summaries are on, **When** the first video's captions are made, **Then** Ballast asks once whether to download the summary model (showing its size); agreeing downloads it, and the summary is then made.
2. **Given** summaries are on and the model is downloaded, **When** a video's captions are made, **Then** a summary is produced automatically on the Mac and, once the video is in Drive, placed in the same folder, named after the video (e.g. `Lecture.mp4` → "Lecture — Summary").
3. **Given** a summary has been produced, **When** the user opens it, **Then** it always contains: an overview paragraph, the main points, key quotes each with its time in the video, a suggested title, a suggested description, and a ready-to-share message.
4. **Given** the video mentions references (Bible verses, books, studies, laws, or other named sources), **When** the summary is produced, **Then** it includes a references section listing them; **Given** it mentions none, **Then** that section is left out entirely rather than shown empty.
5. **Given** the speaker asks the audience to do something (e.g. "pray daily", "submit by Friday"), **When** the summary is produced, **Then** it includes a "Key takeaways / actions" section; otherwise that section is left out.
6. **Given** a summary has been produced, **When** the user copies its ready-to-share message into a WhatsApp group, **Then** it reads cleanly with bold headings and contains no emojis, no timestamps, no notes about the transcript, and no housekeeping announcements, and it needs no editing before sending.
7. **Given** the recording made a reference unclear, **When** the summary is produced, **Then** the most likely reference is used in the text, and a separate "Check before sharing" note (outside the ready-to-share message) lists each guessed reference and what was actually heard.
8. **Given** a video of 3 hours or more, **When** its summary is produced, **Then** it covers the whole video, including points made in the last hour, not just the beginning.
9. **Given** a summary is being made, **When** the user looks at the transfer, **Then** they see the summary's status (waiting for captions, downloading model, writing, ready, or could not be made) separately from the upload's and the captions' status.
10. **Given** a summary is ready, **When** the user opens the transfer's details, **Then** they can open it in Drive, or show the copy on their Mac.

---

### User Story 2 - Summaries never put the upload or captions at risk (Priority: P1)

Summarising happens after captions and off to the side. If summarising fails for any reason, the video still uploads and the captions still arrive exactly as they would without this feature. The user is told plainly why the summary could not be made and can try again with one click.

**Why this priority**: A successful upload is the core of Ballast. A summary feature that risks the upload or the captions is a net loss, so this protection is a precondition for shipping Story 1.

**Independent Test**: Make summarising fail (e.g. a damaged model file) and upload a video. Confirm the video and captions complete exactly as with summaries off, and the transfer shows a specific reason with "Try again". Fix the cause, choose Try again, and confirm the summary is produced without redoing captions.

**Acceptance Scenarios**:

1. **Given** summarising fails for any reason (model missing or damaged, not enough memory or disk, the model's answer unusable), **When** the upload and captions complete, **Then** both are recorded as succeeded, and the summary shows as failed with a plain-language reason.
2. **Given** a summary failed, **When** the user chooses Try again, **Then** it is attempted again from the existing transcript, without re-uploading the video or redoing the captions.
3. **Given** the model's answer cannot be used (for example, it is not in the expected shape), **When** summarising is attempted, **Then** it is retried automatically a limited number of times before being reported as failed.
4. **Given** the user cancels a video upload, **When** the cancellation takes effect, **Then** no summary is uploaded to Drive for it.
5. **Given** captions could not be made for a video, or found no speech, **When** captioning ends, **Then** no summary is attempted, and the transfer says why ("No speech to summarise").

---

### User Story 3 - Turn summaries on or off (Priority: P2)

Summaries are on by default, like captions, because they cost nothing and keep everything on the Mac. A user who doesn't want them, for example to save time or battery on a laptop, turns them off in Settings. The first time a summary is needed, Ballast asks once before downloading the summary model.

**Why this priority**: Users need control over the extra processing and download, but the feature delivers value with a sensible default before the switch exists.

**Independent Test**: Turn summaries off, upload a video, and confirm no summary is attempted and no summary model runs. Turn them back on and confirm the next video gets a summary. Decline the download prompt and confirm summaries switch off and the video and captions are unaffected.

**Acceptance Scenarios**:

1. **Given** summaries are off, **When** a video's captions are made, **Then** no summary is made and no summary status is shown.
2. **Given** the download prompt is shown, **When** the user declines, **Then** that video gets no summary, summaries are switched off, and the user can turn them back on in Settings (which starts the download).
3. **Given** the user changes the setting, **When** they quit and reopen Ballast, **Then** the setting is remembered.

---

### User Story 4 - Use the summary before the upload finishes (Priority: P2)

A long video can take hours to upload. As soon as its summary is ready, a copy is saved on the user's Mac next to the original video, the same way Feature 005 saves the caption file, so the user can share the message right away. The Drive copy follows once the video has arrived.

**Why this priority**: Useful, and it mirrors the caption file's behaviour, but the summary in Drive (Story 1) is the main deliverable.

**Independent Test**: Upload a large video and confirm the summary appears next to the local video before the upload completes, and in Drive only after the video arrives.

**Acceptance Scenarios**:

1. **Given** a video is still uploading, **When** its summary is ready, **Then** a copy is saved immediately next to the original video on the Mac.
2. **Given** the video upload is later cancelled, **When** the cancellation takes effect, **Then** the local copy stays on the Mac and nothing is put in Drive.

---

### Edge Cases

- **Announcements, thanks, and housekeeping** (e.g. church notices, meeting logistics): left out of every part of the summary (main content only).
- **Videos that mix English with other languages (e.g. Twi/Ga)**: the summary is written in English. Passages whose transcript is unclear are summarised only as far as the transcript supports; nothing is invented to fill gaps.
- **Videos with little speech** (mostly music, a short clip, or a few sentences): the summary is kept short and in proportion. Sections with nothing to say are left out rather than padded, and the overview says plainly that the video has little spoken content.
- **Very long videos (3+ hours, ~30,000+ words)**: summarised in parts that are then combined, so the whole video is covered within the model's limits on a Mac.
- **Long stretches of music or silence**: not treated as content; the summary focuses on what is said.
- **No references or calls to action in a video**: those sections are left out entirely; none are invented.
- **The summary model download fails, is interrupted, or there's no internet the first time**: the summary waits and retries the download later (resuming where possible), or fails with a clear reason. The video and captions are unaffected, and a damaged or partial model is never used.
- **Not enough free memory or disk**: summarising waits until the speech model has finished (only one model runs at a time) and fails with a clear reason if there still isn't enough room. It never slows the upload.
- **The app quits or the computer restarts mid-summary**: the summary is started again automatically on the next launch, from the saved transcript.
- **A summary with the same name already exists in the Drive folder or next to the local video**: the existing one is not overwritten; the new one gets a distinguishing suffix, the same rule as caption files in Feature 005.
- **Summaries on but captions off**: no transcript exists, so no summary can be made. Settings explains that summaries need captions turned on.
- **Platforms without captions (Feature 005: Intel Macs, Windows, Linux)**: summaries are unavailable for the same reason, and Settings says so.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST produce a summary automatically for every video whose captions Feature 005 completes with speech, whatever the video's subject, when summaries are on. The user does nothing beyond starting the upload, plus answering the one-time download prompt (FR-013).
- **FR-002**: Every summary MUST contain six core parts: an overview paragraph, the main points, key quotes each with its time in the video, a suggested title, a suggested description suitable for YouTube or social media, and a ready-to-share message. It MUST cover the main content only: announcements, thanks, notices, and other housekeeping are left out.
- **FR-002b**: A summary MUST also include a references section (for example Bible verses, books, studies, or other named sources) when the video mentions any, and a "Key takeaways / actions" section when the speaker asks the audience to do something. Each of these sections MUST be omitted entirely when the video has nothing for it, never shown empty or padded.
- **FR-002a**: The ready-to-share message MUST be a shorter, self-contained text written for the people who will receive it: plain text using WhatsApp bold (`*like this*`) for headings, no emojis, no timestamps, no notes about the transcript or its accuracy, and no housekeeping. It MUST be ready to paste without editing.
- **FR-003**: Quotes MAY be lightly tidied into clear English (fixing transcription errors and grammar) but MUST keep the speaker's meaning, MUST each trace back to an identifiable passage of the transcript, and MUST carry the time that passage was said. The summary MUST NOT present invented quotes or references.
- **FR-003a**: Where a reference was unclear in the recording, the summary MUST use the most likely correct reference and MUST list each such guess, with what was actually heard, in a separate "Check before sharing" note that is not part of the ready-to-share message.
- **FR-004**: The summary MUST cover the whole video, however long, not only its opening portion.
- **FR-005**: Summarising MUST run entirely on the user's computer. No transcript, audio, or video may be sent to any outside service, and it MUST work without an account, API key, payment, or usage limit. The only network use is the one-time model download and uploading the finished summary to the user's own Drive.
- **FR-006**: The summary MUST be placed in the same Drive folder as its video, named after the video, only after the video has been confirmed in Drive; it MUST NOT overwrite an existing file.
- **FR-007**: As soon as a summary is ready, the system MUST save a copy on the user's computer next to the original video, without waiting for the video upload, following the same placement and naming rules as Feature 005's local caption copy.
- **FR-008**: Summarising MUST NOT slow, block, or fail the video upload or the captions; their behaviour MUST be identical whether summaries are on or off. The summary model and the speech model MUST NOT run at the same time.
- **FR-009**: If summarising fails, the system MUST show a plain-language reason and offer "Try again", which reuses the existing transcript.
- **FR-010**: An unusable answer from the model MUST be retried automatically a limited number of times before the summary is reported as failed.
- **FR-011**: The system MUST show each video's summary status (waiting for captions, downloading model, writing, ready, could not be made) separately from its upload and caption status.
- **FR-012**: Summaries MUST be on by default. Users MUST be able to turn them on or off in Settings; the setting MUST persist across restarts and apply to videos whose captions finish after the change.
- **FR-013**: The summary model MUST NOT be part of the app download. It MUST be downloaded once, the first time a summary is needed, after the user agrees to a one-time prompt stating its size. Declining switches summaries off without affecting the video or captions; turning summaries back on starts the download. A downloaded model MUST be verified before use and MUST NOT be downloaded again unless it is missing or damaged.
- **FR-014**: The system MUST tell the user, in Settings, that summaries are made on this computer for free, that nothing but the finished summary leaves the computer, and that summaries of long or unclear recordings may be less detailed.
- **FR-016**: Cancelling a video upload MUST prevent its summary from being uploaded to Drive; a local copy already saved stays on the user's computer.
- **FR-017**: Interrupted summaries MUST be restarted automatically from the saved transcript on the next launch.
- **FR-018**: The user MUST be able to open a finished summary in Drive, or show its local copy, from the transfer's details.
- **FR-019**: The system MUST be designed so that a different summary engine (for example, an optional paid cloud model for higher quality) can be added later without changing how summaries look or behave. This version MUST ship with only the free on-computer engine.

### Key Entities *(include if feature involves data)*

- **Summary job**: The work of summarising one video. Linked to exactly one upload and its caption job. Has a status (waiting for captions, in progress, ready, failed, cancelled), a failure reason when failed, the number of attempts made, the engine and model used, and, once ready, references to the Drive copy and the local copy. Survives app restarts.
- **Summary**: The produced document: its six core parts (FR-002), the optional references and takeaways sections when the video has them (FR-002b), and any "Check before sharing" note (FR-003a). Named after its video, stored in the video's Drive folder and next to the original video on the user's computer.
- **Summary settings**: On/off (default on) and whether the user has agreed to the model download, remembered across restarts.
- **Summary model**: The free AI model file, downloaded once, verified, and shared by all summary jobs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With summaries on, 100% of videos whose captions complete with speech get a summary in their Drive folder, across subjects (at least: a sermon, a lecture or talk, and a documentary-style video), including videos of 3 hours or more and those Drive's own summary cannot handle.
- **SC-002**: In 100% of tested summaries, every quote can be matched by a reviewer to a transcript passage with the same meaning within 30 seconds of its stated time. Every reference listed is either clearly said in the transcript or appears in the "Check before sharing" note.
- **SC-003**: For a 3-hour video, the main points include at least one point from each hour.
- **SC-004**: On a recent Apple-silicon Mac, a summary is ready within 10 minutes of captions finishing for a video of up to 1 hour, and within 30 minutes for up to 3 hours, excluding the one-time model download.
- **SC-005**: In 100% of tested summary failures, the video upload and captions still succeed and the user sees a specific reason.
- **SC-006**: Zero transcript, audio, or video bytes are sent to any outside service during summarising, confirmed by observing network traffic; the cost to the user is zero.
- **SC-007**: The summary model and the speech model are never running at the same time, confirmed by observing processes during back-to-back uploads.
- **SC-008**: Turning summaries on or off takes one switch, and the first summary needs only one confirmation (the download prompt).
- **SC-009**: In 100% of tested summaries, the ready-to-share message contains no emojis, timestamps, transcript notes, or housekeeping, and a reviewer judges it ready to send without edits.
- **SC-010**: In 100% of tested non-religious videos, the summary contains no Bible section and no invented references. In 100% of tested sermons, every clearly spoken Bible reference appears in the references section or the "Check before sharing" note.

## Assumptions

- **Engine**: A free, open-source AI model that runs offline on the Mac does the summarising, run the same way Feature 005 runs its speech engine. Which model is chosen during planning by testing a few candidates on real videos of different kinds (a sermon and a non-religious video) against reference summaries. It must fit an 8 GB Apple-silicon Mac once the speech model has finished.
- **Quality**: A model small enough to run on a Mac will write plainer summaries than a large cloud model, and may lose detail on very long or unclear recordings. The quote check (FR-003) and the "Check before sharing" note (FR-003a) guard against invented content; the user accepted this trade-off for zero cost.
- **One format for all videos**: The same summary layout is used for every video; sections that don't apply are left out (FR-002b). There is no per-video "type" setting, and the user never has to say what kind of video it is.
- **Summary format**: In Drive, the summary is a Google Doc, so it can be read, shared, and edited directly in Drive. The local copy is a plain-text Markdown file with the same content.
- **Language**: Summaries are always written in English, whatever languages the video mixes.
- **Quote timings**: Quote times come from the caption file's timings and are shown as hours:minutes:seconds, which viewers can use to jump to that point in Drive's player.
- **Order of work**: A summary starts once the transcript is ready and the speech model has finished, which may be while the video is still uploading. Its local copy is saved immediately; its Drive copy waits for the video, as with caption files.
- **Platforms**: Summaries are available wherever Feature 005's captions are (macOS on Apple silicon), since they need its transcript.
- **Scope**: Only videos captioned after summaries are on are summarised. Summarising older videos already in Drive, translating summaries, chat about a video, editing summaries inside Ballast, and any paid cloud engine are out of scope for this version.
- **Retries**: Automatic retries (FR-010) are limited to a small number so a model that keeps producing unusable answers doesn't keep the Mac busy.
