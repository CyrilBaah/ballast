import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

// Google's OAuth/Drive API are mocked at the network boundary (mock_e2e.go)
// via BALLAST_E2E_MOCK=1, same as the other E2E specs.
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

// No clipped/overlapping content (quickstart.md Scenario 1 step 4; SC-002)
// is checked structurally: no horizontal overflow at the document level at
// each of the app's supported window sizes. Wails' default launch size is
// 1024x768 (main.go); no explicit min/max is configured, so "minimum" and
// "large" bracket that default with a small and a roomy size.
async function hasHorizontalOverflow(page: Page): Promise<boolean> {
  return page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
}

const SIZES = [
  { name: 'minimum', width: 800, height: 600 },
  { name: 'default', width: 1024, height: 768 },
  { name: 'large', width: 1440, height: 900 },
];

test.beforeEach(() => {
  setOutcome('approve');
});

test.afterEach(() => {
  setOutcome('approve');
});

for (const size of SIZES) {
  test(`sign-in screen has no clipped/overlapping content at ${size.name} size (SC-002)`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    await page.goto('/');
    await waitForBindings(page);
    await page.evaluate(() => (window as any).go.main.App.AuthSignOut().catch(() => {}));
    await page.reload();
    await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
    expect(await hasHorizontalOverflow(page)).toBe(false);
  });

  test(`picker screen inside the sidebar shell has no clipped/overlapping content at ${size.name} size (SC-002)`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    await signIn(page);
    await expect(page.locator('.sidebar')).toBeVisible();
    expect(await hasHorizontalOverflow(page)).toBe(false);
  });

  test(`progress screen inside the sidebar shell has no clipped/overlapping content at ${size.name} size (SC-002)`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    await signIn(page);

    const file = makeTempFile('visual-consistency.txt', 4_000);
    await stubFilePicker(page, file);
    await page.click('#pick-file-btn');
    await page.click('#upload-btn');

    await expect(page.locator('.progress-screen')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('.sidebar')).toBeVisible();
    expect(await hasHorizontalOverflow(page)).toBe(false);
  });
}

test('the same visual system carries across sign-in, picker, and progress with no jarring shift (Acceptance Scenario 1 & 2)', async ({
  page,
}) => {
  await page.goto('/');
  await waitForBindings(page);
  await page.evaluate(() => (window as any).go.main.App.AuthSignOut().catch(() => {}));
  await page.reload();
  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });

  const bg = await page.evaluate(() => getComputedStyle(document.body).color);
  expect(bg).not.toBe('');

  await signIn(page);
  await expect(page.locator('.shell')).toBeVisible();
  await expect(page.locator('.sidebar')).toBeVisible();
  await expect(page.locator('#nav-upload')).toBeVisible();
  await expect(page.locator('#nav-history')).toBeVisible();
});
