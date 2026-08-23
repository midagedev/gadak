import { defineConfig, devices } from '@playwright/test'
import { apiURL, e2eHomeDir, e2eServePort } from './helpers'

/**
 * Browser E2E against the committed demo.db mirror.
 * webServer runs e2e/serve.sh (build binary + UI, seed home, serve GADAK_E2E_PORT).
 */
const e2ePort = e2eServePort()
console.log(`[e2e] GADAK_E2E_PORT=${e2ePort} home=${e2eHomeDir()}`)

export default defineConfig({
  testDir: '.',
  globalSetup: './helpers.ts',
  // demo/ is the recording pipeline; hosted/ has its own config (static Pages
  // smoke). Excluding both keeps the suite honest ("N passed", not "N + skipped").
  testIgnore: ['**/demo/**', '**/hosted/**', '**/perf/**'],
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['github']] : 'list',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: apiURL(),
    // Local retries are 0, so on-first-retry produced no trace for the
    // GDK-39 flake. retain-on-failure + a screenshot keep the key sequence
    // and the URL/list-count in the artifact without retrying the test.
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    locale: 'en-US',
  },
  webServer: {
    command: `GADAK_E2E_PORT=${e2ePort} bash e2e/serve.sh`,
    url: apiURL('/healthz'),
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
    cwd: '..',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
