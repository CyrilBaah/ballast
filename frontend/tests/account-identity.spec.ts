import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';

// Google's OAuth/userinfo/Drive about.get are mocked at the network
// boundary (mock_e2e.go) via BALLAST_E2E_MOCK=1. userinfo's name/picture
// and about.get's storageQuota shape both vary by BALLAST_E2E_OUTCOME_FILE
// (mock_e2e.go's /userinfo and /about handlers) so this spec can reliably
// exercise the sidebar's account-identity fallbacks (FR-011/FR-012).
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
  await page.goto('/');
  await waitForBindings(page);
  await page.evaluate(() => (window as any).go.main.App.AuthSignOut().catch(() => {}));
  await page.reload();
  await waitForBindings(page);
  await page.click('#signin-btn');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
}

test.afterEach(() => {
  setOutcome('approve');
});

test('sidebar shows display name, profile photo, and storage usage when all are present (Acceptance Scenario 4, SC-006)', async ({
  page,
}) => {
  setOutcome('approve');
  await signIn(page);

  await expect(page.locator('#sidebar-name')).toHaveText('E2E Mock User');
  await expect(page.locator('.avatar-photo')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.avatar-fallback')).toHaveCount(0);

  const storageEl = page.locator('#sidebar-storage');
  await expect(storageEl).toBeVisible({ timeout: 10_000 });
  await expect(storageEl).toContainText('of');
  await expect(page.locator('.storage-bar')).toBeVisible();
});

test('a generated-initials avatar renders when no photo is available (FR-011)', async ({ page }) => {
  setOutcome('userinfo-name-only');
  await signIn(page);

  await expect(page.locator('#sidebar-name')).toHaveText('E2E Mock User');
  await expect(page.locator('.avatar-fallback')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.avatar-fallback')).toHaveText('E');
  await expect(page.locator('.avatar-photo')).toHaveCount(0);
});

test('a generated-initials avatar renders when the photo fails to load (FR-011)', async ({ page }) => {
  setOutcome('approve');
  await signIn(page);

  await expect(page.locator('.avatar-photo')).toBeVisible({ timeout: 10_000 });
  // Simulate a broken image URL rather than depending on real network
  // failure timing -- exercises the same <img onerror> fallback path.
  await page.evaluate(() => {
    const img = document.querySelector<HTMLImageElement>('.avatar-photo')!;
    img.src = 'https://127.0.0.1:1/does-not-exist.png';
  });

  await expect(page.locator('.avatar-fallback')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.avatar-photo')).toHaveCount(0);
});

test('the name line and avatar fall back to the email address when neither name nor picture is returned (FR-011, Clarifications 2026-08-05)', async ({
  page,
}) => {
  setOutcome('userinfo-neither');
  await signIn(page);

  await expect(page.locator('#sidebar-name')).toHaveText('e2e-mock-user@example.com');
  await expect(page.locator('.avatar-fallback')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.avatar-fallback')).toHaveText('E');
});

test('an unlimited-storage account renders usage without dividing by zero (data-model.md StorageQuota)', async ({
  page,
}) => {
  setOutcome('quota-unlimited');
  await signIn(page);

  const storageEl = page.locator('#sidebar-storage');
  await expect(storageEl).toBeVisible({ timeout: 10_000 });
  await expect(storageEl).toContainText('used');
  await expect(storageEl).not.toContainText('of');
  await expect(page.locator('.storage-bar')).toHaveCount(0);
});

test('a failed Drive.GetStorageQuota call omits the storage indicator while name/photo still render (FR-012, Clarifications 2026-08-05)', async ({
  page,
}) => {
  setOutcome('quota-fail');
  await signIn(page);

  await expect(page.locator('#sidebar-name')).toHaveText('E2E Mock User');
  await expect(page.locator('.avatar-photo')).toBeVisible({ timeout: 10_000 });

  // Give the (rejected) Drive.GetStorageQuota call time to settle, then
  // confirm no error state or retry affordance ever appears.
  await page.waitForTimeout(500);
  await expect(page.locator('#sidebar-storage')).toBeHidden();
});
