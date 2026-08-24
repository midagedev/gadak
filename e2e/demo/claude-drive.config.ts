import { defineConfig, devices } from '@playwright/test'

import {
  FLAGSHIP_L_FRAME_H,
  FLAGSHIP_L_FRAME_W,
  PROMO_PORT,
  V_FRAME_H,
  V_FRAME_W,
  isPromoVertical,
} from './promo-split'

/**
 * Right-hand serve tab for docs/media/claude-drive.{gif,mp4} (landscape)
 * and docs/media/claude-drive-vertical.mp4.
 *
 * The left/top pane is a VHS recording of a live Claude Code session
 * (tools/tapes/claude-drive.tape); this config only captures the paper
 * chrome + app iframe so export-claude-drive.sh can stack them.
 *
 * GADAK_PROMO_LAYOUT=vertical selects 1080×1350. Unset is flagship
 * landscape FLAGSHIP_L_FRAME_* (1880×720). tokens/dashboards keep
 * FRAME_W×FRAME_H (1744×672) via their own configs.
 *
 * No webServer: record-claude-drive.sh starts serve itself against the
 * frozen capture home. reuseExistingServer is therefore required.
 *
 * Gated by GADAK_MEDIA=1. Viewport must stay the layout's frame size or
 * Playwright letterboxes the capture.
 */
const vertical = isPromoVertical()
const VW = vertical ? V_FRAME_W : FLAGSHIP_L_FRAME_W
const VH = vertical ? V_FRAME_H : FLAGSHIP_L_FRAME_H
const resultsDir = vertical ? '../.tmp/test-results-claude-drive-vertical' : '../.tmp/test-results-claude-drive'

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
  outputDir: resultsDir,
  use: {
    baseURL: `http://127.0.0.1:${PROMO_PORT}`,
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: VW, height: VH },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: VW, height: VH },
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
        viewport: { width: VW, height: VH },
        deviceScaleFactor: 2,
      },
    },
  ],
})
