import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Covers Feature 006's UI contract: the one-time summary-model prompt, a
// summary line per phase kept separate from the upload and captions, the
// summary's detail actions including Try again, and the Settings section.
// As in captions.spec.ts, states arrive through the same Wails events the
// worker emits ("summaries:updated", "summaries:consent-needed"); the
// worker itself is covered by internal/summaries' Go tests.
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

function summaryJob(uploadId: number, over: Record<string, unknown>) {
  return { uploadId, status: 'in_progress', progressPercent: 0, canRetry: false, ...over };
}

test.afterEach(() => fs.writeFileSync(outcomeFile, 'approve'));

test('the summary-model prompt shows and answering it hides it', async ({ page }) => {
  await signInAndUpload(page, `sprompt-${Date.now()}.mp4`);
  await emit(page, 'summaries:consent-needed', { modelSizeBytes: 2_500_000_000 });
  const banner = page.locator('[data-summary-consent-banner]');
  await expect(banner).toBeVisible();
  await page.click('[data-summary-consent="no"]');
  await expect(banner).toHaveCount(0);
});

test('each summary phase shows its own line, separate from captions', async ({ page }) => {
  const name = `sphases-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  const card = page.locator('.transfer-card', { hasText: name });
  await emit(page, 'captions:updated', { uploadId: id, status: 'done', progressPercent: 0, language: 'en', driveFileLink: 'https://drive/c' });
  const line = card.locator('[data-summary-line]');
  const steps: [Record<string, unknown>, string][] = [
    [{ status: 'waiting', phase: 'waiting_for_captions' }, 'Summary: waiting for captions'],
    [{ status: 'waiting', phase: 'waiting_for_engine' }, 'Summary: waiting for captions to finish on this Mac'],
    [{ phase: 'summarising', progressPercent: 40 }, 'Summary: writing — 40%'],
    [{ phase: 'waiting_for_video', localCopyPath: '/tmp/s.md' }, 'Summary ready on this Mac — waiting for the video'],
    [{ status: 'done', driveFileLink: 'https://docs/x', localCopyPath: '/tmp/s.md' }, 'Summary ready'],
  ];
  for (const [over, text] of steps) {
    await emit(page, 'summaries:updated', summaryJob(id, over));
    await expect(line).toHaveText(text);
  }
  await expect(card.locator('[data-caption-line]')).toHaveText('Captions ready');
});

test('a failed summary offers Try again and leaves the upload and captions alone', async ({ page }) => {
  const name = `sfail-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  const card = page.locator('.transfer-card', { hasText: name });
  const chip = await card.locator('.status-chip').textContent();
  await emit(page, 'captions:updated', { uploadId: id, status: 'done', progressPercent: 0, language: 'en', driveFileLink: 'https://drive/c' });
  await emit(page, 'summaries:updated', summaryJob(id, { status: 'failed', note: 'Not enough free memory to write the summary — close other apps and try again', canRetry: true }));
  await expect(card.locator('[data-summary-line]')).toContainText("Summary couldn't be made — Not enough free memory");
  await expect(card.locator('.status-chip')).toHaveText(chip ?? '');
  await expect(card.locator('[data-caption-line]')).toHaveText('Captions ready');
  await card.click();
  await expect(page.locator('[data-detail-action="retry-summary"]')).toBeVisible();
});

test('the detail panel offers the summary in Drive and in Finder', async ({ page }) => {
  const name = `sdetail-${Date.now()}.mp4`;
  const id = await signInAndUpload(page, name);
  await emit(page, 'summaries:updated', summaryJob(id, { status: 'done', driveFileLink: 'https://docs/x', localCopyPath: '/tmp/s.md' }));
  await page.locator('.transfer-card', { hasText: name }).click();
  await expect(page.locator('[data-detail-action="open-summary"]')).toBeVisible();
  await expect(page.locator('[data-detail-action="show-summary"]')).toBeVisible();
});

test('Settings shows the Video summaries section and why it is unavailable', async ({ page }) => {
  await signInAndUpload(page, `ssettings-${Date.now()}.pdf`);
  const settings = await page.evaluate(() => (window as any).go.main.App.SummariesGetSettings());
  await page.click('[data-nav="settings"]');
  const section = page.locator('[data-summaries-settings]');
  await expect(section).toBeVisible();
  if (!settings.available) {
    await expect(section.locator('[data-summaries-notice]')).toContainText(settings.unavailableReason);
  } else {
    await expect(section.locator('[data-summaries-notice]')).toContainText('written on this Mac for free');
  }
});
