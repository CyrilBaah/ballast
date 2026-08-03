# Research: Resumable, Crash-Safe Upload Engine

All items below were flagged as `NEEDS CLARIFICATION` in Technical Context or
left open by the spec's Assumptions. Each is resolved with a decision,
rationale, and alternatives considered.

## 1. Talking to Drive's resumable protocol directly, not via the SDK's convenience wrapper

**Decision**: Implement the resumable upload session (initiate, chunk PUT,
offset query) as raw HTTP calls (`net/http`) against
`https://www.googleapis.com/upload/drive/v3/files`, authenticated with the
same `oauth2.Config.Client(ctx, tok)` HTTP client `driveService()` already
builds in `app.go` — not through `drive/v3`'s
`Files.Create(...).ResumableMedia()`/`.Media(..., googleapi.ChunkSize(n))`
path that Feature 001 used for the simple upload.

**Rationale**: The whole point of this feature is FR-004 — recovering a
session URI and byte offset *after the process that started the upload is
gone*. `google-api-go-client`'s resumable support (`gensupport` package)
manages the session URI internally for the lifetime of one `Do()` call; it
has no supported way to hand it a session URI recovered from SQLite and
resume from an arbitrary externally-tracked offset. Google's resumable
protocol itself is simple enough (initiate → `Location` header; chunk PUT
with `Content-Range`; offset query via empty-body PUT with
`Content-Range: bytes */{total}`) that reimplementing just this slice
directly is less code and less risk than fighting the SDK's internal state
machine. `drive/v3` is still used unchanged for everything that isn't the
upload byte stream itself (folder listing, and building the authenticated
`http.Client`/token source).

**Alternatives considered**:
- *`ResumableMedia()` + `.ProgressUpdater()`* — rejected: no supported hook
  to inject a recovered session URI/offset after a restart; would require
  reaching into unexported internals.
- *Re-uploading the whole file on every resume* — rejected outright: this is
  precisely the non-resumable behavior Feature 001 shipped and this feature
  exists to replace (FR-001, FR-011).

## 2. Chunk size

**Decision**: Fixed chunk size of 8 MiB (`32 × 256 KiB`), a package-level
constant. Every chunk except the final one is sent as an exact multiple of
256 KiB per Constitution Principle II; the final chunk is whatever remainder
closes out the file.

**Rationale**: The spec's Assumptions explicitly defer "adaptive chunk-size
tuning" to a separate future feature — this slice needs *a* correct,
protocol-compliant fixed size, not the AIMD algorithm. 8 MiB is chosen
because it's already the documented starting point for that future AIMD
policy (`Ballast_Project_Problem_Statement.md`), so this feature's constant
becomes that later feature's starting value instead of a second number to
reconcile.

**Alternatives considered**:
- *256 KiB (the protocol minimum)* — rejected: correct but wastes
  round-trips on fast connections; no reason to start at the floor when a
  larger fixed value is still safely within memory bounds (FR-012) and one
  chunk buffer is trivially small relative to hundreds-of-GB files.
- *Implementing AIMD sizing now* — rejected: explicitly out of scope per the
  spec's own Assumptions; would also pull in Constitution Principle III's
  "validate against the network-simulation harness before treating as
  settled" obligation for a mechanism this feature doesn't need yet.

## 3. Detecting "connectivity returned" — no OS reachability watcher

**Decision**: No OS-level network-reachability API is used. A dropped
connection or failed chunk send is treated purely as a retryable error (§4);
recovery is detected implicitly by the next retry attempt succeeding, not by
a separate connectivity signal.

**Rationale**: Constitution Principle VII requires any OS-specific
integration to ship with an explicit fallback, adding real per-platform
surface area (macOS `SCNetworkReachability`, Windows `NLM`, Linux
`NetworkManager`/`netlink` — three more OS integrations alongside the
keychain one Feature 001 already carries). A retry loop that simply keeps
trying the next chunk (or an offset query if it's unsure what Drive has)
achieves the same observable outcome — resumes as soon as the network is
back — without any of that surface area, and keeps this feature at
Principle VII **N/A** rather than requiring a third per-OS fallback story.

**Alternatives considered**:
- *Per-OS reachability watcher* — rejected per above: real complexity for a
  signal the retry loop already gives us for free.
- *DNS/TCP probe to a fixed host (e.g. `8.8.8.8`)* — rejected: adds a
  network dependency of its own, doesn't reflect whether *Drive specifically*
  is reachable (e.g. captive portals), and a failed probe still just means
  "retry the chunk," so it's strictly extra code for no better outcome.

## 4. Retry backoff shape and error classification

**Decision**: Two-tier backoff for retryable errors: a **fast tier** — retry
every 2s for the first 30s of continuous failure — then an **escalating
tier** — exponential backoff (base 2s, ×2 per attempt, capped at 30s) for as
long as the failure continues. Retries never stop on their own for a
retryable error (FR-007 has no retry-count ceiling — only terminal problems
stop retrying, per FR-006/FR-008); the upload simply stays `paused` and
visibly "in progress" to the user throughout.

Classification (HTTP status / transport outcome → bucket):

| Condition | Bucket | User-visible outcome |
|---|---|---|
| Network I/O error (timeout, connection reset, EOF mid-chunk) | Retryable | stays `paused`, auto-retries |
| `429`, `403` with reason `rateLimitExceeded`/`userRateLimitExceeded` | Retryable | stays `paused`, auto-retries |
| `500`/`502`/`503`/`504` | Retryable | stays `paused`, auto-retries |
| `403` with reason `storageQuotaExceeded` | Terminal | `failed`, "Google Drive storage is full" |
| `401`/`403` where silent token refresh also fails | Terminal | `failed`, "signed out — sign in again" (reuses Feature 001's `driveService()` refresh-failure path) |
| `404`/`410` on the session URI itself (chunk send or offset query against an already-initiated session) | Terminal, recoverable | `awaiting_confirmation`, "this upload's session expired — restart from the beginning?" |
| `404` on session *initiation* (or a `404`/`400` referencing the `parents` field in the error body), or `404` on the session URI whose error body names the parent rather than the upload session | Terminal, not recoverable | `failed`, "the destination folder no longer exists — choose a new destination" |
| Local file identity check fails (§5) | Terminal, recoverable | `awaiting_confirmation`, "the local file changed — restart from the beginning?" |
| Local file missing (`os.Stat` fails) | Terminal, not recoverable | `failed`, "local file can no longer be found" |
| User calls `Upload.Cancel` on a `paused`/`awaiting_confirmation` upload | N/A — direct user action, not a detected error | `cancelled` (§8) |

Distinguishing the two `404` rows above matters because they demand
opposite responses: a session-URI-gone `404` means *the transfer* can be
retried by restarting the same logical upload (same destination, just a
fresh session), whereas a missing-parent `404` means *the destination
itself* is gone and restarting a session against it would just fail again
— Drive's JSON error body's `errors[].reason`/`errors[].location` (e.g.
`reason: "notFound"` with `location: "parents"`) is what distinguishes
them, not the bare HTTP status code.

**Rationale**: SC-001 requires resuming "within 2 seconds of connectivity
returning" — a 2s fast-tier interval satisfies this for the common case (a
brief Wi-Fi blip, a laptop sleep/wake) without hammering Drive during a
genuine multi-minute/hour outage, where the escalating tier takes over. Per
Constitution Principle III, these two intervals (2s fast-tier duration, 30s
escalation cap) are explicitly **starting hypotheses**, not settled values —
they MUST be run against the network-simulation test harness (§6) before
being trusted, same as any other empirically-tuned constant.

**Alternatives considered**:
- *Flat exponential backoff from the first failure* — rejected: a single
  slow-drop right when connectivity returns (e.g. next attempt scheduled 16s
  out) would blow SC-001's 2-second bound on the most common case.
- *Uncapped exponential growth* — rejected: could reach multi-minute
  intervals during a long-but-eventually-recovered outage, which reads to
  the user as the app having given up even though FR-007 says it hasn't.

## 5. Source File Identity check (FR-009) without hashing the whole file on every resume

**Decision**: `size` and `mtime` are captured once at upload creation
(`os.Stat`) and stored on the Upload row — the cheap check. In addition, a
streaming SHA-256 digest is updated incrementally as bytes are sent and
acknowledged, and its serialized state (`encoding.BinaryMarshaler`, which
`crypto/sha256`'s digest type supports) is checkpointed to SQLite alongside
`bytes_sent` after each acknowledged chunk — so the stored hash always
covers exactly `[0, bytes_sent)` of the file as of the last checkpoint, at
no extra I/O cost (it's computed from the same bytes already being read to
upload them).

On any resume (in-process retry excluded — see below), the check is:
1. Cheap: does current `os.Stat` size/mtime match the stored snapshot? If
   yes, resume immediately, no hashing.
2. Full (only on cheap-check mismatch): re-read just the local file's
   `[0, bytes_sent)` prefix, hash it, and compare to the checkpointed digest.
   Match → the already-acknowledged bytes are still intact, safe to resume
   despite the mtime/size discrepancy (e.g. some tool touched the file
   without altering its content) — resume without asking. Mismatch (or the
   file is now shorter than `bytes_sent`) → FR-010's confirm-before-restart
   flow.

The full check only ever re-reads/re-hashes up to `bytes_sent` bytes — never
the whole file, and never the not-yet-sent remainder — satisfying the edge
case's "must avoid re-hashing the entire file on every resume attempt." The
in-process auto-resume path (User Story 1: a network blip during an
already-running upload) skips this check entirely — the process never lost
its handle on "what the file is," so there's nothing new to verify; the
check applies to resuming a *persisted* upload (after an app restart, or an
explicit user-initiated resume of a long-paused upload — User Story 2/3).

**Rationale**: This is the only design that satisfies FR-009's literal
requirement (cheap check first, full content check as a fallback) and the
edge case's cost constraint simultaneously, without needing an expensive
whole-file pre-hash at upload creation (which would double the disk I/O
before a multi-hundred-GB upload even starts) or an unfounded hash
comparison with no baseline to compare against.

**Alternatives considered**:
- *Whole-file SHA-256 computed once at upload creation, compared in full on
  every fallback* — rejected: correct but pays a full extra read pass over
  the entire file upfront regardless of whether the fallback is ever needed,
  which is a real, avoidable cost at the file sizes FR-012 targets.
- *Cheap check only, no fallback (any mismatch → confirm)* — rejected:
  simpler, but doesn't implement FR-009's explicit two-step check, and would
  force an unnecessary confirmation prompt on the (real, if rare) case of a
  tool touching a file's mtime without changing its bytes.

## 6. Test harness for retry/resume/classification logic

**Decision**: Extend the existing `mock_e2e.go` fake-server pattern (already
used for Playwright's `network-fail` scenario) into a reusable Go
`httptest.Server`-backed fake resumable-upload endpoint under
`internal/drive`, parameterized to simulate: connection drops
mid-chunk (hijack+close, as `mock_e2e.go` already does for the simple
upload), `429`/`5xx` responses, `404`/`410` (expired session), and
configurable latency — driving the Go unit tests required by Constitution
Principle III (session/offset/resume logic and retry classification tests,
written before/alongside the implementation) and giving Principle VI's
reliability gates (e.g. "successful completion under simulated 5% packet
loss / 500ms RTT") a concrete harness to run against.

**Rationale**: `mock_e2e.go`'s hijack-and-close technique for simulating a
dropped connection is already proven in this codebase; reusing the same
technique in a Go-test-scoped harness (rather than only the Playwright-level
mock) is what makes it possible to unit-test retry/backoff/classification
logic directly, in-process, without spinning up Playwright/`wails dev` for
every case.

**Alternatives considered**:
- *Only testing through Playwright's `BALLAST_E2E_MOCK` path* — rejected:
  too slow and coarse-grained for the volume of retry/classification edge
  cases Principle III requires (rate-limit, quota, expired session, dropped
  mid-chunk, etc.), and doesn't give the "before the implementation exists"
  failing-test workflow Principle III mandates at the unit level.

## 7. Startup recovery (FR-005)

**Decision**: On `startup()`, after opening the DB, query for an Upload row
whose status is `in_progress`, `paused`, or `awaiting_confirmation` (at most
one, per FR-013's single-active-upload constraint). If found, the frontend
is told about it via a new `Upload.GetRecoverable()` binding (contracts.md)
instead of the transfer restarting silently in the background before the UI
has even mounted. If the recovered upload's identity/session checks (§5)
still pass, the backend begins resuming it right away (matching the
Assumptions' "automatically resume... without a confirmation prompt" for a
same-transfer resume) while the UI shows it as "Resuming upload..."; if they
don't pass, it's surfaced already in `awaiting_confirmation` so the UI can
prompt immediately rather than the user having to click anything first.

**Rationale**: Matches Acceptance Scenario 2 of User Story 2 ("it detects
the interrupted upload and shows the user it can be resumed") while staying
consistent with the Assumptions' distinction between resuming (silent) and
restarting from byte 0 (always confirmed).

## 8. Cancelling a paused or awaiting-confirmation upload (FR-014)

**Decision**: `Upload.Cancel(id)` is only valid when the upload's status is
`paused` or `awaiting_confirmation`. It makes a best-effort `DELETE` call
against the stored `session_uri` (Drive's documented way to release a
resumable session server-side — research.md §1's endpoint family), ignoring
any error from that call (the local cancellation must succeed regardless of
whether Drive's cleanup call succeeds), then transitions the row directly
to the new terminal `cancelled` status (data-model.md). No new Wails event
is added for this — the call is a direct, synchronous user action, so the
frontend that invoked it already knows the outcome from the method's
return; unlike the automatic transitions (`upload:paused`,
`upload:awaiting-confirmation`, `upload:failed`), there's no other listener
that needs to be told asynchronously.

**Rationale**: FR-013's single-active-upload constraint needs an explicit
way out of a `paused`/`awaiting_confirmation` upload the user doesn't want
to continue, or that slot would otherwise stay permanently occupied by an
upload the user has no way to end. Restricting `Cancel` to those two
statuses (not `in_progress`) keeps the action unambiguous — an actively
transferring upload isn't "stuck," so cancelling it isn't this feature's
concern and isn't requested by the spec.

**Alternatives considered**:
- *Silently discard the old upload the moment a new one starts* — rejected:
  considered and explicitly rejected in favor of an explicit action during
  clarification (spec's Clarifications session) — a silent discard could
  surprise a user who intended to come back to the paused upload later.
- *Emit `upload:cancelled`* — rejected: no listener needs it that the
  synchronous call result doesn't already satisfy; adding it would be an
  unused speculative event (Constitution Principle V).
