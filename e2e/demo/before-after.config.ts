import { defineConfig, devices } from '@playwright/test'

/**
 * Before/after take (before-after.spec.ts). No webServer: record-before-after.sh
 * owns two serves — the previous release from a worktree and this one — and
 * runs this config once against each (GADAK_BA_BASE). 1472×994 at 1×, the
 * round-trip rig's frame, recorded at the viewport's own size so the camera
 * layer does the one and only resample.
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'before-after.spec.ts',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 240_000,
  expect: { timeout: 30_000 },
  outputDir: process.env.GADAK_BA_RESULTS || '../.tmp/test-results-before-after',
  use: {
    baseURL: process.env.GADAK_BA_BASE || 'http://127.0.0.1:7899',
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: 1472, height: 994 },
    deviceScaleFactor: 1,
    video: { mode: 'on', size: { width: 1472, height: 994 } },
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], colorScheme: 'light', viewport: { width: 1472, height: 994 }, deviceScaleFactor: 1 },
    },
  ],
})
