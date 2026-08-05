import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Google's OAuth/Drive API are mocked at the network boundary
// (mock_e2e.go) via BALLAST_E2E_MOCK=1. "slow-list"/"500-list" (T001) and
// "signin-fail" trigger the picker's loading/error states and a genuine
// sign-in failure on demand, rather than depending on real network timing.
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

async function resetToSignedOut(page: Page) {
  setOutcome('approve');
  await page.goto('/');
  await waitForBindings(page);
  await page.evaluate(() => (window as any).go.main.App.AuthSignOut().catch(() => {}));
  await page.reload();
  await waitForBindings(page);
}

// outcomeDuringSignIn is applied *after* resetToSignedOut's own internal
// 'approve' reset and *before* the sign-in click, so it's still in effect
// for the picker's automatic on-mount folder load -- setting it only
// before calling signIn() would get clobbered by resetToSignedOut.
async function signIn(page: Page, outcomeDuringSignIn: string = 'approve') {
  await resetToSignedOut(page);
  setOutcome(outcomeDuringSignIn);
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

test.afterEach(() => {
  setOutcome('approve');
});

// --- User Story 2: picker loading/error states -----------------------

test("the picker shows a loading indicator while folders are loading, not a blank list (Acceptance Scenario 1)", async ({
  page,
}) => {
  await signIn(page, 'slow-list');

  const loading = page.locator('#picker-folder-loading');
  await expect(loading).toBeVisible();
  await expect(loading).toHaveClass(/state-loading/);

  // The 1.5s mock delay (mock_e2e.go's "slow-list" outcome) resolves and
  // the indicator clears once folders arrive.
  await expect(loading).toBeHidden({ timeout: 10_000 });
});

test('a folder-load failure renders the shared error-state treatment with a plain-language message (Acceptance Scenario 2)', async ({
  page,
}) => {
  await signIn(page, '500-list');

  const error = page.locator('#picker-folder-error');
  await expect(error).toHaveClass(/state-error/, { timeout: 10_000 });
  await expect(error).not.toHaveText('');
});

// --- User Story 2: sign-in / upload state treatments -------------------

test('a sign-in failure renders the shared error-state treatment with plain-language text, not a raw error string (FR-003, Acceptance Scenario 2)', async ({
  page,
}) => {
  await resetToSignedOut(page);
  setOutcome('signin-fail');
  await page.click('#signin-btn');

  const error = page.locator('#signin-error');
  await expect(error).toHaveClass(/state-error/, { timeout: 10_000 });
  await expect(error).not.toHaveText('');
  const text = await error.textContent();
  expect(text?.toLowerCase()).not.toContain('invalid_grant');
});

test('an upload failure renders the shared error-state treatment, distinct from success/warning (Acceptance Scenario 2)', async ({
  page,
}) => {
  await signIn(page);
  const file = makeTempFile('state-feedback-fail.txt', 4_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');

  setOutcome('403-quota');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  const result = page.locator('.progress-result');
  await expect(result).toHaveClass(/state-error/, { timeout: 15_000 });
  await expect(result).not.toHaveClass(/state-success/);
  await expect(result).not.toHaveClass(/state-warning/);
  await expect(result).toContainText('Upload failed');
});

test('a successful upload renders an unambiguous success state, visually distinct from in-progress (Acceptance Scenario 3)', async ({
  page,
}) => {
  await signIn(page);
  const file = makeTempFile('state-feedback-success.txt', 4_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  const result = page.locator('.progress-result');
  await expect(result).toHaveClass(/state-success/, { timeout: 15_000 });
  await expect(result).not.toHaveClass(/state-error/);
  await expect(result).not.toHaveClass(/state-warning/);
});

test('a dropped connection mid-upload shows an active/recovering treatment, not one identical to a hard failure (Acceptance Scenario 4)', async ({
  page,
}) => {
  await signIn(page);
  setOutcome('network-fail');
  const file = makeTempFile('state-feedback-retry.txt', 4_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  const result = page.locator('.progress-result');
  await expect(result).toHaveClass(/state-warning/, { timeout: 15_000 });
  await expect(result).not.toHaveClass(/state-error/);
  await expect(result).not.toHaveClass(/state-success/);

  setOutcome('approve');
  await expect(result).toHaveClass(/state-success/, { timeout: 15_000 });
});
