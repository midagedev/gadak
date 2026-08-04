import { defineConfig, devices } from '@playwright/test'

/**
 * Demo media recording config — separate from e2e/playwright.config.ts.
 * Records a single slow, readable web UI walkthrough for docs/media/.
 *
 * Run via `make media-web` (sets SCRY_MEDIA=1). Do not use for CI gates.
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'web-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results',
  use: {
    baseURL: 'http://127.0.0.1:7877',
    locale: 'en-US',
    // Fixed showcase framing (retina-ish pixels for crisp stills).
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1280, height: 800 },
    },
    // Slow enough that a viewer can read each step; not so slow it becomes a slog.
    launchOptions: { slowMo: 35 },
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'off',
  },
  webServer: {
    command: 'SCRY_FRESHEN=1 bash e2e/serve.sh',
    url: 'http://127.0.0.1:7877/healthz',
    // Demo recordings often re-run while a previous serve is still up.
    reuseExistingServer: true,
    timeout: 180_000,
    cwd: '../..',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 800 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
