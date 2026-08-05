import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers quickstart.md Scenario 2 (User Story 2): an upload interrupted by
// the app/process dying is detected on next launch and resumes from its
// last persisted checkpoint. A real OS-level kill+relaunch can't be driven
// from inside a single Playwright run against one shared `wails dev`
// server, so this spec simulates "the process died" via App.DebugRestart
// (E2E-mock-only) -- it discards the in-memory App state (DB connection,
// auth token cache, and cancels the in-flight upload goroutine) and
// reopens a fresh handle against the *same* on-disk SQLite file. That is
// exactly the mechanism this feature actually relies on for crash
// recovery (Constitution Principle VII: SQLite's own durability plus
// startup-time detection, not any OS crash-reporting API), so it's a
// faithful test of the real recovery path -- just without an actual
// process kill/relaunch. Because the implementation treats a still
// in_progress row and a cleanly-paused row identically at startup
// (data-model.md's startup-normalization rule), this one mechanism covers
// both "force-killed" and "gracefully quit" restarts; there is no
// separate code path to exercise twice.
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

async function debugRestart(page: Page) {
  return page.evaluate(() => (window as any).go.main.App.DebugRestart());
}

async function getRecoverable(page: Page) {
  return page.evaluate(() => (window as any).go.main.App.UploadGetRecoverable());
}

test.afterEach(() => {
  setOutcome('approve');
});

test('an upload interrupted by a process restart is detected and resumes from its last checkpoint (Acceptance Scenarios 1-3, SC-002)', async ({
  page,
}) => {
  await signIn(page);

  // Larger than one baseline (8 MiB) chunk, so there's a real first chunk
  // to succeed before the second one is failed -- 'network-fail-after-progress'
  // (mock_e2e.go) fails deterministically only once some progress exists,
  // rather than racing setOutcome() against how fast a single tiny request completes.
  const file = makeTempFile('recover-me.txt', 12 * 1024 * 1024);
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

  // Pause it mid-transfer so there's a persisted session_uri/bytes_sent
  // checkpoint before the simulated crash (quickstart.md Scenario 2 setup).
  setOutcome('network-fail-after-progress');
  await expect.poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 15_000 }).toBe('paused');
  const beforeRestart = await getStatus(page, uploadId!);
  expect(beforeRestart.bytesSent).toBeGreaterThan(0);

  // Simulate the process dying and a fresh launch reopening the same DB file.
  await debugRestart(page);

  // The new "process" must detect the interrupted upload with no lost progress.
  const recovered = await getRecoverable(page);
  expect(recovered).not.toBeNull();
  expect(recovered.id).toBe(uploadId);
  expect(recovered.bytesSent).toBe(beforeRestart.bytesSent);
  expect(['in_progress', 'paused']).toContain(recovered.status);

  // Connectivity restored -- the backend already kicked off resuming it
  // in the background (research.md §7); it must complete without ever
  // having restarted from 0.
  setOutcome('approve');
  await expect
    .poll(() => getStatus(page, uploadId!).then((s) => s.status), { timeout: 15_000 })
    .toBe('succeeded');
  const finalStatus = await getStatus(page, uploadId!);
  expect(finalStatus.bytesSent).toBe(file.sizeBytes);
});
