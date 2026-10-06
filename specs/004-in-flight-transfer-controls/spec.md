# Feature Specification: In-Flight Transfer Controls

**Feature Branch**: `004-in-flight-transfer-controls`

**Created**: 2026-08-17

**Status**: Draft

**Input**: User description: "The ones on the in-flight side: give a button where I can let a transfer continue, stop it, or remove it from the list — right now selecting an in-flight transfer only shows its details, with no way to act on it. I should be able to pause an in-flight transfer, resume it later, or remove it entirely so I know it's no longer in flight."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Pause and resume an in-flight transfer (Priority: P1)

A user has a large file actively uploading and needs to temporarily hold it — for example, to free up bandwidth for something else on a poor connection — without losing the progress already made. They pause it from the in-flight list, do what they need to do, then come back and resume it, and it picks up from where it left off rather than starting over.

**Why this priority**: This is the core gap the user is reporting: today an in-flight item shows only read-only details with no action available. Pause/resume is the primary control they're missing, and it's the capability the other actions build on.

**Independent Test**: Start a transfer, pause it while it's actively uploading, confirm its progress (bytes already confirmed by Drive) is retained and it stops advancing, then resume it and confirm it continues from the same point without re-sending already-confirmed bytes.

**Acceptance Scenarios**:

1. **Given** a transfer that is actively uploading, **When** the user selects Pause on it (from either the Home in-flight list or the Transfers page), **Then** the transfer stops sending further data, its already-confirmed bytes are preserved, and it is clearly labeled as paused by the user (not as an error or a connectivity dropout).
2. **Given** a transfer the user has paused, **When** the user selects Continue (Resume), **Then** the transfer resumes sending data starting from the last confirmed byte, without restarting from the beginning.
3. **Given** a transfer that is queued (not yet actively sending bytes) but not started, **When** the user selects Pause, **Then** the transfer is held out of the queue and does not begin sending until the user resumes it.
4. **Given** a transfer the user paused, **When** the user quits and reopens Ballast, **Then** the transfer is still shown as paused and can be resumed, rather than being lost or silently restarted.

---

### User Story 2 - Remove an in-flight transfer (Priority: P2)

A user decides they no longer want a given transfer to run at all — whether it's actively uploading, paused, or holding position after a dropped connection — and wants it off their in-flight list entirely, with a clear record that it did not complete.

**Why this priority**: This gives users a definitive way to stop and clear a transfer they no longer want, distinct from and complementary to pause/resume. It's slightly lower priority than pause/resume because a Cancel-like action already exists on some transfer states today; this extends it consistently to every in-flight state and surface.

**Independent Test**: Start a transfer (or pause one), select Remove, and confirm it stops immediately, disappears from the in-flight list, and appears in transfer history as not completed.

**Acceptance Scenarios**:

1. **Given** a transfer that is actively uploading, paused, or holding position after a connectivity dropout, **When** the user selects Remove, **Then** the transfer stops immediately, is taken out of the in-flight list, and is recorded in history as not completed.
2. **Given** a transfer the user has just removed, **When** the user reopens Ballast later, **Then** it is not offered again as a transfer to pick back up.

---

### User Story 3 - Consistent controls everywhere in-flight transfers appear (Priority: P2)

A user glances at the Home dashboard's in-flight summary and expects to be able to act on a transfer right there, not just on the dedicated Transfers page.

**Why this priority**: Without this, users would have to know to navigate to a different screen to act on a transfer they can already see, which recreates the "I can't do anything" complaint on one of the two screens.

**Independent Test**: From the Home dashboard's in-flight list, pause, resume, and remove a transfer without navigating to the Transfers page, and confirm the same actions and resulting states are available there as on the Transfers page.

**Acceptance Scenarios**:

1. **Given** the Home dashboard showing in-flight transfers, **When** the user opens the actions for one of them, **Then** Pause/Continue and Remove are available directly, matching what's available on the Transfers page.
2. **Given** an action taken on a transfer from the Home dashboard, **When** the user navigates to the Transfers page, **Then** the transfer reflects the same up-to-date state (paused, resumed, or removed).

---

### Edge Cases

- What happens if the user pauses the one transfer that is actively sending bytes while another transfer is waiting in the queue? The queued transfer should be free to start in its place, the same way it does today when a transfer is cancelled.
- What happens if the user hits Remove on a transfer at the exact moment it finishes and Drive confirms the last byte? The completed outcome should win — the transfer should end up recorded as completed, not removed.
- What happens if the user tries to resume a paused transfer whose local file has since been moved, renamed, or deleted? The user should get a clear message that the file can no longer be found, the same way this is already handled for other transfer failures.
- What happens if the user pauses a transfer that is already holding position due to a connectivity dropout (auto-pause)? The action should still take effect (the user's pause now governs it) and it should read as user-paused, not as still-reconnecting.
- What happens to a paused transfer if the user signs out of their Google account? It should remain paused and pick up again once the user is signed back in and resumes it, provided the file and destination are still valid.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Users MUST be able to pause a transfer that is actively uploading or queued, directly from the in-flight list, without losing bytes already confirmed by Google Drive.
- **FR-002**: Users MUST be able to resume ("Continue") a transfer they previously paused, and the resumed transfer MUST continue from the last confirmed byte rather than restarting from the beginning.
- **FR-003**: Users MUST be able to remove a transfer that is actively uploading, queued, paused, or holding position after a connectivity dropout. Removing MUST stop the transfer immediately and record it in history as not completed.
- **FR-004**: A removed transfer MUST NOT be offered again for pickup or recovery on a later app launch.
- **FR-005**: Pause, Continue (Resume), and Remove MUST be available for every in-flight transfer both on the Home dashboard's in-flight list and on the Transfers page (list and detail views), with consistent results across both.
- **FR-006**: The system MUST visually distinguish a transfer the user has paused from one that is automatically holding position due to a connectivity dropout, so the user is never left wondering why a transfer stopped moving.
- **FR-007**: A paused transfer MUST remain paused and resumable across an app restart (quit and reopen), rather than being lost or silently restarted from the beginning.
- **FR-008**: When the user pauses the transfer that currently holds the single active upload slot, the next queued transfer MUST be free to begin in its place, consistent with existing cancel behavior.
- **FR-009**: If a transfer reaches a completed state (fully confirmed by Drive) before a pending Remove or Pause action takes effect, the system MUST record it as completed rather than removed or paused.
- **FR-010**: Attempting to resume a paused transfer whose local file can no longer be found MUST surface a clear, plain-language message rather than silently failing or resuming incorrectly.

### Key Entities

- **Transfer**: A single file's upload to Google Drive, tracked through statuses (queued, uploading, holding position after a dropout, paused by user, awaiting a decision, completed, removed/canceled, failed). This feature adds "paused by user" as a status distinct from the existing automatic connectivity-hold state, and extends removal to apply consistently across every non-final in-flight status.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From any in-flight transfer, a user can pause, resume, or remove it in two actions or fewer (e.g., open its controls, tap the action), from both the Home dashboard and the Transfers page.
- **SC-002**: A resumed transfer never re-sends bytes that were already confirmed by Google Drive before it was paused.
- **SC-003**: 100% of paused transfers survive an app restart in a resumable state — none are silently lost or restarted from zero.
- **SC-004**: Users can correctly tell, without opening a detail view, whether a stopped transfer is paused by their own action or automatically holding position due to a connectivity issue.
- **SC-005**: A removed transfer never reappears as an in-flight item or a recovery prompt on a later app launch.

## Assumptions

- The current system allows only one transfer to be actively sending bytes at a time, with additional files waiting in a queue; this feature does not change that constraint, but pausing the active transfer is expected to free its slot for the next queued transfer, matching how Cancel behaves today.
- "Remove" ends the transfer for good (same end-state as today's existing Cancel action) rather than merely hiding the row while it keeps running in the background.
- Pausing is a user-initiated hold, separate from the existing automatic pause the system already performs when it hits a retryable connectivity error mid-upload; the two must remain visually distinguishable but can share the same underlying "holding, not moving" mechanism where useful.
- Removing a paused or queued transfer that never sent any bytes still produces a history record, consistent with how other non-completed transfers are recorded today.
