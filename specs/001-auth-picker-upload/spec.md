# Feature Specification: Google Sign-In, File/Folder Picker & Basic Upload

**Feature Branch**: `001-auth-picker-upload`

**Created**: 2026-08-02

**Status**: Draft

**Input**: User description: "Feature 1 of the Ballast project: Google sign-in, local file/folder picker, and a basic (non-resumable) upload to Google Drive. This is the smallest end-to-end vertical slice — it gets a real working desktop app (auth → pick a local file → pick a Drive destination folder → upload it → confirm success) before the resumable/adaptive upload engine (a later feature) is built underneath it."

## Clarifications

### Session 2026-08-02

- Q: Does "select a file" mean the user can only pick a single local file, or can they also pick a whole local folder (with the app uploading its contents) as the upload source? → A: File only — user selects exactly one local file (FR-004); "folder" in the feature title refers to the Drive destination folder browser (FR-005), not a local source.
- Q: When a signed-in user explicitly signs out (FR-003), should the app revoke the Google OAuth grant at Google's end, or just clear the app's local session? → A: Revoke at Google — sign-out revokes the OAuth grant server-side in addition to clearing local state; a future sign-in requires fresh Google consent.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sign in with Google (Priority: P1)

A user opens the app for the first time and connects it to their Google
account so the app can act on their behalf with Google Drive.

**Why this priority**: Nothing else in the app is reachable or meaningful
without an authenticated Google identity — this is the foundation every other
capability depends on.

**Independent Test**: Can be fully tested by launching the app, completing
Google sign-in, and confirming the app shows an authenticated state that
persists across an app restart — delivers the value of "the app is connected
to my Google account" on its own, independent of any upload happening.

**Acceptance Scenarios**:

1. **Given** the user has never signed in, **When** they choose to sign in
   with Google and complete the Google consent flow successfully, **Then**
   the app shows them as signed in and stores the connection for future
   launches.
2. **Given** the user is already signed in, **When** they relaunch the app,
   **Then** they are still signed in and are not asked to sign in again.
3. **Given** the user is signed in, **When** they choose to sign out,
   **Then** the app returns to a signed-out state and no longer allows file
   selection, folder browsing, or uploads until they sign in again.

---

### User Story 2 - Select a file and destination, then upload (Priority: P2)

A signed-in user picks a file from their computer, chooses a folder in their
Google Drive to put it in, and sends it.

**Why this priority**: This is the core value of the app — getting a file
from the user's machine into their Drive. It's the reason the app exists,
but it cannot happen without Story 1's authenticated session.

**Independent Test**: Can be fully tested by, while signed in, selecting a
local file, browsing to and selecting a Drive folder, starting the upload,
and confirming the file appears in that folder in Google Drive — delivers
the app's core value independent of how progress/confirmation is displayed
(Story 3).

**Acceptance Scenarios**:

1. **Given** a signed-in user, **When** they select a local file and a Drive
   destination folder and start the upload, **Then** the app transfers the
   file to that folder in their Drive.
2. **Given** a signed-in user with no existing folders beyond the top level
   of their Drive, **When** they browse for a destination, **Then** the top
   level ("My Drive") is available as a valid destination.
3. **Given** a user has selected a local file, **When** that file is moved,
   renamed, or deleted before the upload starts, **Then** the app tells the
   user the file can no longer be found and asks them to choose again.
4. **Given** an upload is in progress, **When** the network connection is
   lost and does not return before the transfer can complete, **Then** the
   upload is marked as failed and the user is told it failed (automatic
   resume is out of scope for this feature; the user retries manually).

---

### User Story 3 - See upload progress and confirmation (Priority: P3)

While and after an upload runs, the user can see that it's happening and
knows for certain whether it succeeded or failed.

**Why this priority**: Builds on Story 2 — the upload can function without
this, but a user staring at a frozen-looking app during a multi-minute
transfer, or left unsure whether a file actually landed in Drive, is not an
acceptable experience even for a first version.

**Independent Test**: Can be fully tested by starting an upload and
observing that progress is visibly updating during the transfer, and that a
clear success (with a reference to the uploaded file) or failure message
appears at the end — independently verifiable without needing to inspect the
upload-initiation flow itself.

**Acceptance Scenarios**:

1. **Given** an upload is in progress, **When** the user looks at the app,
   **Then** they see a progress indicator that is actively updating, not
   stalled or ambiguous.
2. **Given** an upload finishes successfully, **When** the app reports
   completion, **Then** the user sees a clear success confirmation including
   a reference/link to the file's location in Drive.
3. **Given** an upload fails for any reason, **When** the app reports the
   outcome, **Then** the user sees a clear failure message — never an
   indefinite spinner or silent nothing-happened state.

---

### Edge Cases

- What happens when the user denies or cancels the Google permission prompt
  mid-sign-in? The app returns to the signed-out state with a clear message;
  no partial session is created.
- What happens when the user's Google authentication expires or is revoked
  while the app is open (including mid-upload)? The user is prompted to
  re-authenticate; any upload in progress at that moment is treated as
  failed per Story 2's connectivity-loss behavior.
- What happens if two files with the same name are uploaded to the same
  destination folder? Google Drive permits duplicate names; this feature
  does not detect, warn about, or rename duplicates.
- What happens if the user tries to access file selection, folder browsing,
  or upload while signed out? These are unavailable until sign-in completes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST require the user to authenticate with their
  Google account before file selection, folder browsing, or upload
  capabilities become available.
- **FR-002**: System MUST persist the authenticated session across app
  restarts so the user does not need to sign in on every launch.
- **FR-003**: System MUST allow the user to explicitly sign out, returning
  the app to a signed-out state. Signing out MUST revoke the Google OAuth
  grant server-side (in addition to clearing the local session), so the app
  no longer appears as a connected app in the user's Google Account and a
  future sign-in requires fresh Google consent.
- **FR-004**: System MUST allow the signed-in user to select exactly one
  file from their local device as the upload source.
- **FR-005**: System MUST allow the signed-in user to browse their Google
  Drive folder structure and select a destination folder for the upload,
  including the top level of their Drive.
- **FR-006**: System MUST transfer the selected local file to the selected
  Drive destination folder when the user starts the upload.
- **FR-007**: System MUST display upload progress to the user for the
  duration of an active upload.
- **FR-008**: System MUST notify the user clearly and unambiguously when an
  upload succeeds, including a reference to the uploaded file's location in
  Drive.
- **FR-009**: System MUST notify the user clearly and unambiguously when an
  upload fails, and MUST NOT leave the user in an indefinite or ambiguous
  waiting state.
- **FR-010**: System MUST NOT automatically retry or resume a failed upload
  within this feature — the user retries manually. (Automatic resume is
  explicitly deferred to a later feature.)
- **FR-011**: System MUST detect and inform the user if the selected local
  file is no longer available (moved, renamed, or deleted) before the
  upload starts.

### Key Entities

- **User Account**: The authenticated Google identity connected to the app
  for the session. One account per installation for this feature.
- **Local File**: The file selected from the user's device as the upload
  source (identified by its location, name, and size on the local device).
- **Drive Destination**: The Google Drive folder selected as the upload
  target, including the top level of the user's Drive.
- **Upload**: A single transfer linking one Local File to one Drive
  Destination, with a status of pending, in progress, succeeded, or failed,
  and — on success — a reference to the resulting file in Drive.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A first-time user can go from opening the app to being signed
  in with their Google account in under 30 seconds of active interaction.
- **SC-002**: A signed-in user can select a local file, choose a Drive
  destination, and start an upload in under 1 minute of active interaction.
- **SC-003**: 100% of uploads the app reports as successful are verifiably
  present in the user's Google Drive at the selected destination
  immediately afterward.
- **SC-004**: Upload progress visibly updates at least every 5 seconds
  during an active upload.
- **SC-005**: A returning user is not asked to re-authenticate on at least
  95% of app launches occurring within their session's valid lifetime.
- **SC-006**: 100% of failed uploads result in a clear failure notification
  to the user — zero silent failures or indefinite waiting states.

## Assumptions

- Single Google account per installation for this feature; multi-account
  support is explicitly out of scope (open question for a later feature).
- "File/Folder Picker" in the feature title refers to browsing and picking a
  Drive *destination* folder (FR-005); the local upload source is a single
  file only (FR-004). Local folder selection/upload is out of scope for this
  feature.
- The user already has a Google account with Google Drive access; this
  feature does not cover Google account creation.
- No artificial file-size limit is imposed by this feature. Reliability of
  very large uploads over unstable connections is explicitly deferred to the
  resumable/adaptive upload feature — this feature only needs to move a file
  successfully under normal connectivity.
- If an upload fails partway (e.g., due to lost connectivity), the user must
  restart it manually; automatic resume/retry is out of scope here and is a
  later feature.
- Whether the Drive destination browser is purpose-built for this app or
  uses an embedded Google-provided picker is an implementation decision for
  the planning phase, not a business requirement of this spec — both satisfy
  FR-005 equally from the user's perspective.
- Resumable session persistence, adaptive chunk sizing, cross-file
  concurrency, network-quality monitoring, and multi-account support are
  explicitly out of scope for this feature and are covered by later features
  per the project's constitution.
