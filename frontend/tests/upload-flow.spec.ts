import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Google's Drive API is mocked at the network boundary (mock_e2e.go) via
// BALLAST_E2E_MOCK=1: Files.List always reports an empty folder list, and
// Files.Create either succeeds or, when the outcome file is set to
// "network-fail", drops the connection to simulate lost connectivity.
const outcomeFile =
  process.env.BALLAST_E2E_OUTCOME_FILE ?? `${__dirname}/.e2e-outcome`;

function setOutcome(outcome: 'approve' | 'deny' | 'network-fail') {
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

// Files.PickLocal drives a native OS dialog, which Playwright cannot
// automate, so we stub `window.go.main.App.FilesPickLocal` and exercise the
// rest of the real upload flow unmodified.
async function stubFilePicker(page: Page, file: { path: string; name: string; sizeBytes: number }) {
  await page.evaluate((f) => {
    (window as any).go.main.App.FilesPickLocal = () => Promise.resolve(f);
  }, file);
}

function makeTempFile(name: string, contents: string): { path: string; name: string; sizeBytes: number } {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ballast-e2e-'));
  const filePath = path.join(dir, name);
  fs.writeFileSync(filePath, contents);
  const stat = fs.statSync(filePath);
  return { path: filePath, name, sizeBytes: stat.size };
}

test.beforeEach(async ({ page }) => {
  setOutcome('approve');
  await signIn(page);
});

test('pick a file, list folders (My Drive only), and start an upload (Acceptance Scenario 1 & 2)', async ({
  page,
}) => {
  await expect(page.locator('#picker-breadcrumb')).toContainText('My Drive', {
    timeout: 10_000,
  });
  await expect(page.locator('.picker-folder-empty')).toContainText('No sub-folders');

  const file = makeTempFile('upload-me.txt', 'hello ballast');
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await expect(page.locator('#picked-file')).toContainText('upload-me.txt');

  await page.click('#upload-btn');
  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.progress-screen')).toContainText('upload-me.txt');
});

test('a file that vanishes before upload starts is rejected with a clear message (Acceptance Scenario 3)', async ({
  page,
}) => {
  const file = makeTempFile('will-vanish.txt', 'temporary');
  fs.rmSync(file.path);

  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await expect(page.locator('#picked-file')).toContainText('will-vanish.txt');

  await page.click('#upload-btn');
  await expect(page.locator('#upload-error')).not.toHaveText('', { timeout: 10_000 });
  // Upload.Start must reject before creating any Upload row.
  await expect(page.locator('.progress-screen')).toHaveCount(0);
});

test('network loss mid-upload ends in a failed status, not a hang (Acceptance Scenario 4)', async ({
  page,
}) => {
  setOutcome('network-fail');
  const file = makeTempFile('big-upload.txt', 'x'.repeat(1024));
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });

  // Poll Upload.GetStatus until it reaches a terminal state.
  const uploadId = await page.evaluate(async () => {
    const text = document.querySelector('.progress-screen')!.textContent ?? '';
    const match = text.match(/upload #(\d+)/);
    return match ? Number(match[1]) : null;
  });
  expect(uploadId).not.toBeNull();

  await expect
    .poll(
      async () => {
        const status = await page.evaluate(
          (id) => (window as any).go.main.App.UploadGetStatus(id),
          uploadId,
        );
        return status.status;
      },
      { timeout: 15_000 },
    )
    .toBe('failed');

  setOutcome('approve');
});
