import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';

// Google's OAuth consent screen is mocked entirely at the network boundary
// (mock_e2e.go) via BALLAST_E2E_MOCK=1, so no real Google account or
// browser popup is involved. The outcome is toggled between test cases by
// writing "approve" or "deny" to BALLAST_E2E_OUTCOME_FILE.
const outcomeFile =
  process.env.BALLAST_E2E_OUTCOME_FILE ?? `${__dirname}/.e2e-outcome`;

function setOutcome(outcome: 'approve' | 'deny') {
  fs.writeFileSync(outcomeFile, outcome);
}

async function waitForBindings(page: Page) {
  await page.waitForFunction(() => !!(window as any).go?.main?.App, undefined, {
    timeout: 10_000,
  });
}

async function resetToSignedOut(page: Page) {
  await waitForBindings(page);
  // Best-effort: ignore failures if there was no session to begin with.
  await page.evaluate(() => {
    const w = window as any;
    return w.go.main.App.AuthSignOut().catch(() => {});
  });
  await page.reload();
}

test.beforeEach(async ({ page }) => {
  setOutcome('approve');
  await page.goto('/');
  await resetToSignedOut(page);
});

test('fresh launch shows the sign-in screen, signed out', async ({ page }) => {
  await expect(page.locator('#signin-btn')).toBeVisible();
  await expect(page.locator('.picker-screen')).toHaveCount(0);
});

test('sign-in completes and the session persists across a relaunch (Acceptance Scenario 1 & 2)', async ({
  page,
}) => {
  setOutcome('approve');
  await page.click('#signin-btn');

  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toContainText(
    'e2e-mock-user@example.com',
  );

  // Simulate relaunch: a reload survives exactly like an app restart, so
  // Auth.GetStatus must report signedIn:true immediately with no consent screen shown again.
  await page.reload();
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('#signin-btn')).toHaveCount(0);
});

test('sign-out returns the app to the signed-out state (Acceptance Scenario 3)', async ({
  page,
}) => {
  setOutcome('approve');
  await page.click('#signin-btn');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });

  await page.evaluate(() => (window as any).go.main.App.AuthSignOut());
  await page.reload();

  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toHaveCount(0);
});

test('clicking the picker\'s Sign out button returns the app to the signed-out state', async ({
  page,
}) => {
  setOutcome('approve');
  await page.click('#signin-btn');
  await expect(page.locator('.picker-screen')).toBeVisible({ timeout: 10_000 });

  await page.click('#sign-out-btn');

  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toHaveCount(0);

  // Confirm the session was actually cleared, not just the UI swapped.
  await page.reload();
  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toHaveCount(0);
});

test('cancelling/denying Google consent leaves no session (Edge Case)', async ({
  page,
}) => {
  setOutcome('deny');
  await page.click('#signin-btn');

  // A denial is a valid, expected outcome: a clean signed-out state, no error banner, no session persisted.
  await expect(page.locator('.picker-screen')).toHaveCount(0);
  await expect(page.locator('#signin-btn')).toBeVisible();
  await expect(page.locator('#signin-error')).toHaveText('');

  await page.reload();
  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toHaveCount(0);

  setOutcome('approve');
});
