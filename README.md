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
