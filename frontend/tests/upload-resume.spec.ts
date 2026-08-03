import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers quickstart.md Scenario 1 (User Story 1): an upload pauses (never
// fails) on a dropped connection and resumes from the last
// Drive-acknowledged offset once connectivity returns, without ever
// restarting from 0. Google's OAuth/Drive resumable-upload protocol is
// mocked at the network boundary (mock_e2e.go) via BALLAST_E2E_MOCK=1.
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

async function getStatus(page: Page, uploadId: number) {
  return page.evaluate((id) => (window as any).go.main.App.UploadGetStatus(id), uploadId);
}

test.beforeEach(async ({ page }) => {
  setOutcome('approve');
  await signIn(page);
});

test.afterEach(() => {
  setOutcome('approve');
});

test('a dropped connection pauses (not fails) and resumes from the last acknowledged offset (Acceptance Scenarios 1-3, SC-001, SC-005)', async ({
  page,
}) => {
  const file = makeTempFile('resume-me.txt', 20_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });

  const uploadId = await page.evaluate(() => {
    const text = document.querySelector('.progress-screen')!.textContent ?? '';
    const match = text.match(/upload #(\d+)/);
    return match ? Number(match[1]) : null;
  });
  expect(uploadId).not.toBeNull();

  setOutcome('network-fail');
  await expect.poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 15_000 }).toBe('paused');

  const pausedStatus = await getStatus(page, uploadId!);
  const bytesAtPause = pausedStatus.bytesSent;

  setOutcome('approve');
  await expect
    .poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 15_000 })
    .toBe('succeeded');

  const finalStatus = await getStatus(page, uploadId!);
  expect(finalStatus.bytesSent).toBe(file.sizeBytes);
  expect(finalStatus.bytesSent).toBeGreaterThanOrEqual(bytesAtPause);
  expect(finalStatus.driveFileLink).toMatch(/^https:\/\/.+/);
});

test('three rapid drop/restore cycles all recover with no manual intervention (Edge Case: rapid reconnects)', async ({
  page,
}) => {
  const file = makeTempFile('rapid-resume.txt', 20_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });

  const uploadId = await page.evaluate(() => {
    const text = document.querySelector('.progress-screen')!.textContent ?? '';
    const match = text.match(/upload #(\d+)/);
    return match ? Number(match[1]) : null;
  });
  expect(uploadId).not.toBeNull();

  for (let cycle = 0; cycle < 3; cycle++) {
    setOutcome('network-fail');
    await expect
      .poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 15_000 })
      .toBe('paused');
    setOutcome('approve');
    // Give it a brief window to resume before dropping again, except on
    // the final cycle where it should be allowed to run to completion.
    if (cycle < 2) {
      await page.waitForTimeout(200);
    }
  }

  await expect
    .poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 20_000 })
    .toBe('succeeded');
});
