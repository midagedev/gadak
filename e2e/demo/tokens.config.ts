import { defineConfig, devices } from '@playwright/test'

import { FRAME_H, FRAME_W, PROMO_PORT } from './promo-split'

/**
 * Tokens promo — CLI `config set` retints an open tab (GDK-795).
 * Separate outputDir so `make media-web` cannot pick up this video.webm.
 *
 * Run via `bash e2e/demo/record-promo.sh` (sets GADAK_MEDIA=1).
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'tokens-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 30_000 },
  outputDir: '../.tmp/test-results-tokens',
  use: {
    baseURL: `http://127.0.0.1:${PROMO_PORT}`,
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: FRAME_W, height: FRAME_H },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: FRAME_W, height: FRAME_H },
    },
    launchOptions: { slowMo: 0 },
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'off',
  },
  webServer: {
    command: `GADAK_E2E_PORT=${PROMO_PORT} GADAK_FRESHEN=1 bash e2e/serve.sh`,
    url: `http://127.0.0.1:${PROMO_PORT}/healthz`,
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
        viewport: { width: FRAME_W, height: FRAME_H },
        deviceScaleFactor: 2,
      },
    },
  ],
})
