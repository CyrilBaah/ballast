# Contract: Visual Design Tokens

The interface every screen's CSS is written against, defined once in
`frontend/src/styles/tokens.css` and imported before `app.css`/`style.css`
(data-model.md's Visual Design Tokens entity). Screens MUST reference
these custom properties rather than hardcoding colors, font sizes, or
spacing values — this is what FR-001's cross-screen consistency and
FR-006's light/dark support are actually built on.

## Visual direction

Concrete token values below take their direction from established
cloud-storage desktop dashboard conventions (the genre Dropbox, Google
Drive, and comparable file-storage apps share — reviewed via a
publicly-published case study for one such app, "CopyCase," as a concrete
reference point for this genre; research.md §6): a light neutral surface
with a single confident accent color rather than a busy palette, soft
rounded cards for file/folder-shaped content, a persistent left-hand
navigation shell with an account/storage summary anchored at its bottom,
and small color-coded file-type icons as an at-a-glance recognition aid
in list views. Nothing brand-specific (logos, product name, copy) is
adopted — only the layout and color conventions common to this class of
app, adapted to Ballast's own single-account, single-upload-at-a-time
scope. A subtle ambient wave/dot motif (Ambient background, below) adds
a touch of depth behind this otherwise flat, neutral surface — restrained
enough not to compete with file/folder content, consistent with the same
reference genre's occasional use of soft background texture behind an
otherwise minimal dashboard chrome.

## Color roles

| Token | Light value | Dark value | Usage |
|---|---|---|---|
| `--color-bg` | `#F5F7FB` | `#121317` | Page/screen background (replaces `style.css`'s hardcoded `rgba(27, 38, 54, 1)`) |
| `--color-surface` | `#FFFFFF` | `#1B1D23` | Card/panel surface (file tiles, the upload-progress widget, the sidebar) — new, replaces implicit "surface = bg" in today's flat layout |
| `--color-text` | `#14161C` | `#F2F3F5` | Primary text (replaces hardcoded `white`) |
| `--color-text-muted` | `#6B7280` | `#9CA3AF` | Secondary text — replaces today's ad hoc `opacity: 0.6/0.7/0.8/0.85` pattern seen throughout `app.css` |
| `--color-accent` | `#2F5DF4` | `#5B84FF` | Primary buttons, links, active nav item, focus rings, progress-bar fill |
| `--color-accent-hover` | `#2547C7` | `#7C9CFF` | Hover/active state for accent controls |
| `--color-border` | `#E3E7EF` | `rgba(255,255,255,0.08)` | Card/control borders, breadcrumb separators, sidebar divider |
| `--color-error` | `#E5484D` | `#FF6B6B` | Replaces hardcoded `#ff6b6b` |
| `--color-success` | `#1E9E64` | `#3DD68C` | Replaces hardcoded `#2f9e44` |
| `--color-warning` | `#B9790C` | `#F2B84B` | Replaces hardcoded `#ffb020` (light value darkened from a raw amber to clear WCAG AA on `#F5F7FB`) |

Dark values apply under `@media (prefers-color-scheme: dark)` (research.md
§2) — no class or attribute toggle, no persisted preference.

## Typography

| Token | Value | Usage |
|---|---|---|
| `--font-family` | existing Nunito stack (unchanged from `style.css` — no new font asset is introduced) | All text |
| `--font-size-body` | `1rem` | Default body/control text |
| `--font-size-small` | `0.85rem` | Secondary labels (matches today's `.btn-secondary` size) |
| `--font-size-heading` | `1.75rem` | Screen titles (e.g. picker's `<h1>Ballast</h1>`) — raised from `1.5rem` and paired with `--font-weight-heading` for the bolder, more confident heading treatment the reference direction uses |
| `--font-weight-heading` | `800` | Screen titles only — replaces the browser-default `<h1>` weight |

## Spacing scale

| Token | Value |
|---|---|
| `--space-xs` | `0.4rem` |
| `--space-sm` | `0.6rem` |
| `--space-md` | `1rem` |
| `--space-lg` | `1.5rem` |
| `--space-xl` | `2rem` |

Existing ad hoc values (`0.4rem`, `0.6rem`, `0.75rem`, `1.5rem`, `10vh`,
etc.) scattered across `app.css` map onto the nearest step above during
the rewrite; no screen introduces a value outside this scale.

## Shape & elevation

New token category, needed for the card-based file-tile/list-row look the
visual direction above calls for (today's CSS has no card concept at all
— everything is flat text on `--color-bg`).

| Token | Light value | Dark value | Usage |
|---|---|---|---|
| `--radius-sm` | `6px` | same | Buttons, inputs |
| `--radius-md` | `10px` | same | Cards: file/folder tiles, the upload-progress widget, history rows |
| `--shadow-card` | `0 1px 3px rgba(16,24,40,0.08), 0 4px 12px rgba(16,24,40,0.06)` | `none` (use `--color-border` as a 1px outline instead) | Applied to `--color-surface` cards — shadows read poorly against a near-black dark background, so dark mode substitutes a subtle border for the same "raised card" cue |

## File-type accent tokens

New token category for the history screen (User Story 3): a small
colored tag on each row's leading file-type icon, matching the reference
direction's color-coded-file-icon convention, so a user can tell a
document from an image from an archive at a glance without reading the
extension.

| Token | Value (same in both themes) | Applies to extensions |
|---|---|---|
| `--filetype-doc` | `#2F6FED` | `.doc`, `.docx`, `.txt`, `.pdf`-adjacent text formats |
| `--filetype-pdf` | `#E5484D` | `.pdf` |
| `--filetype-image` | `#14B8A6` | `.jpg`, `.jpeg`, `.png`, `.gif`, `.heic` |
| `--filetype-audio` | `#84CC16` | `.mp3`, `.wav`, `.m4a` |
| `--filetype-archive` | `#F59E0B` | `.zip`, `.rar`, `.7z` |
| `--filetype-generic` | `var(--color-text-muted)` | Anything not matched above |

`history.ts` derives the category from the uploaded file's extension
(client-side, from `fileName` — no new backend field) and applies the
matching token as the icon's accent color. This is cosmetic only; it has
no effect on FR-008's actual status data.

## Avatar fallback

The sidebar's account row (FR-011) prefers the user's real Google
profile photo (`AuthStatus.pictureUrl`, contracts/wails-bindings.md),
loaded directly via `<img>`. When it's absent or fails to load, a
generated fallback renders in its place — a circle filled with a
deliberately warm, non-accent gradient (distinct from `--color-accent`,
so it never reads as an active/selected state) containing the first
initial of `name` (or `email` if `name` is also absent). This is not a
new token category — it reuses the existing `--filetype-*` warm hues
(e.g. archive/pdf) as the gradient stops, keeping the palette closed
rather than inventing an avatar-specific color pair.

## Ambient background

A subtle, low-contrast wave/dot motif sits behind every screen,
reinforcing FR-001's "one considered product" feel without competing
with content. It is rendered as one inline `<svg class="wave-layer">`
per screen root — full-bleed on the sign-in screen, and once behind the
sidebar shell (shared across picker/progress/history rather than
duplicated per screen):

```svg
<svg class="wave-layer" viewBox="0 0 880 500" preserveAspectRatio="none" aria-hidden="true">
  <path class="w1" d="M-20,120 C120,70 220,170 360,120 C500,70 600,170 740,120 C820,95 860,110 900,100" />
  <path class="w2" d="M-20,230 C100,190 240,270 380,230 C520,190 640,270 780,230 C840,215 870,225 900,220" />
  <path class="w3" d="M-20,360 C140,320 260,400 400,360 C540,320 660,400 800,360 C850,345 875,352 900,348" />
  <circle cx="120" cy="88" r="4" fill="var(--color-accent)" />
  <circle cx="700" cy="205" r="3.5" fill="var(--color-success)" />
  <circle cx="260" cy="340" r="3" fill="var(--color-warning)" />
  <circle cx="620" cy="380" r="4" fill="var(--filetype-image)" />
</svg>
```

- `.wave-layer path` strokes are `var(--color-accent)` at
  `stroke-opacity` `.08`/`.16`/`.08` (w1/w2/w3), `stroke-width: 1.6`,
  `fill: none`; `.wave-layer circle` at `opacity: .5` — all built from
  existing color-role and file-type tokens, no new custom properties are
  introduced for this motif.
- Positioning: the containing element (`.signin-screen`, `.shell`) gets
  `position: relative; overflow: hidden;`; `.wave-layer` itself is
  `position: absolute; inset: -10% -4%; width: 108%; height: 120%;
  z-index: 0`.
- Motion: `@keyframes waveDrift { from { transform: translateX(0) } to
  { transform: translateX(-36px) } }`, applied as `.wave-layer {
  animation: waveDrift 34s ease-in-out infinite alternate; }` only under
  `@media (prefers-reduced-motion: no-preference)` — matches the
  reduced-motion handling already required elsewhere in this feature
  (FR-007's accessibility requirements).
- Anything meant to read above the wave (the sign-in card, the sidebar,
  the content panel) MUST set `position: relative; z-index: 1` or
  higher — without it, the absolutely-positioned wave paints on top and
  hides the screen's real content.
- Because colors are `var()` references rather than baked-in hex, the
  motif re-themes automatically with the rest of the token set under
  `prefers-color-scheme: dark` — no separate dark-mode wave asset is
  needed.

## Sidebar translucency

`.sidebar`'s surface is `color-mix(in srgb, var(--color-surface) 90%,
transparent)` with `backdrop-filter: blur(6px)` (and
`-webkit-backdrop-filter` for Safari/WebKit parity, Constitution
Principle VII) instead of a flat opaque `--color-surface` fill — so the
ambient wave reads faintly through the nav column while nav text/icons
keep full contrast against the still-90%-opaque backing.
`border-right: 1px solid var(--color-border)` is unchanged.

## Content scrim

The `.content` panel (picker/progress/history, inside the sidebar
shell) gets a `::before` layer — `content: ""; position: absolute;
inset: 0; z-index: -1; background: color-mix(in srgb, var(--color-bg)
60%, transparent); pointer-events: none;` — with `.content` itself set
to `position: relative; z-index: 1`. This keeps the wave visible in the
surrounding gutters while dense rows (the folder list, the history
table, the progress card) sit on a calmer, near-opaque field and stay
fully legible.

## Primary button fill

`.btn`'s fill is `linear-gradient(135deg, var(--color-accent),
var(--color-accent-hover))` rather than a flat `--color-accent`, with
`:hover` adding `translateY(-1px)` plus a soft accent-tinted shadow
(`0 6px 14px -4px` of the accent color) and `:active` returning to
`translateY(0)`. The hover lift MUST be gated behind
`@media (prefers-reduced-motion: no-preference)`, matching this
feature's other motion conventions (state-treatment's loading pulse,
the wave drift above).

## State-treatment convention

Every screen's loading/error/success/in-progress indicator (FR-002) is
built from the color roles above, not a bespoke color:

| Visual state | Token | Existing example this replaces |
|---|---|---|
| Success | `--color-success` | `.progress-result--success { color: #2f9e44; }` |
| Error / failed | `--color-error` | `.progress-result--failed`, `.signin-error` |
| Warning / retrying / needs attention | `--color-warning` | `.progress-result--retrying`, `.signin-error--keychain` |
| Loading / in progress | `--color-accent` (often paired with a visible spinner/progress element, not color alone) | today's plain unstyled text |

## Accessibility requirements on tokens

- `--color-text` on `--color-bg`, and `--color-*` state colors on
  `--color-bg`, MUST meet WCAG AA contrast (4.5:1 for body text) in both
  light and dark values (research.md §4) — the light-mode `--color-warning`
  value above was specifically darkened from a raw amber to satisfy this
  against `#F5F7FB`.
- A `--focus-ring` token (`--color-accent`, 2px, with a 2px offset) MUST
  be applied via `:focus-visible` on every interactive element — buttons,
  links, folder-list items, sidebar nav items, history rows — satisfying
  FR-007's keyboard-operability requirement.
- The sidebar's 90%-opacity surface (Sidebar translucency) and the
  `.content` scrim (Content scrim) MUST still meet WCAG AA contrast for
  their text/icons against the composited result (wave motif plus
  translucent surface), not just against the flat token color in
  isolation — re-check contrast with the wave visible, not only in the
  abstract.
