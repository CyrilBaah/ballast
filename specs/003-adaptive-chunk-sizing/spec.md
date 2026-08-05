# Feature Specification: Adaptive Chunk-Size Tuning

**Feature Branch**: `003-adaptive-chunk-sizing`

**Created**: 2026-08-04

**Status**: Draft

**Input**: User description: "Adaptive chunk-size tuning for the resumable upload engine: dynamically grow or shrink the size of each chunk sent to Google Drive based on how the transfer is actually performing, so uploads move faster on healthy connections without wasting retransmission on unstable ones."

## Clarifications

### Session 2026-08-04

- Q: When a paused or crashed upload resumes, what chunk size should the next chunk use? → A: Restore the last known chunk size in use before the interruption, rather than resetting to the baseline.
- Q: When a paused upload's Drive session has expired (7 days of inactivity) and it must fully restart from byte 0, should the restart also reset the chunk size back to baseline, or keep the size the upload had earned before the pause? → A: Keep the earned size — a session-expiry restart is treated the same as any other resume for chunk-size purposes; it does not reset to baseline.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Uploads speed up automatically on a healthy connection (Priority: P1)

As a user uploading a large file over a stable, reasonably fast connection,
I want the app to send progressively more data per request as chunks keep
succeeding, so my upload finishes faster than it would if every chunk
stayed the same small size for the whole transfer — with nothing for me
to configure.

**Why this priority**: This is the feature's core value: most uploads
happen over adequate-but-not-great connections, and reducing per-request
round-trip overhead directly cuts total upload time — the project's
central promise of being the fastest upload experience an unstable
connection allows.

**Independent Test**: Start uploading a large file (large enough to span
many chunks) over a fast, reliable connection with no failures, and
confirm the amount of data sent per request grows over the course of the
transfer up to a defined ceiling, and that the file still arrives
byte-identical to the source.

**Acceptance Scenarios**:

1. **Given** a new upload just started at the baseline chunk size, **When**
   several consecutive chunks are sent and acknowledged successfully,
   **Then** the amount of data sent per request increases.
2. **Given** the chunk size has grown to its ceiling, **When** chunks keep
   succeeding, **Then** the chunk size stops growing and stays at the
   ceiling for the rest of the transfer.

---

### User Story 2 - Uploads back off automatically on an unstable connection (Priority: P2)

As a user on a poor or unstable connection, I want a failed chunk to make
the app send smaller chunks afterward, so a failure wastes less data and
the next attempt is more likely to succeed.

**Why this priority**: This is the safety complement to User Story 1 —
without it, growing chunk size unconditionally would make failures more
costly on exactly the unstable connections this project targets, instead
of less costly.

**Independent Test**: Start an upload, let the chunk size grow above its
starting point, then induce a single chunk failure (e.g. a simulated
dropped connection mid-chunk) and confirm the next attempt uses a smaller
amount of data than the one that failed.

**Acceptance Scenarios**:

1. **Given** the chunk size has grown above its starting point, **When** a
   chunk fails, **Then** the amount of data sent per request decreases for
   the next attempt.
2. **Given** repeated consecutive chunk failures, **When** each new attempt
   also fails, **Then** the chunk size keeps decreasing until it reaches a
   defined floor and never goes below it.
3. **Given** the chunk size has just decreased because of a failure,
   **When** the very next chunk succeeds, **Then** growth resumes under
   the same rule as User Story 1 (several consecutive successes required
   before growing again) rather than immediately jumping back to the size
   that just failed.

---

### User Story 3 - Resuming an upload picks up at a sensible size, not from scratch (Priority: P3)

As a user whose upload was paused by a dropped connection or interrupted
by an app restart, I want it to resume using the chunk size it had
already earned, so a long-running upload doesn't have to slowly re-ramp
up from the smallest size every time it's interrupted.

**Why this priority**: Directly protects the experience of the largest,
longest-running uploads this project is built for — those are exactly
the uploads most likely to be interrupted at least once, and are where
re-ramping from scratch every time would cost the most.

**Independent Test**: Grow an upload's chunk size above baseline, pause it
(dropped connection) or interrupt it (simulated app crash/restart), then
let it resume and confirm the next chunk sent uses the size it had
reached before the interruption, not the baseline.

**Acceptance Scenarios**:

1. **Given** an upload's chunk size had grown above baseline before being
   paused by a dropped connection, **When** connectivity returns and the
   upload resumes automatically, **Then** the next chunk sent uses the
   size it had reached, not the baseline.
2. **Given** an upload's chunk size had grown above baseline before the
   app crashed or was restarted, **When** the app relaunches and resumes
   the upload, **Then** the next chunk sent uses the size it had reached
   before the interruption.
3. **Given** a chunk fails immediately after a resume, **When** the next
   attempt is made, **Then** it shrinks from the restored size following
   the same rule as User Story 2, rather than falling back to the
   baseline first.

### Edge Cases

- What happens when a file is smaller than the baseline chunk size? The
  whole file is sent as one chunk; growth/shrink rules never apply since
  there's only ever one chunk.
- What happens on the very last chunk of a file, when the remaining bytes
  are smaller than the current chunk size? The final chunk is whatever
  remains, same as today — the adaptive size only governs chunks before
  the last one.
- What happens if the very first chunk of a brand-new upload fails, before
  any growth has ever happened? The size decreases from the baseline
  toward the floor, the same as any other failure — there's nothing
  special about the first chunk.
- What happens if network conditions fluctuate rapidly (repeated
  success/failure/success)? Growth requires several consecutive successes
  while a single failure shrinks immediately — this asymmetry is
  intentional so the system leans cautious under flapping conditions
  rather than oscillating at a large size.
- What happens to chunk size on a terminal, not-recoverable failure
  (e.g. storage quota exceeded, permission revoked)? Nothing — the
  transfer stops per the existing terminal-failure handling; chunk-size
  adaptation only concerns itself with attempts that get retried.
- What happens if the paused upload's Drive session has expired (7 days
  of inactivity) before it resumes, forcing a new session and a restart
  from byte 0? Chunk size still restores to the size earned before the
  interruption — a session-expiry restart is not treated as a new
  upload for chunk-size purposes, even though the byte stream itself
  starts over.
- What happens if the paused upload's source file changed during the
  pause, forcing the same explicit-confirmation restart from byte 0 as a
  session expiry? Chunk size still restores to the size earned before
  the interruption — this is the same restart mechanism as the
  session-expiry case above, and the spec draws no distinction between
  the two reasons for chunk-size purposes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST vary the amount of data sent in each chunk
  of a resumable upload over the course of a transfer, rather than using
  one fixed size for the whole upload.
- **FR-002**: Each new upload MUST start at a defined baseline chunk size.
- **FR-003**: After a defined number of consecutive chunks are sent and
  acknowledged successfully, the system MUST increase the chunk size used
  for the next chunk, up to a defined ceiling.
- **FR-004**: The system MUST NOT increase the chunk size beyond the
  ceiling, no matter how many further chunks succeed.
- **FR-005**: When a chunk attempt fails in a way that gets retried, the
  system MUST decrease the chunk size used for the next attempt, down to
  a defined floor.
- **FR-006**: The system MUST NOT decrease the chunk size below the floor,
  no matter how many further chunks fail.
- **FR-007**: Every chunk size the system uses MUST stay within Google
  Drive's required increment for chunk sizes, including after growing or
  shrinking, except the file's final chunk, which may be smaller.
- **FR-008**: A chunk-size decrease MUST reset the count of consecutive
  successes needed before the size grows again, so growth resumes
  gradually rather than immediately returning to the size that just
  failed.
- **FR-009**: When an upload resumes after being paused or interrupted,
  the next chunk sent MUST use the chunk size the upload had reached
  before the interruption, not the baseline. This applies even when the
  prior Drive session has expired and the upload must start a new
  session from byte 0 — a session-expiry restart is not treated as a
  new upload for chunk-size purposes.
- **FR-010**: Chunk-size adaptation MUST NOT change any of the existing
  resumable-upload guarantees: chunks are still sent strictly in order,
  one at a time, never concurrently for the same file, and no
  already-acknowledged bytes are ever re-sent.
- **FR-011**: The system MUST NOT expose chunk-size behavior as a
  user-facing setting — sizing decisions happen automatically and are not
  something a user configures.
- **FR-012**: A failure that is not retried (a terminal condition) MUST
  NOT trigger a chunk-size change — chunk-size adaptation only responds to
  attempts that get retried.

### Key Entities

- **Chunk-Size State**: The amount of data currently being used per chunk
  for one upload's transfer, and how many consecutive chunks have
  succeeded since the size last changed. Carried alongside an upload's
  other resume information (how much has been acknowledged, its session
  with Drive) so an interrupted upload resumes at the size it had earned,
  per this feature's Clarifications. This state outlives the Drive
  session URI itself: if the prior session has expired and a new one is
  created on resume, the earned chunk size still carries over.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Uploading a large file (1 GB or more) over a stable,
  adequate-bandwidth connection completes at least 15% faster than the
  same file uploaded with a fixed baseline chunk size for the whole
  transfer.
- **SC-002**: Uploading over a degraded connection (intermittent packet
  loss and elevated latency) still completes successfully, with no more
  than 5% of total transferred bytes being re-sent due to failed chunks.
- **SC-003**: Across a full transfer, the chunk size in use never exceeds
  the defined ceiling and never drops below the defined floor.
- **SC-004**: Memory use during an upload stays flat regardless of how
  large the chunk size has grown, consistent with the existing
  bounded-memory guarantee for files of any size.
- **SC-005**: An upload resumed after a pause, crash, or session-expiry
  restart sends its first post-resume chunk at the size it had reached
  before the interruption, not the baseline, in every case.

## Assumptions

- The specific numbers governing growth/shrink (baseline, ceiling, floor,
  and how many consecutive successes are required before growing) are
  adopted from the project's own problem statement as a starting policy,
  not a settled, final answer — they are expected to be validated and
  tuned against real network-simulation testing before being trusted, the
  same way this project already treats its retry-backoff timings.
- This feature operates within the existing one-upload-at-a-time model.
  Running multiple uploads at once (cross-file concurrency) is a separate
  future feature and out of scope here.
- No new user interface is introduced. Chunk-size behavior is fully
  automatic; the only user-visible effect is upload speed.
- "Failure" in this spec's requirements refers specifically to a
  retried/retryable attempt (e.g. a dropped connection, a rate limit, a
  server error) — not a terminal condition (e.g. storage full, permission
  revoked, destination gone), which stops the transfer entirely under the
  existing terminal-failure handling and has nothing to do with chunk size.
