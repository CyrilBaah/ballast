# Feature Specification: Full Experience UI/UX Redesign

**Feature Branch**: `004-ui-ux-redesign`

**Created**: 2026-08-04

**Status**: Draft

**Input**: User description: "Full UI/UX redesign of Ballast's desktop upload experience: rethink and polish the entire flow end-to-end — Google sign-in, file/folder picker, upload progress and status, and multi-upload management — into a cohesive, well-designed visual system (layout, typography, color, states like loading/error/success, and navigation between screens), replacing the current minimal/unstyled interface built during initial scaffolding."

## Clarifications

### Session 2026-08-05

- Q: When a user has more uploads than fit in one view, should the
  history list show only a bounded, recent set, or every upload ever
  with some way to reach older ones? → A: Bounded recent list only (most
  recent 50) — older uploads age out of view, with no pagination/search
  affordance to reach them from this screen.
- Q: If the live Drive storage-quota lookup fails (network hiccup, API
  error), what should the user see? → A: Hide the storage indicator
  silently for that session — name and photo still render normally; no
  error state or retry affordance for this supplementary readout.
- Q: If Google returns no display name at all (only an email), what
  should the sidebar's name line and avatar initials show? → A: Fall
  back to the email address for the name line, and derive the avatar
  initial from the email's first letter.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A cohesive, polished visual identity across the whole app (Priority: P1)

As a user moving through sign-in, picking a file and destination, and
watching an upload progress, I want every screen to look and feel like
part of the same considered product — consistent colors, typography,
spacing, and button/control styling — instead of the current bare,
unstyled scaffold, so the app feels trustworthy and finished rather than
like a work-in-progress.

**Why this priority**: This is the foundation everything else builds on.
Without a shared visual system, every other improvement (state feedback,
history) just adds more inconsistent pieces to an already inconsistent
whole.

**Independent Test**: Walk through sign-in → file/folder picker → upload
progress using only the redesigned interface, and confirm colors,
typography, spacing, and control styling are visually consistent across
all three screens, with no screen looking visually disconnected from the
others.

**Acceptance Scenarios**:

1. **Given** a user on the sign-in screen, **When** they proceed to the
   picker screen, **Then** typography, color palette, and control styling
   (buttons, links) remain visually consistent between the two screens.
2. **Given** a user on the picker screen, **When** they start an upload
   and move to the progress screen, **Then** the same visual system
   (colors, typography, spacing) continues, with no jarring style shift.
3. **Given** any screen in the app, **When** it is displayed at the
   application's supported window sizes, **Then** layout, spacing, and
   text remain legible and are not clipped or overlapping.
4. **Given** a signed-in user viewing the sidebar, **When** the sidebar
   renders, **Then** it shows the user's Google display name, their
   profile photo (or a generated fallback if none is available), and
   their current Google Drive storage usage — without the user needing
   to open a browser or check Drive directly.

---

### User Story 2 - Always-clear feedback for what's happening (Priority: P2)

As a user waiting on a folder list to load, watching an upload progress,
or hitting an error (sign-in failure, folder-load failure, upload
failure), I want a consistent, unmistakable visual treatment for
"loading," "error," and "success" states across every screen, so I never
have to guess whether the app is working, stuck, or has failed.

**Why this priority**: The underlying reliability work (Features 002 and
003) is only valuable to a user if its results — retries happening,
resume in progress, a terminal failure — are legible in the interface.
Good state feedback is what makes the engine's reliability visible and
trustworthy, not just true in the background.

**Independent Test**: Trigger each state (folder list loading, a
simulated sign-in failure, a simulated folder-load failure, a simulated
upload failure, and a successful upload completion) and confirm each one
renders with a distinct, consistent visual treatment recognizable across
screens.

**Acceptance Scenarios**:

1. **Given** the picker screen is loading Drive folders, **When** the
   request is in flight, **Then** a clear loading indicator is shown
   instead of a blank or frozen-looking list.
2. **Given** any screen encounters an error (sign-in, folder load, or
   upload), **When** the error occurs, **Then** it is shown with a
   consistent visual style (distinct from normal/success content) and a
   message a non-technical user can understand.
3. **Given** an upload completes successfully, **When** the final chunk
   is acknowledged, **Then** the progress screen shows an unambiguous
   success state, distinct from the in-progress state.
4. **Given** an upload is retrying after a dropped connection (per
   Feature 002), **When** the retry is in progress, **Then** the
   interface reflects an active/recovering state rather than looking
   identical to a hard failure.

---

### User Story 3 - See past and current uploads at a glance (Priority: P3)

As a user who has uploaded files before, I want to see a list of my
uploads — what's completed, what's currently in progress, and what
failed — instead of losing that information the moment I leave the
progress screen, so I can confirm past work succeeded without
re-checking Google Drive myself.

**Why this priority**: This is the most net-new UI surface of the three
stories (the other two redesign existing screens), so it's the most
deferrable if scope needs to shrink — but it directly uses upload records
the engine already persists (Feature 002), turning existing data into
visible user value.

**Independent Test**: Complete one upload, start a second, and confirm a
list view shows both — the first marked completed, the second showing
live in-progress status — without needing to leave the app or check
Google Drive directly.

**Acceptance Scenarios**:

1. **Given** a user has completed at least one upload, **When** they view
   the upload list, **Then** it shows the file name, destination folder,
   and a completed status for that upload.
2. **Given** an upload is currently in progress, **When** the user views
   the upload list, **Then** that upload shows a live in-progress status
   reflecting current progress.
3. **Given** an upload previously failed with a terminal error, **When**
   the user views the upload list, **Then** it shows a clearly marked
   failed status with the reason, distinguishable from completed and
   in-progress entries.

### Edge Cases

- What happens when a file name or Drive folder name is very long? Text
  MUST truncate or wrap without breaking layout, spacing, or overlapping
  neighboring elements.
- What happens when the Drive destination folder list is empty (no
  subfolders)? The picker MUST show a clear empty state rather than a
  blank area indistinguishable from a loading or broken state.
- What happens when the user resizes the application window? All
  screens MUST remain legible and usable across the range of window
  sizes the application supports, without clipped controls or
  unreachable content.
- What happens when the OS is set to a dark color theme? The interface
  MUST remain legible and visually consistent whether the OS is in light
  or dark mode.
- What happens when a user relies on keyboard navigation or a screen
  reader? Every interactive control (buttons, folder list items,
  breadcrumbs) MUST be reachable and operable via keyboard, with
  sufficient color contrast for readability.
- What happens when an upload in the history list is still in progress
  and the app is closed and reopened? Per Feature 002's crash-safe
  design, the list MUST reflect the upload's actual persisted status on
  next launch, not lose or misrepresent it.
- What happens when the Drive storage-quota lookup fails? The sidebar
  MUST omit the storage indicator for that session rather than showing
  an error or blocking name/photo from rendering (Clarifications,
  2026-08-05).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST apply one consistent visual design system
  (color palette, typography, spacing, and control styling) across the
  sign-in, file/folder picker, and upload progress screens.
- **FR-002**: The system MUST visually distinguish, in a consistent way
  across all screens, at least these states: idle/ready, loading/in
  progress, error, and success.
- **FR-003**: Every error state (sign-in failure, Drive folder load
  failure, upload failure) MUST present a message a non-technical user
  can understand, styled consistently with other error states in the
  app.
- **FR-004**: The upload progress screen MUST continue to show
  numeric progress (bytes or percentage transferred) introduced in prior
  features, presented within the redesigned visual system.
- **FR-005**: The Drive folder picker MUST clearly show the user's
  current location (breadcrumb/path) and visually distinguish it from
  selectable subfolders.
- **FR-006**: The system MUST remain legible and usable across the
  window sizes the application supports, and under both light and dark
  OS color themes.
- **FR-007**: Every interactive control MUST be operable via keyboard
  and MUST meet standard color-contrast legibility expectations.
- **FR-008**: The system MUST provide a view listing the 50 most recent
  uploads (past and current), showing for each: file name, destination
  folder, and status (completed, in progress, or failed with reason).
  Uploads beyond the most recent 50 are not shown and are not reachable
  from this view (Clarifications, 2026-08-05).
- **FR-009**: The upload list's displayed status MUST reflect each
  upload's actual persisted state (per Feature 002's crash-safe session
  storage), including after the application is restarted.
- **FR-010**: This redesign MUST NOT change any underlying upload,
  retry, resume, or chunk-sizing behavior introduced in Features
  002–003 — it changes presentation and interaction only, aside from
  the two additive, read-only data sources FR-011 and FR-012 introduce
  (an additional OAuth profile scope, and a Drive storage-quota lookup),
  neither of which alters sign-in, upload, retry, resume, or
  chunk-sizing behavior itself.
- **FR-011**: The persistent sidebar MUST display the signed-in user's
  Google display name and profile photo. If no profile photo is
  available (or it fails to load), the system MUST show a generated
  fallback (the user's initials) instead of leaving the space blank. If
  no display name is available either, the system MUST fall back to the
  user's email address for the name line and derive the initial from it
  instead (Clarifications, 2026-08-05).
- **FR-012**: The persistent sidebar MUST display the user's current
  Google Drive storage usage (amount used vs. total available) as a
  visual indicator, refreshed at least once per app session (sign-in or
  app launch). If this lookup fails, the system MUST omit the storage
  indicator for that session rather than showing an error or blocking
  the rest of the sidebar (Clarifications, 2026-08-05) — name and photo
  (FR-011) still render normally.

### Key Entities

- **Visual Design System**: The shared set of colors, typography scale,
  spacing units, and control styles (buttons, inputs, lists, breadcrumbs,
  status indicators) applied consistently across every screen.
- **Upload List Entry**: A user-facing representation of one upload
  record already persisted by Feature 002 (file name, destination
  folder, status, and progress or failure reason) — this feature exposes
  and presents that existing data; it does not introduce new upload
  tracking.
- **Account Profile**: The signed-in user's display name and profile
  photo URL, obtained from Google alongside the email Feature 001
  already captures, and persisted the same way (unencrypted, non-secret
  display data — not a credential).
- **Storage Quota**: The user's Google Drive storage usage (bytes used,
  bytes total), fetched live from Drive on sign-in/app launch. Not
  persisted — always reflects a fresh lookup rather than a potentially
  stale cached value.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A person unfamiliar with the app can identify, without
  explanation, whether an upload is in progress, completed, or failed,
  just from looking at the relevant screen.
- **SC-002**: All screens remain fully legible and usable (no clipped or
  overlapping content) across the full range of window sizes the
  application supports.
- **SC-003**: Users can locate the status of a previously completed or
  in-progress upload in under 10 seconds without leaving the app.
- **SC-004**: The interface remains legible and visually consistent under
  both light and dark OS color themes, with no unreadable text or
  invisible controls in either mode.
- **SC-005**: Every interactive control can be reached and activated
  using only the keyboard.
- **SC-006**: A signed-in user can see who they're signed in as and how
  much Drive storage they have left without leaving the app or opening
  a browser.

## Assumptions

- This redesign covers the presentation and interaction layer only; it
  does not add new upload capabilities (e.g., true concurrent multi-file
  uploads). "Multi-upload management" in this feature means visibility
  into past and current uploads via the upload list, consistent with the
  existing one-upload-at-a-time engine model — genuine concurrent
  uploads remain a separate future feature.
- The application's supported window sizes follow standard desktop
  application conventions (resizable, with a reasonable minimum size);
  no fixed target device sizes were specified.
- Light/dark theme support follows the OS-level preference rather than
  introducing an in-app theme toggle, consistent with standard desktop
  app behavior and keeping scope bounded per the constitution's
  Simplicity & Bounded Scope principle.
- No new backend or data-model work is required for the upload list
  beyond exposing already-persisted Feature 002 upload records to the
  frontend.
- Displaying the account's name/photo (FR-011) requires requesting an
  additional, standard OAuth profile scope beyond what Feature 001
  requested — a one-time consent-screen change, not a new sign-in flow.
  A generated-initials fallback is treated as fully meeting FR-011 when
  Google returns no photo or it fails to load; this feature does not
  need to guarantee a real photo renders.
- Storage quota (FR-012) is fetched once per session (sign-in or app
  launch), not continuously polled or updated in real time as the user
  uploads — acceptable staleness for a status readout, consistent with
  this feature's presentation-layer scope.
- The upload history view (FR-008) is intentionally not paginated or
  searchable — it caps at the 50 most recent uploads, a deliberate scope
  boundary (Clarifications, 2026-08-05) rather than an oversight;
  building a full uploads archive/search experience is a separate future
  feature if ever needed.
