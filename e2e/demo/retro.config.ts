import { defineConfig, devices } from '@playwright/test'
import { apiURL, e2eServePort } from '../helpers'

/**
 * Retro promo recording — the weekly report, and a number opened as issues.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-retro` (sets GADAK_MEDIA=1).
 */
const e2ePort = e2eServePort()

export default defineConfig({
  testDir: '.',
  testMatch: 'retro-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results-retro',
  use: {
    baseURL: apiURL(),
    locale: process.env.GADAK_MEDIA_LOCALE === 'ko' ? 'ko-KR' : process.env.GADAK_MEDIA_LOCALE === 'ja' ? 'ja-JP' : 'en-US',
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
    command: `GADAK_E2E_PORT=${e2ePort} GADAK_FRESHEN=1 bash e2e/serve.sh`,
    url: apiURL('/healthz'),
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
