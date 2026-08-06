import { defineConfig, devices } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Interaction performance budgets against a ~10k-issue fixture.
 * Isolated from the main e2e suite (e2e/playwright.config.ts on :7877).
 *
 * Run: npm run test:perf
 */
const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

export default defineConfig({
  testDir: '.',
  testMatch: 'perf.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  // 4 metrics × (1 warmup + 20 samples); cold boot dominates wall time.
  timeout: 600_000,
  expect: { timeout: 60_000 },
  use: {
    baseURL: 'http://127.0.0.1:7878',
    trace: 'off',
    locale: 'en-US',
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
  },
  webServer: {
    command: 'bash e2e/perf/serve.sh',
    url: 'http://127.0.0.1:7878/healthz',
    reuseExistingServer: !process.env.CI,
    timeout: 300_000,
    cwd: root,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
