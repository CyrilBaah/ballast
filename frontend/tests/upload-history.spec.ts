import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers quickstart.md Scenario 3 (User Story 3): the history screen lists
// past and current uploads, updating live as upload:* events arrive, and
// survives an app restart. Google's OAuth/Drive API are mocked at the
// network boundary (mock_e2e.go) via BALLAST_E2E_MOCK=1, same as the other
// E2E specs.
const outcomeFile =
  process.env.BALLAST_E2E_OUTCOME_FILE ?? `${__dirname}/.e2e-outcome`;

function setOutcome(outcome: string) {
  fs.writeFileSync(outcomeFile, outcome);
}

async function waitForBindings(page: Page) {
  await page.waitForFunction(() => !!(window as any).go?.main?.App, undefined, {
    timeout: 10_000,
  });
}

async function signIn(page: Page) {
  setOutcome('approve');
  await page.goto('/');
  await waitForBindings(page);
  await page.evaluate(() => (window as any).go.main.App.AuthSignOut().catch(() => {}));
  await page.reload();
  await waitForBindings(page);
  await page.click('#signin-btn');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
}

async function stubFilePicker(page: Page, file: { path: string; name: string; sizeBytes: number }) {
  await page.evaluate((f) => {
    (window as any).go.main.App.FilesPickLocal = () => Promise.resolve(f);
  }, file);
}

function makeTempFile(name: string, sizeBytes: number): { path: string; name: string; sizeBytes: number } {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ballast-e2e-'));
  const filePath = path.join(dir, name);
  fs.writeFileSync(filePath, 'x'.repeat(sizeBytes));
  const stat = fs.statSync(filePath);
  return { path: filePath, name, sizeBytes: stat.size };
}

// Upload.ListRecent reads the same local SQLite file across separate test
// runs (it isn't wiped between `wails dev` restarts), so a fixed filename
// re-used by this spec on a second run would collide with its own earlier
// row and break the hasText lookups below with a strict-mode multi-match.
// A unique suffix per run keeps each row identifiable while the "history-*"
// prefix stays stable enough for hasText to distinguish rows within one run.
function uniqueFileName(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.txt`;
}

test.afterEach(() => {
  setOutcome('approve');
});

test('history shows a completed upload and a second upload updating live, without leaving the app (Acceptance Scenario 1 & 2, SC-003)', async ({
  page,
}) => {
  await signIn(page);

  const name1 = uniqueFileName('history-one');
  const file1 = makeTempFile(name1, 4_000);
  await stubFilePicker(page, file1);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  await expect(page.locator('.progress-result')).toContainText('Upload complete', { timeout: 15_000 });

  await page.click('#nav-upload');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
  const name2 = uniqueFileName('history-two');
  const file2 = makeTempFile(name2, 4_000);
  await stubFilePicker(page, file2);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await page.click('#nav-history');
  await expect(page.locator('.history-screen')).toBeVisible({ timeout: 10_000 });

  // Asserts on these two rows specifically, not the list's total count --
  // Upload.ListRecent deliberately returns every upload in the shared local
  // DB (data-model.md), so other spec files' uploads may also be present.
  const row1 = page.locator('.history-row', { hasText: name1 });
  await expect(row1).toBeVisible({ timeout: 10_000 });
  await expect(row1.locator('.history-row-status')).toHaveClass(/state-success/);

  const row2 = page.locator('.history-row', { hasText: name2 });
  // It updates live as upload:progress/upload:complete arrive, without
  // ever needing to leave the history screen or re-fetch.
  await expect(row2.locator('.history-row-status')).toHaveClass(/state-success/, { timeout: 15_000 });
});

test('a forced terminal failure is clearly marked, visually distinct from a succeeded row (Acceptance Scenario 3)', async ({
  page,
}) => {
  await signIn(page);

  const okName = uniqueFileName('history-ok');
  const file1 = makeTempFile(okName, 4_000);
  await stubFilePicker(page, file1);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  await expect(page.locator('.progress-result')).toContainText('Upload complete', { timeout: 15_000 });

  await page.click('#nav-upload');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
  const failName = uniqueFileName('history-fail');
  const file2 = makeTempFile(failName, 4_000);
  await stubFilePicker(page, file2);
  await page.click('#pick-file-btn');
  setOutcome('403-quota');
  await page.click('#upload-btn');

  await page.click('#nav-history');
  await expect(page.locator('.history-screen')).toBeVisible({ timeout: 10_000 });

  const failedRow = page.locator('.history-row', { hasText: failName });
  await expect(failedRow.locator('.history-row-status')).toHaveClass(/state-error/, { timeout: 15_000 });
  await expect(failedRow.locator('.history-row-status')).toContainText('Failed');

  const okRow = page.locator('.history-row', { hasText: okName });
  await expect(okRow.locator('.history-row-status')).toHaveClass(/state-success/);
});

test("an upload's status persists across a restart while it's paused (Edge Case: crash-safety, Feature 002)", async ({
  page,
}) => {
  await signIn(page);
  setOutcome('network-fail');
  const restartName = uniqueFileName('history-restart');
  const file = makeTempFile(restartName, 4_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  await expect(page.locator('.progress-result')).toContainText('retrying', { timeout: 15_000 });

  // Simulate the process dying and a fresh launch reopening the same DB
  // file (same DebugRestart mechanism as upload-recovery.spec.ts).
  await page.evaluate(() => (window as any).go.main.App.DebugRestart());
  await page.reload();
  await waitForBindings(page);

  // GetRecoverable routes straight back to the progress screen for the
  // still-paused upload; from there, History is reachable via its nav item.
  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  await page.click('#nav-history');
  await expect(page.locator('.history-screen')).toBeVisible({ timeout: 10_000 });

  const row = page.locator('.history-row', { hasText: restartName });
  await expect(row.locator('.history-row-status')).toHaveClass(/state-warning/, { timeout: 10_000 });
});
