# Ballast

## Problem Statement

Large file uploads to cloud storage providers such as Google Drive are
frustratingly unreliable and inefficient, especially in regions with
unstable or slow internet connectivity.

Users frequently experience uploads that:

-   Pause unexpectedly when connectivity drops.
-   Restart after browser crashes or device reboots.
-   Take several hours for large files.
-   Fail near completion, forcing the user to restart.
-   Provide inaccurate or unreliable progress indicators.
-   Cannot intelligently adapt to changing network conditions.

Although Google Drive supports resumable uploads, the default web
experience is designed for general users rather than environments where
connectivity is intermittent, bandwidth is limited, or uploads are
mission-critical.

The goal of this project is to build a **production-grade upload
engine** that provides the fastest and most reliable upload experience
possible over unreliable networks while remaining fully compatible with
Google Drive.

The system will not attempt to violate the physical limitations of
network bandwidth. Instead, it will minimise wasted bandwidth,
intelligently recover from failures, optimise data transfer, and
maximise the effective use of every available network resource.

## Vision

> **The fastest, most reliable upload engine for unstable internet.**

The upload engine should make users forget that they have unreliable
internet.

Instead of asking:

> "How can we upload faster than the internet allows?"

The project asks:

> "How can we ensure every available byte of bandwidth is used
> efficiently, intelligently and without interruption?"

## How Google Drive's Upload API Actually Works

This section constrains everything below it, so it's called out up front
rather than left implicit.

Google Drive resumable uploads are a **single sequential byte stream per
session**, not an independently-parallelizable multipart upload like S3:

-   Chunks must be sent in order, in multiples of 256 KB, except the
    final chunk.
-   You cannot upload chunks out of order or upload two chunks of the
    *same file* concurrently.
-   On interruption, you query status with an empty `PUT` and a
    `Content-Range: */<total>` header. The server replies:
    -   `308 Resume Incomplete` + `Range` header → resume from the next
        byte.
    -   `404 Not Found` → session expired (sessions expire after 7 days
        of inactivity) — the upload must restart from byte 0.
    -   `200`/`201` → already complete.
-   `403` (rate limit) and `5xx` responses are expected and must be
    retried with backoff; Google does not publish exact numeric quotas
    for this endpoint.

**Implication:** techniques that depend on splitting one transfer across
multiple concurrent streams or network paths (BitTorrent-style piece
scheduling, bandwidth bonding, Multipath TCP) do not apply to a single
file's upload against this API. The only concurrency available to us is
**uploading multiple different files at the same time**, each on its own
sequential session. Every design decision below assumes this.

## Core Pain Points

1.  **Interrupted uploads**
    -   Connectivity drops pause or fail uploads.
    -   Browser refreshes or crashes lose state.
    -   Users should be able to resume automatically, from the correct
        byte offset, without re-uploading already-received data.
2.  **Browser dependency**
    -   Browser crashes, sleep mode, tab closures and memory limits
        interrupt uploads.
    -   A dedicated upload engine should continue independently.
3.  **Slow uploads**
    -   Poor retry logic.
    -   Inefficient buffering.
    -   Suboptimal chunk sizing.
    -   Idle periods between retries.
4.  **Poor visibility**
    -   Users lack insight into upload progress, speed changes,
        completion estimates and failures.
5.  **Network instability**
    -   Upload behaviour should adapt automatically to latency, packet
        loss, bandwidth fluctuations and temporary disconnections.
6.  **Resource inefficiency**
    -   Avoid excessive CPU, RAM, disk usage and unnecessary
        retransmissions.
7.  **Auth and session lifecycle** *(previously missing)*
    -   OAuth tokens expire and must be refreshed transparently
        mid-upload.
    -   Resumable sessions expire after 7 days idle and must be detected
        (via `404`) and restarted cleanly rather than left in a stuck
        state.
    -   Rate-limit (`403`) and server (`5xx`) errors must be retried
        with backoff, not surfaced as failures — but `403` is not a
        single error class: `error.errors[].reason` must be inspected.
        `rateLimitExceeded` / `userRateLimitExceeded` are retryable;
        `storageQuotaExceeded` / `insufficientFilePermissions` are
        terminal and must surface to the user, not loop forever.

## Objectives

Build a cross-platform upload engine that:

-   Uploads directly to Google Drive, correctly implementing the
    resumable session protocol (sequential chunking, offset queries,
    session-expiry handling).
-   Maximises effective upload throughput **within** the constraints of
    a single sequential stream per file, plus safe concurrency across
    multiple files.
-   Survives crashes, power failures and OS restarts by persisting
    session URI, byte offset, and file identity (hash + mtime) to disk.
-   Automatically resumes uploads without re-sending already-acknowledged
    bytes.
-   Intelligently retries failures, classifying by parsed error reason
    rather than status code alone: retryable (`403 rateLimitExceeded` /
    `userRateLimitExceeded`, `5xx`, network) vs. terminal (`404` expired
    session, `403 storageQuotaExceeded` / `insufficientFilePermissions`,
    auth revoked).
-   Dynamically adapts chunk size and concurrency to measured network
    quality (throughput, RTT, loss).
-   Detects and safely handles source-file changes during a paused
    upload (hash/mtime mismatch), rather than resuming onto corrupted
    data. For files in the hundreds-of-GB range, use size+mtime as a
    cheap first-pass check before paying for a full re-hash, to avoid a
    full-file read on every resume attempt.
-   Provides enterprise-grade reliability, including transparent OAuth
    token refresh.

## Non-Goals

The project will **not**:

-   Bypass ISP bandwidth limits.
-   Upload faster than the physical network allows.
-   Modify Google's backend.
-   Break or circumvent Google Drive policies.
-   Attempt to parallelize the byte stream of a *single* file's upload
    session — Drive's API does not support this. Concurrency is
    achieved across files, not within one.

Instead, it focuses on eliminating inefficiencies in the upload client.

## Research Areas

### Directly applicable to a Drive resumable session
-   Google Drive Resumable Upload API (protocol details above)
-   Adaptive chunk sizing based on measured throughput/RTT
-   Intelligent, backoff-based retry policies
-   HTTP/2 vs HTTP/3/QUIC transport behaviour, *if and where Google's
    endpoint supports it* — treat as an opportunistic win, not a load-
    bearing design assumption
-   TCP congestion-control research (BBR, AIMD) — this is the
    algorithmic basis for the adaptive chunk-size policy below, not a
    novel algorithm we need to invent. QUIC implementations typically
    default to BBR internally, so if the HTTP/3 client library exposes
    congestion signals, we may get part of this behaviour for free at
    the transport layer instead of reimplementing it at the application
    layer.

### Useful as prior art / competitive benchmark, not directly transferable
-   AWS S3 Multipart Upload (independently-parallelizable — Drive is
    not; useful only as a contrast case)
-   Dropbox block-based uploads
-   OneDrive upload sessions
-   rsync delta transfer (relevant only if we ever do local diffing
    before upload, not to the Drive transfer itself)

### Exploratory — likely inapplicable given Drive's single-stream API
Keep for engineering curiosity, but do not plan core architecture around
these unless Drive's API changes:
-   BitTorrent piece scheduling
-   Multipath TCP (MPTCP)
-   Bandwidth bonding
-   Custom congestion control below the OS/HTTP stack
-   UDP-accelerated transfer protocols (e.g. Aspera FASP) — the
    standard industry answer to "big file, bad network, go fast," but
    it requires the *receiving server* to speak the custom protocol.
    Drive only speaks HTTP, so this is not available to us. Documented
    here so it isn't re-proposed later without this constraint in mind.

## Optimisation Strategies

-   Adaptive chunk sizing (in multiples of 256 KB, per Drive's
    requirement)
-   Intelligent retry policies (retryable vs. terminal error
    classification, exponential backoff with jitter)
-   Upload scheduling and **cross-file concurrency** (parallel sessions
    for different files, not parallel chunks of one file)
-   Persistent, crash-safe upload sessions (session URI + byte offset +
    file hash in SQLite)
-   Network quality monitoring to drive chunk-size/concurrency decisions
-   Compression — only for file types where it demonstrably helps
    (e.g. uncompressed text/logs/CSV); skip for already-compressed
    media, archives, and video, where it wastes CPU for no gain
-   Local deduplication — skip re-uploading content whose hash already
    exists in a completed or in-progress session
-   Checksum verification of completed uploads against source
-   Background upload daemon, independent of the browser/UI process

## Suggested Technology Stack

**Language** — Go

**Desktop** — Tauri / React / TypeScript, *or* **Wails** (Go backend +
web frontend, single language, no Rust/Go split) — worth a short spike
to compare before committing, since Tauri's native backend is Rust and
pairing it with a Go sidecar adds a second language and an IPC boundary
that Wails avoids.

**Storage** — SQLite (session state, resume offsets, file hashes —
encrypted at rest, since this store will hold OAuth tokens)

**Observability** — structured logs (Zap) and local metrics as the
default. Prometheus/Grafana/OpenTelemetry are more appropriate if
there's a fleet/enterprise server component collecting metrics from many
installs; for a single-user desktop client, confirm that component
exists before adopting this stack, or scope it down to OTel export only.

**Testing** — Go testing, Playwright, GitHub Actions

**Networking** — HTTP/2, HTTP/3/QUIC (opportunistic), gRPC (internal)

## Success Criteria

Measurable targets, replacing qualitative goals:

-   Resume an interrupted upload within **2 seconds** of connectivity
    returning, with **zero** re-transmission of already-acknowledged
    bytes.
-   Survive process crash, power loss, and OS reboot with full recovery
    of in-progress uploads on next launch.
-   Complete uploads successfully on a simulated network with 5% packet
    loss and 500ms RTT (no permanent failure; degraded throughput is
    acceptable).
-   Progress reporting accurate to within 1% of actual bytes
    transferred.
-   Scale to file sizes of hundreds of GB without unbounded memory
    growth (streamed, not buffered-in-full).
-   Outperform **rclone's** Drive resumable-upload implementation on
    total transfer time and resume overhead, on an identical unstable-
    network test harness — this is the concrete competitive baseline
    for "fastest, most reliable."

## Engineering Review Notes (Open Design Questions)

This spec has been through a senior-engineer review. Small factual
corrections were folded directly into the sections above (403 error
classification, key management, hash-check cost). The items below are
larger design gaps that need an explicit answer before implementation
starts — each is load-bearing for a stated Success Criterion or Security
Consideration, not a nice-to-have.

Items #1, #2, and #4 now carry a **proposed strawman policy**, grounded
in the congestion-control research noted above (BBR/AIMD) — these are
starting points to prototype and tune against the Success Criteria test
harness, not settled design; treat the specific numbers as guesses
worth measuring, not constants to trust blindly. Item #3 cannot be
resolved by research alone — it needs a named mechanism and then an
actual benchmark run against rclone. Item #5 is product/architecture
decisions for a human to make, not something research answers.

### 1. Chunk-size adaptation algorithm — proposed policy, needs empirical validation
The core tradeoff: larger chunks amortise per-request round-trip
overhead, but on a lossy connection a drop mid-chunk wastes everything
back to the last *acknowledged* byte (Drive only acks on chunk
completion), not the last byte sent. Proposed strawman, adapted from
AIMD congestion-control research (see Research Areas):

-   Start at 8 MiB (32 × 256 KiB).
-   On 3 consecutive successful chunk acks, double the chunk size, up
    to a cap of 64 MiB (raw throughput gain above this is marginal and
    the retransmission cost of a failure keeps growing).
-   On any failed/retried chunk, halve the chunk size immediately, down
    to a floor of 1 MiB (below this, per-request overhead dominates).
-   All sizes stay multiples of 256 KiB per Drive's requirement.

This is a starting point to prototype and tune against the 5%-loss/
500ms-RTT test harness in Success Criteria — not a final answer. Needs
validation before being treated as settled.

### 2. Cross-file concurrency throttle — proposed policy, needs empirical validation
Google does not publish numeric rate limits for this endpoint, so this
uses the same AIMD shape as chunk sizing rather than a fixed pool size:

-   Start at 2 concurrent file sessions.
-   On sustained success (e.g. 30s with no rate-limit errors), add 1,
    up to a configurable cap (default 6 — beyond this, per-user Drive
    quotas are likely to dominate regardless of client behaviour).
-   On any `403 rateLimitExceeded` / `userRateLimitExceeded`, halve the
    concurrent session count immediately and pause new session starts
    for one backoff interval.

Same caveat as #1: strawman numbers to prototype against, not final —
Google's actual undocumented limits will only be known empirically.

### 3. Differentiation thesis vs. rclone (unstated)
"Outperform rclone" is a good concrete benchmark, but currently has no
named mechanism behind it — rclone already does chunked resumable
uploads with retry/backoff. Before building to this target, name the
specific edge(s), e.g.: adaptive chunk sizing rclone doesn't do, faster
reconnect detection, warm daemon vs. cold CLI start, smarter cross-file
concurrency. Each claimed edge should be independently benchmarkable.

### 4. Connectivity-change detection — proposed hybrid approach
The "resume within 2 seconds of connectivity returning" success
criterion needs a detection mechanism. Proposed: OS-level reachability
hooks where available (`NWPathMonitor` on macOS, `NLM`/`NCSI` on
Windows, netlink route monitoring on Linux) as the primary signal, with
a lightweight polling fallback (e.g. a small `HEAD` request every 500ms
while in a disconnected state) so the 2-second target is still met on
whichever platform's hook integration lands first. This keeps the
target achievable without blocking all platforms on all three
platform-specific integrations being done up front.

### 5. Minor / scoping questions, unresolved
-   `gRPC (internal)` in the tech stack — internal to what boundary? If
    this ships as a single daemon+UI process (per the Wails option), a
    local IPC channel may be sufficient; gRPC implies a service split
    not otherwise described.
-   The initial resumable-session-creation `POST` (metadata → session
    URI) has its own failure/retry surface, distinct from the chunk
    `PUT` retries covered in Core Pain Points #7 — not yet called out
    as a separate case.
-   Multi-account support is unscoped: is this one Google account per
    install, or does the session store need to key state by account?

## Security Considerations

-   OAuth tokens and resumable session URIs stored locally (SQLite) must
    be encrypted at rest, with the encryption key held in the OS
    keychain (macOS Keychain / Windows DPAPI-Credential Manager / Linux
    Secret Service) — not stored alongside the database file. An
    app-managed key sitting next to the encrypted DB is not real
    encryption at rest.
-   Token refresh must happen transparently without exposing credentials
    in logs.
-   Background daemon must run with least-privilege OS permissions and
    be code-signed/notarized for macOS/Windows distribution.

## Final Vision

**Ballast** is a production-grade upload engine for Google Drive that
transforms unreliable internet into a resilient upload experience.
Rather than attempting to overcome the physical limits of bandwidth, it
intelligently maximises every available byte of network capacity —
within the real constraints of Drive's sequential resumable-upload
protocol — through adaptive chunk sizing, cross-file concurrency,
resumable sessions, fault tolerance, network-aware optimisation, and
enterprise-grade reliability.
