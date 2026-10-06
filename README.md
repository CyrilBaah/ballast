# Ballast

**The fastest, most reliable upload engine for unstable internet.**

Ballast is a production-grade desktop upload engine for Google Drive, built
for environments where connectivity is intermittent, bandwidth is limited,
or uploads are mission-critical. It correctly implements Google Drive's
resumable upload protocol, adapts chunk size and concurrency to real network
conditions, and survives crashes, power loss, and OS restarts without losing
upload progress.

See [`Ballast_Project_Problem_Statement.md`](Ballast_Project_Problem_Statement.md)
for the full problem statement, technical constraints, success criteria, and
open design questions.

## Status

Early — this project is in spec-driven development. There is no runnable
application yet. See [Development Process](#development-process) below.

## Tech Stack

- **Language**: Go
- **Desktop**: [Wails](https://wails.io) (Go backend + web frontend, single process)
- **Storage**: SQLite (session state, resume offsets, file hashes — encrypted
  at rest, key held in the OS keychain)
- **Testing**: Go testing, Playwright, GitHub Actions

## Quickstart

Requires Go 1.22+, Node 20+, and the [Wails CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```sh
wails dev
```

This builds the Go backend, starts the frontend dev server, and opens
Ballast's native window. To exercise Google sign-in, set your own OAuth
desktop-app client credentials first:

```sh
export BALLAST_GOOGLE_CLIENT_ID=your-client-id
export BALLAST_GOOGLE_CLIENT_SECRET=your-client-secret
wails dev
```

To run the test suites:

```sh
go test ./...                 # Go unit tests
cd frontend && npm ci && npx playwright install --with-deps chromium
npm test                      # Playwright, against a running `wails dev` (BALLAST_E2E_MOCK=1 mocks Google/Drive)
```

## Automatic captions

On Apple-silicon Macs, Ballast makes a caption file (`.srt`) for every
`.mp4`, `.mov`, or `.m4v` it uploads, entirely on the Mac, for free. It
saves a copy next to the original video as soon as captions are ready, and
puts one next to the video in Drive once the video has arrived. Captioning
never slows or fails an upload. Intel Macs, Windows, and Linux say captions
aren't available there yet. See `specs/005-automatic-video-captions/`.

- **Speech engine**: [whisper.cpp](https://github.com/ggml-org/whisper.cpp)'s
  `whisper-cli`, run as a helper program. For a packaged app, build it with
  `scripts/build-engines.sh` (needs `brew install cmake` and the Xcode
  command-line tools), then copy it into the app with
  `scripts/build-engines.sh --bundle build/bin/Ballast.app`. For
  `wails dev`, point Ballast at any `whisper-cli`, e.g. Homebrew's:

  ```sh
  brew install whisper-cpp
  export BALLAST_WHISPER_CLI=/opt/homebrew/bin/whisper-cli
  wails dev
  ```

- **Speech model**: `ggml-large-v3-turbo.bin` (about 1.6 GB), downloaded once
  after the user agrees to a one-time prompt, verified by SHA-256, and kept
  in `~/Library/Application Support/ballast/models/`.
- **Work in progress**: each video's temporary audio and transcript live in
  `~/Library/Application Support/ballast/captions/<job id>/` and are
  deleted when its captions finish, fail, or are cancelled.

## Automatic video summaries

Once a video has captions, Ballast writes a summary of it on the Mac, for
free: an overview, the main points, quotes with times (each checked
against the transcript), any references and calls to action the speaker
gave, a suggested title and description, and a message ready to paste into
a group chat. A Markdown copy is saved next to the original video, and a
Google Doc is put next to the video in Drive once the video has arrived.
Doubtful references are listed under "Check before sharing". See
`specs/006-video-summaries/`.

- **Engine**: [llama.cpp](https://github.com/ggml-org/llama.cpp)'s
  `llama-server`, run on `127.0.0.1` only. It's built by the same
  `scripts/build-engines.sh`. For `wails dev`:

  ```sh
  brew install llama.cpp
  export BALLAST_LLAMA_SERVER=/opt/homebrew/bin/llama-server
  ```

- **Model**: a free open model of about 2.5 GB, downloaded once after a
  one-time prompt, verified by SHA-256, and kept in
  `~/Library/Application Support/ballast/models/`.
- Only one AI model runs at a time: summaries wait for captions to finish.

## Development Process

This project is built solo, with AI coding agents, using
[GitHub's Spec Kit](https://github.com/github/spec-kit) for spec-driven
development. Every feature goes through:

```
/speckit-constitution   → project principles & non-negotiables (.specify/memory/constitution.md)
/speckit-specify        → feature spec, WHAT and WHY (specs/<NNN-feature>/spec.md)
/speckit-clarify         → resolve ambiguities before planning (if any remain)
/speckit-plan            → technical approach & design (specs/<NNN-feature>/plan.md)
/speckit-tasks           → dependency-ordered task breakdown (specs/<NNN-feature>/tasks.md)
/speckit-analyze         → cross-check spec/plan/tasks consistency
/speckit-taskstoissues   → turn tasks into GitHub issues
/speckit-implement       → execute, one PR per task
```

The project constitution (`.specify/memory/constitution.md`) governs every
plan and spec — notably: Go/Wails/SQLite only unless justified, no violating
Google Drive's single-sequential-stream upload protocol, test-first for the
upload engine's session/retry/resume logic, and encryption-at-rest for all
stored credentials.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

[MIT](LICENSE)
