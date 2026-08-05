#!/usr/bin/env bash
# Run Ballast's test suites: Go unit tests, then Playwright e2e (mocked Google/Drive).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

go test ./...

cd frontend
npm ci
npx playwright install --with-deps chromium
BALLAST_E2E_MOCK=1 npm test
