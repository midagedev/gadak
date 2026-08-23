import { defineConfig, devices } from '@playwright/test'

/**
 * Group-by promo recording — Breakdown menu, three axes, the bar holds.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-groupby` (sets GADAK_MEDIA=1).
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'groupby-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results-groupby',
  use: {
    baseURL: 'http://127.0.0.1:7877',
    locale: 'en-US',
    // Website clip (not the README 900 px render). 1280×800 is the C1
    // contract; video.size must equal the viewport or Playwright letterboxes.
    colorScheme: 'light',
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1280, height: 800 },
    },
    launchOptions: { slowMo: 35 },
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'off',
  },
  webServer: {
    command: 'GADAK_FRESHEN=1 bash e2e/serve.sh',
    url: 'http://127.0.0.1:7877/healthz',
    reuseExistingServer: false,
    timeout: 180_000,
    cwd: '../..',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        colorScheme: 'light',
        viewport: { width: 1280, height: 800 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
