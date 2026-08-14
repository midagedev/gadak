import { defineConfig, devices } from '@playwright/test'

/**
 * Agent-focus promo recording — CLI types, the paper list follows.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-agent` (sets GADAK_MEDIA=1).
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'agent-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results-agent',
  use: {
    baseURL: 'http://127.0.0.1:7877',
    locale: 'en-US',
    // Terminal chrome (168) + the same 1024×640 app frame as the hero.
    viewport: { width: 1024, height: 808 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1024, height: 808 },
    },
    launchOptions: { slowMo: 0 },
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
        viewport: { width: 1024, height: 808 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
