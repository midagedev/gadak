import { defineConfig, devices } from '@playwright/test'

import { FRAME_H, FRAME_W, PROMO_PORT } from './promo-split'

/**
 * Right-hand serve tab for docs/media/claude-drive.{gif,mp4}.
 * The left pane is a VHS recording of a live Claude Code session
 * (tools/tapes/claude-drive.tape); this config only captures the paper
 * chrome + app iframe so export-claude-drive.sh can hstack them.
 *
 * No webServer: record-claude-drive.sh starts serve itself against the
 * frozen capture home. reuseExistingServer is therefore required.
 *
 * Gated by GADAK_MEDIA=1. Viewport must stay FRAME_W×FRAME_H (promo-split.ts)
 * or Playwright letterboxes the capture.
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'claude-drive-web.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 600_000,
  expect: { timeout: 30_000 },
  outputDir: '../.tmp/test-results-claude-drive',
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
