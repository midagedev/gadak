import { defineConfig, devices } from '@playwright/test'

import { PROMO_PORT, V_FRAME_H, V_FRAME_W } from './promo-split'

/**
 * Dashboards promo — vertical 1080×1350 (4:5) social cut.
 * Reuses dashboards-demo.spec.ts; GADAK_PROMO_LAYOUT=vertical selects stacked
 * chrome in promo-split.ts. Separate outputDir so landscape export cannot
 * pick up this video.webm.
 *
 * Run via `bash e2e/demo/record-vertical.sh` (sets GADAK_MEDIA=1,
 * GADAK_PROMO_LAYOUT=vertical, GADAK_E2E_PORT=7890).
 */
process.env.GADAK_PROMO_LAYOUT = 'vertical'
if (!process.env.GADAK_E2E_PORT) {
  throw new Error(
    'dashboards-vertical.config.ts: set GADAK_E2E_PORT (record-vertical.sh uses 7890) so this does not collide with the landscape promo on 7888',
  )
}

export default defineConfig({
  testDir: '.',
  testMatch: 'dashboards-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 30_000 },
  outputDir: '../.tmp/test-results-dashboards-vertical',
  use: {
    baseURL: `http://127.0.0.1:${PROMO_PORT}`,
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: V_FRAME_W, height: V_FRAME_H },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: V_FRAME_W, height: V_FRAME_H },
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
        viewport: { width: V_FRAME_W, height: V_FRAME_H },
        deviceScaleFactor: 2,
      },
    },
  ],
})
