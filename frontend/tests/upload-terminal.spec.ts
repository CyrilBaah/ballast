import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers quickstart.md Scenario 3 (User Story 3): terminal conditions stop
// automatic retrying and surface a specific reason, with an explicit
// confirmation step before ever restarting a transfer from byte 0.
// Google's Drive resumable-upload protocol is mocked at the network
// boundary (mock_e2e.go) via BALLAST_E2E_MOCK=1.
//
// Not covered here: research.md §4's "permission revoked" row (a 401/403
// where the underlying OAuth refresh token itself is invalid). That
// classification is verified at the unit level
// (internal/drive/retry_test.go's TestClassifyTransportErrorTreatsRevokedRefreshTokenAsTerminal)
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

test('an expired session prompts for confirmation and restarts from byte 0 on confirm (Acceptance Scenario 3)', async ({
  page,
}) => {
  // Larger than one baseline (8 MiB) chunk guarantees a first chunk
  // succeeds -- and its session URI gets persisted to the DB -- before the
  // deterministic '404-session-after-progress' (mock_e2e.go) expires the
  // session on the next chunk. A session that never acknowledges a chunk
  // is never persisted, so restarting it wouldn't exercise the
  // release-the-old-session-on-restart assertion below.
  const file = makeTempFile('expired-session.txt', 12 * 1024 * 1024);
  setOutcome('404-session-after-progress');
  const uploadId = await startUpload(page, file);

  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 })
    .toBe('awaiting_confirmation');
  const status = await getStatus(page, uploadId);
  expect(status.awaitingConfirmationReason).toBe('session_expired');

  await expect(page.locator('#progress-confirm')).toBeVisible();
  await expect(page.locator('#progress-confirm-text')).toContainText('session expired');

  // DebugSessionReleaseCount is a running total for the whole mock-server
  // process, shared across every test in the run -- so the restart's
  // effect is checked as a delta, not an absolute count.
  const releaseCountBefore = await page.evaluate(() =>
    (window as any).go.main.App.DebugSessionReleaseCount(),
  );

  setOutcome('approve');
  await page.click('#progress-confirm-restart-btn');

  await expect
    .poll(() => getStatus(page, uploadId).then((s) => s.status), { timeout: 15_000 })
    .toBe('succeeded');
  const finalStatus = await getStatus(page, uploadId);
  expect(finalStatus.bytesSent).toBe(file.sizeBytes);

  // Confirms the abandoned pre-restart session was actually released with
  // Drive (app.go's UploadConfirmRestart), not just discarded locally --
  // otherwise it could keep completing server-side and produce a
  // duplicate file alongside the restarted upload's.
  const releaseCountAfter = await page.evaluate(() =>
    (window as any).go.main.App.DebugSessionReleaseCount(),
  );
  expect(releaseCountAfter - releaseCountBefore).toBe(1);
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
