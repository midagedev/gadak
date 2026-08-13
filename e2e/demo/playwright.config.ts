import { defineConfig, devices } from '@playwright/test'

/**
 * Demo media recording config — separate from e2e/playwright.config.ts.
 * Records a single slow, readable web UI walkthrough for docs/media/.
 *
 * Run via `make media-web` (sets GADAK_MEDIA=1). Do not use for CI gates.
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
    // Showcase framing. 1024 CSS px, not 1280: the GIF is displayed at ~900 px
    // in the README, so a narrower logical viewport is the only lever on text
    // size — at 1280 the glyphs land near 0.7× and readers said so. 1024 is the
    // floor that still matches Tailwind's `lg:` breakpoint, which is what keeps
    // the epic chip and the trailing row strip on screen.
    viewport: { width: 1024, height: 640 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      // Must equal the viewport. Playwright never *upscales* into a larger
      // video frame — asking for 2048×1280 pins the capture in the top-left
      // corner and pads the rest black (cost us one take). The GIF is exported
      // at this width 1:1, so no resample happens anywhere in the chain.
      size: { width: 1024, height: 640 },
    },
    // Slow enough that a viewer can read each step; not so slow it becomes a slog.
    launchOptions: { slowMo: 35 },
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'off',
  },
  webServer: {
    command: 'GADAK_FRESHEN=1 bash e2e/serve.sh',
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
        viewport: { width: 1024, height: 640 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
