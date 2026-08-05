#!/usr/bin/env bash
# Start Ballast in dev mode: builds the Go backend, starts the frontend
# dev server, and opens the native window.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! command -v wails >/dev/null 2>&1; then
  export PATH="$(go env GOPATH)/bin:$PATH"
fi

if ! command -v wails >/dev/null 2>&1; then
  echo "wails CLI not found. Install it with:"
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
  exit 1
fi

# Local, gitignored file holding real OAuth credentials (see .gitignore).
# Only lines anchored to `export VAR=` are evaluated, so it's safe even if
# the file also has comments/commands mentioning these var names elsewhere.
if [[ -z "${BALLAST_GOOGLE_CLIENT_ID:-}" && -f export ]]; then
  eval "$(grep -E '^export BALLAST_GOOGLE_(CLIENT_ID|CLIENT_SECRET)=' export)"
fi

if [[ -z "${BALLAST_GOOGLE_CLIENT_ID:-}" || -z "${BALLAST_GOOGLE_CLIENT_SECRET:-}" ]]; then
  echo "Note: BALLAST_GOOGLE_CLIENT_ID/SECRET are not set — Google sign-in will fail." >&2
  echo "Set them first if you want to exercise sign-in and upload." >&2
fi

exec wails dev
