import { defineConfig, devices } from '@playwright/test'
import { apiURL, e2eServePort } from '../helpers'

/**
 * The command block's design captures (GDK-1162 / GDK-1164-A).
 *
 * Its own config, like every other spec in this directory, so nothing here is
 * picked up by a run someone meant for something else — `testMatch` is what
 * keeps these files apart.
 *
 * Unlike the media configs this one records no video and does not freshen the
 * sync clock: it takes still PNGs of the app as the suite serves it, so it
 * rides e2e/serve.sh on the suite's own port and reuses a server that is
 * already up.
 *
 *   npx playwright test --config e2e/demo/issue-command-shots.config.ts
 */
const port = e2eServePort()

export default defineConfig({
  testDir: '.',
  testMatch: 'issue-command-shots.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 15_000 },
  outputDir: 'test-results-issue-command-shots',
  use: {
    baseURL: apiURL(),
    locale: 'en-US',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: `GADAK_E2E_PORT=${port} bash e2e/serve.sh`,
    url: apiURL('/healthz'),
    reuseExistingServer: true,
    timeout: 180_000,
    cwd: '../..',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
