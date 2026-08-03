import { defineConfig, devices } from "@playwright/test";

// Playwright drives the frontend against `wails dev`'s local dev server
// (research.md §5) rather than a packaged native window, so no CDP/WebView2
// automation bridge is needed. `wails dev` serves the frontend at
// http://localhost:34115 by default.
export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  timeout: 30_000,
  use: {
    baseURL: "http://localhost:34115",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
