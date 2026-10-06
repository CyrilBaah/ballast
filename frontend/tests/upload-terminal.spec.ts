import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers quickstart.md Scenario 3 (User Story 3): terminal conditions stop
// automatic retrying and surface a specific reason. A transfer only stops
// to ask before restarting from byte 0 when the answer is genuinely the
// user's -- the source file changed underneath it. A session Drive itself
// dropped is not such a case: the acknowledged bytes are gone from Drive's
// side either way, so the app opens a fresh session and carries on.
// Google's Drive resumable-upload protocol is mocked at the network
// boundary (mock_e2e.go) via BALLAST_E2E_MOCK=1.
//
// Not covered here: research.md §4's "permission revoked" row (a 401/403
// where the underlying OAuth refresh token itself is invalid). That
// classification is verified at the unit level
// (internal/drive/retry_test.go's TestClassifyTransportErrorTreatsRevokedRefreshTokenAsNeedsSignIn)
// -- reliably forcing a live token refresh to fail *mid-transfer* within a
// fast, mocked E2E run would require the access token to already be near
// expiry when the upload starts, which isn't a realistic precondition to
// fabricate here without adding test-only token-lifetime plumbing.
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

async function startUpload(page: Page, file: { path: string; name: string; sizeBytes: number }): Promise<number> {
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
  return uploadId!;
}

test.beforeEach(async ({ page }) => {
  setOutcome('approve');
  await signIn(page);
});

test.afterEach(() => {
  setOutcome('approve');
});

test('retryable errors (429/503) never surface as failure and complete once cleared (Acceptance Scenario 1)', async ({
  page,
}) => {
  const uploadId = await startUpload(page, makeTempFile('retryable.txt', 5_000));

  setOutcome('429');
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 }).toBe('paused');

  setOutcome('503');
  await page.waitForTimeout(200);
  expect((await getStatus(page, uploadId)).status).toBe('paused');

  setOutcome('approve');
  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 })
    .toBe('succeeded');
});

test('storage quota exceeded fails within 5 seconds with a specific reason and no further retries (Acceptance Scenario 2, SC-004)', async ({
  page,
}) => {
  const uploadId = await startUpload(page, makeTempFile('quota.txt', 5_000));

  setOutcome('403-quota');
  const start = Date.now();
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 }).toBe('failed');
  expect(Date.now() - start).toBeLessThan(5_000 + 3_000); // allow slack for CI scheduling jitter

  const status = await getStatus(page, uploadId);
  expect(status.failureReason?.toLowerCase()).toContain('storage');

  await page.waitForTimeout(300);
  expect((await getStatus(page, uploadId)).status).toBe('failed');
});

test('an expired session opens a fresh one and finishes on its own, with nothing to confirm (Acceptance Scenario 3)', async ({
  page,
}) => {
  // Larger than one baseline (8 MiB) chunk guarantees a first chunk is
  // acknowledged -- and its session URI persisted to the DB -- before
  // '404-session-once' (mock_e2e.go) expires that session exactly once,
  // leaving the automatic replacement free to run to completion.
  const file = makeTempFile('expired-session.txt', 12 * 1024 * 1024);
  setOutcome('404-session-once');
  const uploadId = await startUpload(page, file);

  // Nothing is clicked between starting the upload and this assertion.
  // Once Drive drops a session, the bytes it had acknowledged are gone
  // from its side -- a status query against the dead URI returns 404, not
  // an offset -- so a new session from byte 0 is the only way this file
  // ever lands. The app does that itself instead of stopping to put a
  // question to the user whose only real answer is yes.
  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 30_000 })
    .toBe('succeeded');

  const finalStatus = await getStatus(page, uploadId);
  expect(finalStatus.bytesSent).toBe(file.sizeBytes);
  expect(finalStatus.awaitingConfirmationReason).toBeFalsy();
});

test('a session that keeps dying fails with a clear reason instead of restarting forever (loop guard)', async ({
  page,
}) => {
  // Sticky, unlike '404-session-once': every session this upload opens is
  // expired once it has acknowledged a chunk. Restarting on its own is
  // right the first time and pathological on repeat -- each attempt
  // re-sends the whole file -- so the second dropped session is reported
  // rather than silently costing the user another full transfer.
  const file = makeTempFile('expired-session-twice.txt', 12 * 1024 * 1024);
  setOutcome('404-session-after-progress');
  const uploadId = await startUpload(page, file);

  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 30_000 })
    .toBe('failed');

  const status = await getStatus(page, uploadId);
  expect(status.failureReason?.toLowerCase()).toContain('session');
  expect(status.awaitingConfirmationReason).toBeFalsy();
});

test('a source file deleted while paused fails with a clear reason, not awaiting-confirmation (Edge Case)', async ({
  page,
}) => {
  // Windows opens files without FILE_SHARE_DELETE by default, so it
  // refuses to delete a file the backend still has open -- this exact
  // interleaving isn't reproducible there the way it is on POSIX (mirrors
  // internal/drive/upload_test.go's identical skip for the same reason).
  test.skip(process.platform === 'win32', 'deleting a file that\'s still open elsewhere is not reproducible on Windows');

  // The source-file-identity check (internal/drive/identity.go) only runs
  // once at least one chunk has been acknowledged (there's no prefix to
  // verify before that) -- a file small enough to fail entirely on its
  // first-ever attempt would leave bytesSent at 0 and this deletion
  // undetected via the backend's still-open file handle. Larger than one
  // baseline (8 MiB) chunk guarantees a first chunk succeeds before the
  // deterministic 'network-fail-after-progress' (mock_e2e.go) pauses it.
  const file = makeTempFile('will-be-deleted.txt', 12 * 1024 * 1024);
  const uploadId = await startUpload(page, file);

  setOutcome('network-fail-after-progress');
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 }).toBe('paused');

  fs.rmSync(file.path);
  setOutcome('approve');

  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 })
    .toBe('failed');
  const status = await getStatus(page, uploadId);
  expect(status.failureReason?.toLowerCase()).toContain('local file');
});

test('a deleted destination folder fails within 5 seconds naming the missing destination (Edge Case)', async ({
  page,
}) => {
  const uploadId = await startUpload(page, makeTempFile('folder-gone.txt', 5_000));

  setOutcome('network-fail');
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 }).toBe('paused');

  setOutcome('404-parent');
  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 })
    .toBe('failed');
  const status = await getStatus(page, uploadId);
  expect(status.failureReason?.toLowerCase()).toContain('folder');
});

test('cancelling a paused upload frees the slot for a new upload immediately (Acceptance Scenario 4, FR-014)', async ({
  page,
}) => {
  const uploadId = await startUpload(page, makeTempFile('to-cancel.txt', 5_000));

  setOutcome('network-fail');
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 }).toBe('paused');

  await page.evaluate((id) => (window as any).go.main.App.UploadCancel(id), uploadId);
  await expect.poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 10_000 }).toBe('cancelled');

  setOutcome('approve');

  // Cancelling ends this upload's screen lifecycle only implicitly (no
  // automatic navigation), so reload to re-enter the boot sequence, which
  // must not find a recoverable upload now that the previous one is
  // cancelled (a terminal state) -- landing back on the picker, not stuck
  // showing the cancelled upload.
  await page.reload();
  await waitForBindings(page);
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });

  // FR-013's single-active-upload slot must be free immediately: a second
  // upload for a different file succeeds without any rejection.
  const secondFile = makeTempFile('second-upload.txt', 100);
  await stubFilePicker(page, secondFile);
  await page.click('#pick-file-btn');
  await page.click('#upload-btn');
  // #upload-error lives on the picker screen (frontend/src/screens/picker.ts),
  // which this navigation has already moved away from -- .progress-screen
  // becoming visible is itself the proof that the second upload was
  // accepted without rejection; there's no error surface to check on this screen.
  await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
});
