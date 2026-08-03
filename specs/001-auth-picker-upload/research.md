# Research: Google Sign-In, File/Folder Picker & Basic Upload

All items below were flagged as `NEEDS CLARIFICATION` in Technical Context or
left open by the spec's Assumptions. Each is resolved with a decision,
rationale, and alternatives considered.

## 1. OAuth 2.0 flow for a desktop app

**Decision**: Google OAuth 2.0 "installed application" flow using a loopback
IP redirect (`http://127.0.0.1:{ephemeral-port}/callback`), with PKCE
(S256 code challenge). The Go backend starts a short-lived local HTTP
listener on an OS-assigned port, opens the user's system browser to Google's
consent screen, and captures the redirect once the user approves. Use
`golang.org/x/oauth2` + `golang.org/x/oauth2/google` for token exchange and
refresh.

**Rationale**: Google explicitly continues to support the loopback flow for
desktop app OAuth clients (it has deprecated loopback for native
iOS/Android/Chrome-app clients and retired the old out-of-band/copy-paste
code flow entirely, but desktop remains supported). PKCE closes the
main weakness of loopback redirects (a second local process racing to claim
the redirect) by making a stolen auth code useless without the original code
verifier. This satisfies FR-001/FR-002 and the sign-in Edge Case (deny/cancel
mid-flow → clean signed-out state, no partial session) with a standard,
well-supported mechanism.

**Alternatives considered**:
- *Out-of-band (OOB) "copy this code" flow* — rejected: retired by Google,
  no longer available for new OAuth clients.
- *Embedded webview login (loading Google's login page inside the Wails
  webview itself)* — rejected: Google blocks/flags sign-in attempts from
  embedded webviews it can't distinguish from a real browser (anti-phishing
  policy), and it would require handling Google's session cookies inside the
  app's own webview, which is unnecessary complexity for zero benefit over
  loopback + system browser.

**Scopes**: `drive.file` (write access limited to files/folders the app
itself creates or that the user opens with the app) is insufficient alone,
because FR-005 requires browsing *pre-existing* folders the app didn't
create. Decision: request `drive.file` (upload capability) +
`drive.metadata.readonly` (list/read folder structure by metadata only, no
file content access) — the least-privilege scope pair that satisfies both
FR-005 (browse existing folders) and FR-006 (upload) without requesting full
`drive` read/write access to the user's entire Drive contents.

## 2. Drive destination browser: custom vs. Google Picker

**Decision**: Build a minimal custom folder browser: the Go backend calls
Drive API `Files.List` with
`q: "mimeType='application/vnd.google-apps.folder' and trashed=false and 'PARENT_ID' in parents"`,
paginated, exposed to the frontend as a Wails-bound method
(`ListDriveFolders(parentID string)`); the frontend renders a simple
breadcrumb/tree view, defaulting to "My Drive" (root) per Acceptance
Scenario 2 of User Story 2.

**Rationale**: The spec's own Assumptions section treats a purpose-built
browser and Google's embedded Picker widget as equally valid for FR-005.
Google's Picker is a hosted JS widget (`apis.google.com/js/api.js`) designed
for pages served from a registered HTTPS origin in a real browser context;
embedding it inside a Wails webview (served from a custom scheme in
production builds, not a stable HTTPS origin Google's console recognizes)
adds a second JS SDK, an OAuth-token-passing bridge between Go and that SDK,
and a fragile "authorized origins" story — for a feature need (folder
browsing) that a handful of `Files.List` calls already satisfy. Per
Constitution Principle V, this extra dependency is not justified.

**Alternatives considered**:
- *Google Picker widget embedded in the Wails webview* — rejected per above;
  revisit only if the custom browser's UX (e.g. very deep folder trees, slow
  pagination) proves inadequate in practice.

## 3. Non-resumable Drive upload with progress reporting

**Decision**: Use `google.golang.org/api/drive/v3`'s
`Files.Create(...).Media(reader, googleapi.ChunkSize(0))` — passing
`ChunkSize(0)` forces the client library to perform a single-request,
non-resumable multipart upload rather than its default resumable-protocol
path, satisfying the spec's explicit "basic (non-resumable) upload"
requirement and keeping this feature clear of Constitution Principle
II/III's resumable-engine gates. Progress is reported by wrapping the source
`io.Reader` in a small counting-reader that emits a Wails event
(`upload:progress`) with bytes-sent/total on every read, throttled to at most
once per ~1s so SC-004's "at least every 5s" is comfortably met without
flooding the frontend.

**Rationale**: `FilesCreateCall` supports both `.Media()` (simple) and
`.ResumableMedia()` (resumable); `ChunkSize(0)` is the documented way to force
the simple, single-request path through the shared client. Wrapping the
reader is cheaper and simpler than the library's built-in
`.ProgressUpdater()` hook interacting with resumable-only chunking
internals, and keeps progress reporting decoupled from which upload mode is
in use.

**Alternatives considered**:
- *`.ProgressUpdater()` callback* — works today but is documented against the
  resumable path; simple-path progress via `ProgressUpdater` isn't
  reliably granular since the whole body is written in one request.
  Reader-wrapping is protocol-agnostic and simpler to test in isolation.

## 4. Encrypted-at-rest token storage with an OS-keychain key

**Decision**: Store OAuth tokens as AES-256-GCM ciphertext blobs in SQLite
columns (not full-database encryption). The 256-bit data-encryption key is
generated once on first sign-in, stored exclusively in the OS keychain via
`zalando/go-keyring` (macOS Keychain / Windows Credential Manager / Linux
Secret Service over D-Bus), and re-fetched from the keychain on every launch
— it is never written to disk anywhere in the app's own files.

**Rationale**: Full-database encryption (e.g. SQLCipher) requires cgo and a
non-trivial per-OS build/link story, which cuts against Principle VII
(cross-platform parity) and Principle I (stack discipline: avoid new major
dependencies without justification) for a feature that only needs to protect
two columns (access token, refresh token). Column-level AES-GCM with a
keychain-held key satisfies Principle IV's literal requirement ("OAuth
tokens... MUST be encrypted at rest... key... MUST be held in the OS
keychain... MUST NOT be stored in a file alongside the database") with less
surface area. `modernc.org/sqlite` (pure Go, no cgo) is used as the driver,
consistent with avoiding cgo cross-compilation risk.

**Cross-platform fallback (Principle VII)**: Linux Secret Service depends on
a running keyring daemon (e.g. gnome-keyring), which isn't guaranteed on
every distro or headless/minimal-DE session. Decision: if `go-keyring`
cannot read or write the key, sign-in fails closed with a clear, specific
error ("Secure credential storage isn't available on this system; Ballast
cannot sign in without it") rather than silently storing the key
unencrypted next to the database. This is a deliberate, documented
per-Principle-VII fallback path, not a silent gap — it trades availability
on a narrow class of misconfigured Linux systems for never violating
Principle IV.

**Alternatives considered**:
- *SQLCipher (full-database encryption)* — rejected: cgo dependency,
  heavier cross-compilation burden, no benefit over column-level encryption
  for the two sensitive columns this feature actually has.
- *Silently falling back to an unencrypted key file when the keychain is
  unavailable* — rejected outright: directly contradicts Principle IV's
  explicit prohibition, regardless of convenience.

## 5. Testing a Wails app with Go tests + Playwright

**Decision**: Backend logic (auth flow minus the actual browser round-trip,
token encryption/decryption, Drive client wrapper request-building, upload
outcome classification) gets Go `testing` unit tests with the Drive API and
OS keychain faked/mocked at their package boundaries. UI flows (the three
user stories end-to-end) are driven by Playwright against the server Wails'
own dev tooling exposes (`wails dev` serves the frontend at
`http://localhost:34115` by default), letting Playwright drive it like any
local web app without needing a native-webview automation bridge.

**Rationale**: Wails v2 has no first-party E2E testing story, but its dev
server already serves the frontend over plain HTTP, which Playwright talks to
natively — no need for a WebView2/CDP bridge (the approach required for
frameworks like Tauri that don't expose a plain-HTTP dev server). This keeps
CI simple: run `wails dev` (or an equivalent headless dev-server invocation)
in the background, point Playwright at `localhost:34115`, and mock the Google
OAuth consent screen and Drive API responses at the network boundary so CI
doesn't depend on a real Google account.

**Alternatives considered**:
- *CDP-based automation of the packaged native window* (the approach used for
  Tauri/Electron apps without an HTTP dev mode) — rejected as unnecessary
  extra CI infrastructure when Wails' existing dev server already gives
  Playwright a plain HTTP target.
