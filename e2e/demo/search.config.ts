import { defineConfig, devices } from '@playwright/test'
import { apiURL, e2eServePort } from '../helpers'

/**
 * Unified-search palette recording — ⌘K, a body/comment token, All search.
 * Separate from e2e/demo/playwright.config.ts so `make media-web` cannot
 * pick up this video.webm by accident.
 *
 * Run via `make media-search` (sets GADAK_MEDIA=1 and GADAK_SEED_DB to the
 * mirror the take reads — examples/demo.db for en, the translated copy for
 * ko/ja, GDK-1556).
 */
const e2ePort = e2eServePort()

export default defineConfig({
  testDir: '.',
  testMatch: 'search-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results-search',
  use: {
    baseURL: apiURL(),
    locale: 'en-US',
    // Same 1024×640 frame as the hero (README renders search.gif at 900 px).
    viewport: { width: 1024, height: 640 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1024, height: 640 },
    },
    launchOptions: { slowMo: 35 },
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'off',
  },
  webServer: {
    // GADAK_SEED_DB is exported by `make media-search`: unset (or empty) it
    // falls back to examples/demo.db inside serve.sh, which is what the
    // English take has always recorded over; a ko/ja take passes the
    // translated copy e2e/.tmp/demo-<locale>.db (GDK-1556).
    command: `GADAK_E2E_PORT=${e2ePort} GADAK_SEED_DB="$GADAK_SEED_DB" GADAK_FRESHEN=1 bash e2e/serve.sh`,
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
        viewport: { width: 1024, height: 640 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
