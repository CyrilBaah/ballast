import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Google's OAuth and Drive API are mocked at the network boundary
// (mock_e2e.go) via BALLAST_E2E_MOCK=1, same as upload-flow.spec.ts.
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
// automate, so we stub the binding instead (same approach as upload-flow.spec.ts).
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

// Installs an upload:progress listener before the upload starts, recording
// every payload to a window-level array so the test can assert on cadence and monotonicity after the fact.
async function captureProgressEvents(page: Page) {
  await page.evaluate(() => {
    (window as any).__progressEvents = [];
    (window as any).runtime.EventsOn('upload:progress', (payload: unknown) => {
      (window as any).__progressEvents.push(payload);
    });
  });
}

async function getProgressEvents(page: Page): Promise<Array<{ id: number; bytesSent: number; totalBytes: number }>> {
  return page.evaluate(() => (window as any).__progressEvents ?? []);
}

test.beforeEach(async ({ page }) => {
  setOutcome('approve');
  await signIn(page);
});

test.afterEach(() => {
  setOutcome('approve');
});

test('a successful upload reports non-decreasing progress and a terminal success with a Drive link (Acceptance Scenario 1 & 2, SC-004, FR-008)', async ({
  page,
}) => {
  await captureProgressEvents(page);

  const file = makeTempFile('progress-me.txt', 5_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.progress-result')).toContainText('Upload complete', {
    timeout: 15_000,
  });

  const link = page.locator('.progress-result a');
  await expect(link).toHaveAttribute('href', /^https:\/\/.+/);

  const events = await getProgressEvents(page);
  expect(events.length).toBeGreaterThan(0);

  let prevBytes = -1;
  for (const event of events) {
    expect(event.bytesSent).toBeGreaterThanOrEqual(prevBytes);
    prevBytes = event.bytesSent;
  }
  // The final progress report must reflect the true total byte count.
  expect(events[events.length - 1].bytesSent).toBe(file.sizeBytes);
});

test('network loss mid-upload shows a retrying state, never a failure, and completes once connectivity returns (Feature 002, FR-003/FR-007)', async ({
  page,
}) => {
  setOutcome('network-fail');
  const file = makeTempFile('progress-fail.txt', 5_000);
  await stubFilePicker(page, file);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');

  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.progress-result')).toContainText('retrying', {
    timeout: 15_000,
  });
  await expect(page.locator('.progress-result')).toHaveClass(/state-warning/);
  await expect(page.locator('.progress-result')).not.toHaveClass(/state-error/);

  setOutcome('approve');
  await expect(page.locator('.progress-result')).toContainText('Upload complete', {
    timeout: 15_000,
  });
});
