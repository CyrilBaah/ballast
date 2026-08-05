# Research: Full Experience UI/UX Redesign

No `NEEDS CLARIFICATION` markers remain in the spec or Technical Context
— every decision below closes a design choice the spec left open (by
design, per its Assumptions section), not an unresolved unknown.

## §1. Design system implementation approach

**Decision**: Plain CSS custom properties (`frontend/src/styles/tokens.css`)
defining color, typography, and spacing tokens, consumed directly by the
existing per-screen CSS — no CSS framework, utility-class system, or
component library.

**Rationale**: The app has four small screens and no build-tool
complexity beyond Vite's default Wails "vanilla-ts" setup. Constitution
Principle I treats a new major dependency as non-default, requiring
written justification — none exists here. A ~150-line token file plus
consistent class naming (the existing convention: `.screen-name-element`)
fully covers the spec's FR-001 (consistent color/typography/spacing/
control styling) without adding a build step, a new dependency, or a new
class-naming paradigm to learn.

**Alternatives considered**:
- **Tailwind CSS or similar utility framework** — rejected: new major
  dependency, new build-step surface, and a wholesale rewrite of markup
  patterns for a codebase currently at ~200 lines of CSS total.
- **A component library (e.g., a prebuilt Svelte/React kit)** — rejected:
  would also require adopting a UI framework the project doesn't use
  (screens are hand-rolled TypeScript + `innerHTML`, no reactive
  framework); fights the project's existing "single language, minimal
  dependencies" posture already established for the Go backend.

## §2. Light/dark theme mechanism

**Decision**: `prefers-color-scheme` media query in `tokens.css`,
overriding the same custom-property names for dark mode — no in-app
toggle, no persisted theme preference.

**Rationale**: Spec Assumptions explicitly scope this to following the OS
preference, keeping scope bounded per Constitution Principle V. This also
requires no new storage (no settings table/column) and no new Wails
binding.

**Alternatives considered**:
- **In-app toggle with persisted preference** — rejected: adds a settings
  surface and a persistence concern the spec didn't ask for; would need
  its own storage column and UI, out of proportion to this feature's
  scope.
- **Dark-only (matching today's hardcoded `style.css` background)** —
  rejected: fails FR-006, which requires legibility in both OS themes,
  and abandons users on a light-mode OS.

## §3. Upload-history data source

**Decision**: Denormalize the destination folder's display name onto the
`upload` row at creation time (`drive_folder_name` column), rather than
resolving it live from Drive at list-render time.

**Rationale**: Mirrors the precedent Feature 002 already set by
denormalizing `local_mtime` onto the same table for its own read-path
convenience. Avoids adding a new Drive API round trip (with its own
retry/error-classification surface per Feature 002's `retry.go`) just to
render a cosmetic label. The picker screen already has the folder's
display name in memory (its breadcrumb state) at the exact moment
`Upload.Start` is called, so passing it through costs nothing.

**Alternatives considered**:
- **Live Drive API lookup per row when the history screen renders** —
  rejected: extra latency, a new failure mode (what does the list show
  if the lookup fails or the folder was since deleted/renamed?) for a
  label that doesn't need to be live — Drive is the source of truth for
  the file itself, not for this list's cosmetic labeling.
- **Store the full folder path** — rejected: scope creep beyond FR-008
  ("destination folder" — a single name is sufficient for "at a glance"
  per SC-003's 10-second target); the picker's breadcrumb UI already
  shows full-path context at selection time.

## §4. Accessibility verification approach

**Decision**: Semantic HTML (native `<button>`, `<ul>`/`<li>`, existing
`role="alert"` pattern already used for error text) plus visible
`:focus-visible` outlines defined in `tokens.css`; verified manually via
a keyboard-only walkthrough documented in quickstart.md. No automated
accessibility-scanning dependency is introduced.

**Rationale**: FR-007's bar (keyboard-operable, sufficient contrast) is
achievable with correct semantic markup and visible focus states alone,
which the codebase already partially follows (`role="alert"` on error
paragraphs in `picker.ts`/`signin.ts`). Adding a scanning tool (e.g.,
axe-core) would be a new test dependency for a one-time-per-feature
verification need better served by a documented manual pass.

**Alternatives considered**:
- **axe-core / Playwright accessibility scanning in CI** — deferred, not
  rejected outright: worth proposing as a separate, later hardening task
  if the project wants ongoing automated regression coverage, but not
  required to satisfy this feature's FR-007/SC-005.

## §5. Live status updates in the history list

**Decision**: The history screen fetches an initial snapshot via
`Upload.ListRecent` on mount, then subscribes to the same
`upload:progress` / `upload:complete` / `upload:failed` / `upload:paused`
/ `upload:awaiting-confirmation` events `progress.ts` already listens
for, updating the matching row by `id`.

**Rationale**: Zero new backend event surface — these events already
carry everything a list row needs (id, bytes, totals, failure reason).
Matches the established pattern in this codebase (`progress.ts`) rather
than introducing a second data-flow style (polling) for the same
underlying state.

**Alternatives considered**:
- **Polling `Upload.ListRecent` on an interval** — rejected: strictly
  worse than the existing push-based events already available (added
  latency, wasted calls when nothing changed, and a new "how often"
  constant to tune for no benefit).

## §6. Concrete visual direction and navigation shell

**Decision**: Adopt established cloud-storage desktop dashboard
conventions as the concrete visual direction for the tokens in
contracts/design-tokens.md — a light neutral surface with one confident
blue accent (dark-mode counterpart via research.md §2), soft rounded
cards for file/folder-shaped content, and color-coded file-type icons in
list views. This also resolves plan.md's previously open "persistent nav
entry point" question (Foundational phase, `main.ts`): a slim, always-
visible left sidebar with two nav items (Upload, History) and an
account/storage-adjacent summary row anchored at its bottom, wrapping the
signed-in screens (picker, progress, history). Sign-in itself stays
full-screen and outside this shell, unchanged in structure — matching the
reference direction's own convention of a plain, chrome-free auth screen
separate from the dashboard shell, and avoiding a sidebar with nothing
yet to navigate to before a session exists.

**Rationale**: The spec asks for a "cohesive, well-designed visual
system" (User Story 1) without prescribing one, and the existing
interface is an unstyled scaffold with no visual point of view to build
from. Reviewing a publicly-published case study for a comparable
single-purpose file-storage desktop app ("CopyCase," a Behance case study
by Vision Trust) as a concrete reference gave a genre-appropriate,
already-validated direction — this is the same conventions Dropbox,
Google Drive's desktop client, and similar tools converge on, not one
app's proprietary invention. Only layout/color conventions are adopted;
no branding, copy, iconography, or product identity is carried over. A
persistent sidebar also directly fits Ballast's shape better than a
single-column stack once a second screen (history) exists alongside the
picker/progress flow — it's the natural place for navigation once there's
more than one place to navigate to.

**Alternatives considered**:
- **A generic, unopinionated neutral palette (grayscale + one accent,
  no reference point)** — rejected: doesn't resolve the "well-designed"
  bar spec.md's User Story 1 sets; "consistent" alone is satisfiable by a
  bland result, which isn't the same as polished.
- **Top-tab navigation instead of a sidebar** (switching between Upload
  and History via tabs at the top of a single-column layout) — rejected:
  works for two screens today but the sidebar shell scales better if a
  settings/account area is ever added later, and matches the reference
  direction's established pattern for this app category.
- **A new custom font/icon set matching the reference more closely** —
  rejected: stays with the existing Nunito asset and no icon library,
  per research.md §1's no-new-dependency stance; the visual direction is
  expressed through color/shape/layout tokens only, not new assets.

## §7. Account display name and profile photo (FR-011)

**Decision**: Add the standard `https://www.googleapis.com/auth/userinfo.profile`
OAuth scope alongside Feature 001's existing `openid`/`userinfo.email`
scopes, so Google's userinfo response (`internal/auth`'s existing
`FetchUserInfo` call, `internal/auth/oauth.go`) also returns `name` and
`picture`. Both are captured into `auth.Session`/`storage.Account`
alongside the existing `Email` field, at the same point in the same
sign-in flow — no new network round trip, no new flow. The frontend
renders the `picture` URL directly via `<img>`; if it's absent or fails
to load, it falls back to a generated initials avatar (first letters of
`name`, or of `email` if `name` is also absent) rendered entirely in CSS
— no image processing, no avatar caching/storage.

**Rationale**: `profile` is a standard, low-sensitivity OAuth scope
Google's consent screen already groups with `email` in its default
account-info section — this is not a sensitive-scope addition requiring
extra Google verification review, unlike Drive scopes. Capturing it
during the same sign-in flow Feature 001 established (rather than a
separate profile-fetch step) keeps FR-011 additive to, not a redesign
of, the sign-in flow FR-010 protects.

**Alternatives considered**:
- **Fetch profile info via a separate People API call after sign-in** —
  rejected: an extra API surface and its own error/retry handling for
  data the userinfo endpoint already returns for free once the scope is
  granted.
- **Download and store the photo locally** (for offline display, or to
  avoid a live external image request) — rejected: adds file storage,
  cache invalidation (the photo can change), and a privacy-adjacent data
  retention question for a "nice to have" that a direct `<img src>` and
  a same-session-cached URL already solve. The picture URL is treated as
  a display detail, not a secret, and is not encrypted like the OAuth
  tokens.

## §8. Drive storage quota (FR-012)

**Decision**: Call Drive's `about.get` endpoint (`fields=storageQuota`)
once per session — right after sign-in resolves and on each app
launch's silent-refresh path (`App.startup`/`AuthGetStatus`, alongside
Feature 002's existing recoverable-upload check) — via the same
`driveService(ctx)` helper `internal/drive/folders.go` already uses for
`Files.List`. The result (`usageBytes`, `limitBytes`) is held in memory
for the session only, exposed via a new `Drive.GetStorageQuota` Wails
binding; it is not persisted to SQLite.

**Rationale**: `about.get` needs no new OAuth scope — the
`drive.metadata.readonly` scope Feature 001 already requests covers it.
A once-per-session fetch matches FR-012's own "refreshed at least once
per app session" bar without adding a polling loop or a new background
timer, keeping this consistent with the feature's presentation-layer
scope (research.md's framing throughout: reuse existing session
lifecycle events rather than inventing new ones).

**Alternatives considered**:
- **Persist the last-known quota to SQLite** (so it's available
  instantly on next launch before the live call resolves) — rejected:
  quota is exactly the kind of value that goes stale between sessions
  (the user may have freed or used space from another device); showing
  a loading state briefly is preferable to showing a number that might
  already be wrong, and the spec's Assumptions already accept
  once-per-session staleness, not cross-session staleness.
- **Poll periodically while the app is open** — rejected: storage quota
  doesn't change from Ballast's own actions fast enough within one
  session to justify a timer; the one exception (the user's own upload
  completing) doesn't need a separate poll since FR-012 doesn't promise
  live updates during a transfer, only a per-session refresh.

**Failure handling** (spec.md Clarifications, 2026-08-05): if the
`about.get` call fails, the frontend omits the storage indicator for
that session rather than surfacing an error — a deliberately soft
failure mode, since this is a supplementary status readout, not
something the rest of the sidebar (name/photo) or any other screen
depends on. No retry logic is implemented; the next natural retry point
is simply the next app session.
