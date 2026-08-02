import { test, expect, type Page } from '@playwright/test';
import * as fs from 'node:fs';

// Covers quickstart.md Scenario 1 (sign-in, persistence across relaunch,
// sign-out, cancel/deny mid-flow). Google's OAuth consent screen is mocked
// entirely at the network boundary (research.md §5, mock_e2e.go) via
// BALLAST_E2E_MOCK=1 -- no real Google account or browser popup is
// involved. The outcome ("approve" vs "deny" consent) is toggled between
// test cases by writing to BALLAST_E2E_OUTCOME_FILE, which the mocked
// backend re-reads on every Auth.SignIn call.
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

  // Simulate relaunch: reload the page. The backend process and its
  // SQLite-backed session survive a reload exactly as they would survive
  // an app restart, so Auth.GetStatus must report signedIn:true
  // immediately with no consent screen shown again (SC-001/SC-005).
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

test('cancelling/denying Google consent leaves no session (Edge Case)', async ({
  page,
}) => {
  setOutcome('deny');
  await page.click('#signin-btn');

  // A denial is a valid, expected outcome (contracts/wails-bindings.md):
  // the app returns to a clean signed-out state, no error banner, and no
  // session is persisted.
  await expect(page.locator('.picker-screen')).toHaveCount(0);
  await expect(page.locator('#signin-btn')).toBeVisible();
  await expect(page.locator('#signin-error')).toHaveText('');

  await page.reload();
  await expect(page.locator('#signin-btn')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.picker-screen')).toHaveCount(0);

  setOutcome('approve');
});
