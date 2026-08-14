import { defineConfig, devices } from '@playwright/test'

/**
 * Unified-search palette recording — ⌘K, a body/comment token, All search.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-search` (sets GADAK_MEDIA=1).
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'search-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results-search',
  use: {
    baseURL: 'http://127.0.0.1:7877',
    locale: 'en-US',
    // Same 1024×640 frame as the hero (README renders search.gif at 900 px).
    viewport: { width: 1024, height: 640 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1024, height: 640 },
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
        viewport: { width: 1024, height: 640 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
