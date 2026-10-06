import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers Feature 005's UI contract (contracts/wails-bindings.md): the
// one-time speech-model prompt, a caption line per phase kept separate
// from the upload's own status, and the caption file's detail actions.
//
// It runs against the redesigned UI (src/ui/) with Google mocked at the
// network boundary (BALLAST_E2E_MOCK=1). The real speech engine never
// runs here: caption states are delivered through the same Wails event
// channel the captions worker uses ("captions:updated",
// "captions:consent-needed"), emitted from the page, so the UI is tested
// exactly as it receives them. The worker itself is covered by
// internal/captions' Go tests.
const outcomeFile = process.env.BALLAST_E2E_OUTCOME_FILE ?? `${__dirname}/.e2e-outcome`;

async function app(page: Page) {
  await page.waitForFunction(() => !!(window as any).go?.main?.App, undefined, { timeout: 10_000 });
}

async function signInAndUpload(page: Page, name: string): Promise<number> {
  fs.writeFileSync(outcomeFile, 'approve');
  await page.goto('/');
  await app(page);
  await page.evaluate(() => (window as any).go.main.App.AuthSignIn());
  await page.reload();
  await app(page);
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ballast-e2e-'));
  const file = path.join(dir, name);
  fs.writeFileSync(file, 'x'.repeat(2048));
  const id = await page.evaluate((f) => (window as any).go.main.App.UploadStart(f, 'root', 'My Drive'), file);
  await page.reload();
  await app(page);
  await page.click('[data-nav="transfers"]');
  await page.click('[data-filter="all"]');
  return id as number;
}

async function emit(page: Page, name: string, payload: unknown) {
  await page.evaluate(([n, p]) => (window as any).runtime.EventsEmit(n, p), [name, payload] as const);
}

function captionJob(uploadId: number, over: Record<string, unknown>) {
  return { uploadId, status: 'in_progress', progressPercent: 0, language: 'en', ...over };
}

test.afterEach(() => fs.writeFileSync(outcomeFile, 'approve'));

test('the speech-model prompt shows once and answering it hides it', async ({ page }) => {
  await signInAndUpload(page, `prompt-${Date.now()}.mp4`);
  await emit(page, 'captions:consent-needed', { modelSizeBytes: 1_624_555_275 });
  const banner = page.locator('[data-caption-consent-banner]');
  await expect(banner).toBeVisible();
  await expect(banner).toContainText('1.6 GB');
  await page.click('[data-caption-consent="no"]');
  await expect(banner).toHaveCount(0);
});

test('each caption phase shows its own line under the upload', async ({ page }) => {
  const name = `phases-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  const card = page.locator('.transfer-card', { hasText: name });
  const line = card.locator('[data-caption-line]');

  const steps: [Record<string, unknown>, string][] = [
    [{ status: 'waiting', phase: 'awaiting_consent' }, 'Captions: waiting for your answer'],
    [{ phase: 'downloading_model', progressPercent: 40 }, 'Captions: downloading speech model — 40%'],
    [{ phase: 'transcribing', progressPercent: 42 }, 'Captions: transcribing — 42%'],
    [{ phase: 'waiting_for_video', localCopyPath: '/tmp/x.srt' }, 'Captions ready on this Mac — waiting for the video to finish'],
    [{ status: 'done', driveFileLink: 'https://drive/x', localCopyPath: '/tmp/x.srt' }, 'Captions ready'],
  ];
  for (const [over, text] of steps) {
    await emit(page, 'captions:updated', captionJob(id, over));
    await expect(line).toHaveText(text);
  }
});

test('a failed caption shows its reason without changing the upload status', async ({ page }) => {
  const name = `failed-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  const card = page.locator('.transfer-card', { hasText: name });
  const chipBefore = await card.locator('.status-chip').textContent();
  await emit(page, 'captions:updated', captionJob(id, { status: 'failed', note: 'This video has no audio track' }));
  await expect(card.locator('[data-caption-line]')).toHaveText("Captions couldn't be made — This video has no audio track");
  await expect(card.locator('.status-chip')).toHaveText(chipBefore ?? '');
});

test('the detail panel offers the caption file in Drive and in Finder', async ({ page }) => {
  const name = `detail-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  await emit(page, 'captions:updated', captionJob(id, { status: 'done', driveFileLink: 'https://drive/x', localCopyPath: '/tmp/x.srt' }));
  await page.locator('.transfer-card', { hasText: name }).click();
  await expect(page.locator('[data-detail-action="open-captions"]')).toBeVisible();
  await expect(page.locator('[data-detail-action="show-captions"]')).toBeVisible();
});

test('a non-video upload has no caption line', async ({ page }) => {
  const name = `notes-${Date.now()}.pdf`;
  await signInAndUpload(page, name);
  await expect(page.locator('.transfer-card', { hasText: name }).locator('[data-caption-line]')).toHaveCount(0);
});

test('Settings explains when captions are unavailable and disables the controls', async ({ page }) => {
  await signInAndUpload(page, `settings-${Date.now()}.pdf`);
  const settings = await page.evaluate(() => (window as any).go.main.App.CaptionsGetSettings());
  test.skip(settings.available, 'this run has a speech engine configured');
  await page.click('[data-nav="settings"]');
  const section = page.locator('[data-captions-settings]');
  await expect(section).toBeVisible();
  await expect(section.locator('[data-captions-notice]')).toContainText(settings.unavailableReason);
  await expect(section.locator('[data-caption-lang="auto"]')).toBeDisabled();
});

test('caption settings survive a restart', async ({ page }) => {
  await signInAndUpload(page, `persist-${Date.now()}.pdf`);
  const before = await page.evaluate(() => (window as any).go.main.App.CaptionsGetSettings());
  test.skip(!before.available, 'needs BALLAST_WHISPER_CLI pointing at a speech engine');
  await page.click('[data-nav="settings"]');
  await page.click('[data-caption-lang="auto"]');
  await expect(page.locator('[data-caption-lang="auto"]')).toHaveClass(/active/);
  await page.evaluate(() => (window as any).go.main.App.DebugRestart());
  await page.reload();
  await page.waitForFunction(() => !!(window as any).go?.main?.App);
  const after = await page.evaluate(() => (window as any).go.main.App.CaptionsGetSettings());
  expect(after.language).toBe('auto');
  await page.evaluate(() => (window as any).go.main.App.CaptionsSetLanguage('en'));
});
