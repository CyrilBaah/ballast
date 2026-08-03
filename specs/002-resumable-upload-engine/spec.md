# Feature Specification: Resumable, Crash-Safe Upload Engine

**Feature Branch**: `002-resumable-upload-engine`

**Created**: 2026-08-03

**Status**: Draft

**Input**: User description: "Resumable, crash-safe upload engine: replace Feature 001's basic upload with Google Drive's resumable session protocol, persisting session URI, byte offset, and file identity so interrupted uploads resume automatically without re-sending acknowledged bytes, survive process crash/power loss/OS restart, and classify retryable vs terminal errors."

## Clarifications

### Session 2026-08-03

- Q: When a paused upload can't just resume as-is — the Drive session expired, or the local source file changed — should the app restart the transfer from byte 0 automatically, or wait for the user to confirm first? → A: Ask first. The app shows why it can't resume and waits for confirmation before re-sending data; a surprise re-upload of a large file has a real time/bandwidth cost the user should approve.
- Q: If an upload is paused or waiting for confirmation and the user no longer wants to continue it, should they be able to explicitly cancel/discard it, rather than only being able to confirm-restart it or leave it stuck? → A: Yes — add an explicit cancel/discard action, so the single-active-upload rule can never permanently block starting a new upload just because the user doesn't want to continue an interrupted one.
- Q: If the Google Drive destination folder is deleted or becomes inaccessible while an upload is paused, should this be a terminal failure requiring a new destination, or use the same confirm-and-restart flow as an expired session? → A: Terminal, not recoverable — restarting the same session can't fix a destination that no longer exists, so the user must start a new upload with a new destination.
- Q: How quickly, at most, must a terminal failure's specific reason be shown to the user after the failure is detected? → A: Within 5 seconds, matching this project's existing progress-reporting cadence precedent (Feature 001's SC-004).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Upload survives a dropped connection (Priority: P1)

A user starts uploading a file. Partway through, their internet connection
drops — the upload pauses instead of failing, and picks back up on its own
from where it left off as soon as the connection returns, without
re-sending any data that was already received.

**Why this priority**: This is the entire reason this feature exists —
Feature 001 shipped a basic upload that fails outright on any interruption.
Turning "fails on interruption" into "pauses and resumes" is the core
reliability upgrade and the smallest slice that delivers real value on its
own.

**Independent Test**: Can be fully tested by starting an upload, simulating
a network drop mid-transfer, restoring connectivity, and confirming the
upload completes successfully with the total bytes sent over the wire
matching only the unsent remainder (not the whole file again).

**Acceptance Scenarios**:

1. **Given** an upload is in progress, **When** the network connection
   drops, **Then** the app shows the upload as paused (not failed) and
   does not lose the progress already made.
2. **Given** an upload is paused due to a dropped connection, **When**
   connectivity returns, **Then** the upload automatically resumes from
   the last acknowledged byte and no already-sent data is re-transmitted.
3. **Given** an upload resumes after a drop, **When** it finishes, **Then**
   the completed file in Google Drive is byte-identical to the local
   source file.

---

### User Story 2 - Upload survives an app crash, power loss, or restart (Priority: P2)

A user has an upload in progress when the app is killed, the machine loses
power, or the OS restarts. When they relaunch the app, the upload is still
there and continues from where it stopped — it was never silently lost.

**Why this priority**: Network drops are the common case, but the
project's stated reliability bar explicitly includes surviving crashes and
power loss, not just connectivity blips. This depends on User Story 1's
resumable mechanism already existing, so it follows it.

**Independent Test**: Can be fully tested by starting an upload, force-
killing the app process (or simulating a restart) mid-transfer, relaunching
the app, and confirming the upload is shown as recoverable and completes
successfully from its last saved position.

**Acceptance Scenarios**:

1. **Given** an upload is in progress, **When** the app process is
   killed or the machine loses power before it finishes, **Then** the
   upload's progress up to the last acknowledged byte is not lost.
2. **Given** the app is relaunched after an unexpected shutdown during an
   upload, **When** it starts up, **Then** it detects the interrupted
   upload and shows the user it can be resumed.
3. **Given** a recoverable upload is resumed after relaunch, **When** it
   completes, **Then** it produces the same correct, complete file as an
   uninterrupted upload.

---

### User Story 3 - Clear handling when an upload truly cannot continue (Priority: P3)

When an upload runs into a problem it can't recover from on its own — the
account is out of storage, permission was revoked, the resumable session
expired from being idle too long, or the local file changed since it was
paused — the user is told clearly what happened and what to do next,
instead of the app retrying forever or resuming onto corrupted data.

**Why this priority**: This is what makes the automatic retry/resume
behavior from Stories 1 and 2 trustworthy rather than a black box — it
depends on both existing first, since it's the "when the happy path
doesn't apply" complement to them.

**Independent Test**: Can be fully tested by inducing each terminal
condition independently (e.g., simulate a quota-exceeded response, an
expired session, or modify the source file while an upload is paused) and
confirming the app stops retrying, surfaces a clear reason, and offers an
explicit next step rather than looping silently or resuming with wrong
data.

**Acceptance Scenarios**:

1. **Given** an upload hits a rate-limit or server error, **When** the
   engine retries it with backoff, **Then** the user sees it as "still in
   progress," not as a failure, unless retries are exhausted.
2. **Given** an upload hits a condition that cannot be retried (storage
   quota exceeded, permission revoked, expired session, or a changed
   source file), **When** that condition is detected, **Then** the app
   stops automatically retrying and shows the user a specific, actionable
   reason.
3. **Given** an upload's Drive session has expired or its source file
   changed while paused, **When** the app detects this, **Then** it asks
   the user to confirm before restarting the transfer from the beginning,
   rather than resuming onto stale or corrupted data or restarting
   silently.
4. **Given** an upload is paused or awaiting confirmation, **When** the
   user chooses to cancel it instead of resuming or confirming a restart,
   **Then** the upload ends immediately without restarting from byte 0,
   and the app is free to start a new upload.

---

### Edge Cases

- What happens if the network drops and returns multiple times in quick
  succession during one upload? The upload keeps resuming from its last
  acknowledged byte each time, without manual intervention, as long as the
  Drive session hasn't expired.
- What happens if the user closes the app manually (not a crash) while an
  upload is in progress? Same recovery path as an unexpected shutdown —
  the upload resumes from its last saved position on next launch.
- What happens if the source file is deleted from disk while its upload is
  paused? The app cannot resume it; the user is shown a clear terminal
  error rather than the upload silently disappearing or looping.
- What happens if the file to upload is very large (hundreds of GB)? The
  engine must track progress and resume without reading the whole file
  into memory at once, and must avoid re-hashing the entire file on every
  resume attempt (see FR-009).
- What happens if the user's Google sign-in session (from Feature 001)
  expires or is revoked mid-upload? The upload pauses with a clear
  "signed out" reason rather than retrying indefinitely; it resumes once
  the user signs in again.
- What happens if the Google Drive destination folder is deleted or
  otherwise becomes inaccessible while an upload is paused? This is a
  terminal, non-recoverable failure — restarting the same session can't
  fix a destination that no longer exists — so the user is shown a clear
  failure and must start a new upload with a new destination, rather than
  being offered a restart-from-byte-0 confirmation.
- What happens if the user doesn't want to continue a paused or
  awaiting-confirmation upload? They can explicitly cancel/discard it,
  which ends it immediately without requiring a restart-from-byte-0
  confirmation, and frees the app to start a new upload.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST upload files to Google Drive using a
  resumable transfer session rather than sending the whole file in one
  request, so that partially-sent data isn't lost on interruption.
- **FR-002**: The system MUST track how many bytes of a file have been
  successfully received by Drive at any point during an upload.
- **FR-003**: When an upload is interrupted by a lost connection, the
  system MUST pause it (not mark it failed) and automatically resume it
  from the last acknowledged byte once connectivity returns, without
  re-sending already-acknowledged data.
- **FR-004**: The system MUST persist enough state about an in-progress
  upload (its Drive session, byte offset, and source file identity) that
  the upload can be recovered after the app process ends unexpectedly,
  including from a crash, power loss, or OS restart.
- **FR-005**: On app startup, the system MUST detect any upload left
  in-progress from a previous run and present it to the user as
  resumable, rather than losing or silently discarding it.
- **FR-006**: The system MUST distinguish retryable problems (e.g.,
  rate-limiting, temporary server errors, temporary network loss) from
  terminal problems (e.g., storage quota exceeded, permission revoked,
  expired resumable session, a destination folder that no longer exists)
  and MUST NOT retry terminal problems indefinitely.
- **FR-007**: For retryable problems, the system MUST retry automatically
  with increasing wait times between attempts, and MUST keep the upload
  visibly in an "in progress" state to the user rather than showing it as
  failed while retries are ongoing.
- **FR-008**: For terminal problems, the system MUST stop retrying and
  present the user with a specific, understandable reason and a clear
  next step (e.g., sign in again, free up storage, restart the transfer).
- **FR-009**: Before resuming a paused upload, the system MUST verify the
  local source file has not changed since it was paused, using a cheap
  check (size and modification time) before falling back to a full
  content check, to avoid corrupting the uploaded file with mismatched
  data.
- **FR-010**: If the source file changed since the upload was paused, or
  if the Drive resumable session has expired, the system MUST ask the
  user to confirm before restarting the transfer from the beginning,
  rather than resuming onto invalid data or restarting without asking.
- **FR-011**: The system MUST NOT lose or duplicate data in the
  destination file as a result of a resume — a resumed upload MUST
  produce a file identical to what an uninterrupted upload of the same
  source file would produce.
- **FR-012**: The system MUST handle files ranging from small documents to
  hundreds of gigabytes without reading an entire file into memory at
  once.
- **FR-013**: The system MUST continue to support exactly one active
  upload at a time (one local file to one Drive destination), matching
  Feature 001's scope — uploading multiple files concurrently is out of
  scope for this feature.
- **FR-014**: The system MUST let the user explicitly cancel a paused or
  awaiting-confirmation upload, ending it immediately without requiring a
  restart-from-byte-0 confirmation, and freeing the app to start a new
  upload (FR-013).

### Key Entities

- **Upload Session**: Represents one in-progress or paused transfer of a
  single local file to a single Drive destination. Tracks the Drive-issued
  session identifier, the last acknowledged byte offset, the current
  status (in progress, paused, awaiting confirmation, cancelled, failed,
  complete), and timestamps for when it was created and last updated.
- **Source File Identity**: The characteristics used to confirm a local
  file hasn't changed since its upload was paused — file path, size,
  modification time, and (when a full check is needed) a content hash.
- **Error Classification**: The categorization of a failure encountered
  during upload as either retryable (temporary — retry automatically) or
  terminal (permanent — stop and inform the user), along with the
  human-readable reason shown to the user for terminal cases.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An interrupted upload resumes within 2 seconds of
  connectivity returning, with zero re-transmission of data already
  acknowledged by Drive.
- **SC-002**: An upload in progress when the app crashes, the device loses
  power, or the OS restarts is fully recoverable the next time the app is
  opened, with no loss of previously-uploaded progress.
- **SC-003**: Uploads complete successfully over degraded network
  conditions (intermittent drops, high latency, packet loss) without
  requiring the user to manually restart the transfer, as long as the
  underlying Drive session remains valid.
- **SC-004**: 100% of terminal (non-retryable) failures show the user a
  specific, actionable reason within 5 seconds of the failure being
  detected — none surface as a generic or silent failure.
- **SC-005**: A resumed upload's completed file matches the source file
  exactly, with zero cases of missing, duplicated, or corrupted bytes,
  across repeated interrupt-and-resume test cycles.
- **SC-006**: Files of at least several hundred gigabytes can be uploaded
  and resumed without the app's memory usage growing proportionally to
  file size.

## Assumptions

- Feature 001 (Google sign-in, file/folder picker, basic upload) is
  already in place; this feature replaces its non-resumable transfer
  mechanism with a resumable one, and reuses its auth, file selection, and
  destination-selection flows unchanged.
- Exactly one upload is active at a time, consistent with Feature 001 —
  concurrent multi-file uploads, adaptive chunk-size tuning, and
  network-quality-driven throughput optimization are separate future
  features, not part of this slice.
- "Automatically resume" (User Stories 1 and 2) means without requiring
  the user to re-select the file or destination, and without a
  confirmation prompt — it applies only to resuming an in-progress
  transfer, not to restarting one from byte 0. Per the Clarifications
  above, restarting from byte 0 (session expiry or file change) always
  asks for confirmation first.
- The Drive resumable session's 7-day idle expiry window is an external
  Google Drive constraint, not a design choice, and is treated as a
  terminal condition for the current session (a new session must be
  created to continue, per FR-010).
- Upload state is persisted locally in a way that survives process
  restarts; the specific storage mechanism is an implementation decision
  for `/speckit-plan`, not this spec.
