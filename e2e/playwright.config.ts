import { defineConfig, devices } from '@playwright/test'

/**
 * Browser E2E against the committed demo.db mirror.
 * webServer runs e2e/serve.sh (build binary + UI, seed home, serve :7877).
 */
export default defineConfig({
  testDir: '.',
  // demo/ is the recording pipeline; hosted/ has its own config (static Pages
  // smoke). Excluding both keeps the suite honest ("N passed", not "N + skipped").
  testIgnore: ['**/demo/**', '**/hosted/**'],
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['github']] : 'list',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: 'http://127.0.0.1:7877',
    trace: 'on-first-retry',
    locale: 'en-US',
  },
  webServer: {
    command: 'bash e2e/serve.sh',
    url: 'http://127.0.0.1:7877/healthz',
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
