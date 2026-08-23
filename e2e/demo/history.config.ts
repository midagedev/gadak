import { defineConfig, devices } from '@playwright/test'

/**
 * History-focus recording (F2) — one issue thread read end to end.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-history` (sets GADAK_MEDIA=1). Committed fixture
 * (no GADAK_SEED_DB): NMB-139's thread is fixture-injected by serve.sh.
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'history-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 60_000 },
  outputDir: 'test-results-history',
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
    // Trace on: playwright-recast renders the auto-zoom cut from it
    // (GDK-746). The webm video stays the flat-record source of truth.
    trace: 'on',
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
