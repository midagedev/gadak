import { defineConfig, devices } from '@playwright/test'

/**
 * Scale flagship recording — the 20k-issue mirror, search at typing speed.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-scale` (sets GADAK_MEDIA=1 and GADAK_SEED_DB to the
 * snapshot it generated). Viewport 1280×800 matches the C1 contract.
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'scale-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 240_000,
  expect: { timeout: 60_000 },
  outputDir: 'test-results-scale',
  use: {
    baseURL: 'http://127.0.0.1:7877',
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1280, height: 800 },
    },
    launchOptions: { slowMo: 30 },
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
    trace: 'off',
  },
  webServer: {
    // GADAK_SEED_DB is exported by `make media-scale`; a bare config run
    // would record over the 534-issue fixture and the count assert fails.
    command: 'GADAK_SEED_DB="$GADAK_SEED_DB" GADAK_FRESHEN=1 bash e2e/serve.sh',
    url: 'http://127.0.0.1:7877/healthz',
    reuseExistingServer: false,
    timeout: 300_000,
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
