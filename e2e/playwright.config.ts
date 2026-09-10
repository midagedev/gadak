import { defineConfig, devices } from '@playwright/test'
import {
  LINEAR_SEED_DB,
  apiURL,
  e2eHomeDir,
  e2eServePort,
  linearApiURL,
  linearServePort,
} from './helpers'

/**
 * Browser E2E against the committed demo.db mirror.
 * webServer runs e2e/serve.sh (build binary + UI, seed home, serve GADAK_E2E_PORT).
 * A second serve (GDK-1298) seeds the Linear fixture examples/demo-linear.db on
 * linearServePort() for e2e/linear.spec.ts — same script, same mechanism, one
 * more port (GDK-672 homes and stamps are port-keyed already).
 */
const e2ePort = e2eServePort()
const linearPort = linearServePort()
console.log(`[e2e] GADAK_E2E_PORT=${e2ePort} home=${e2eHomeDir()} linear=${linearPort}`)

// The two serves share one worktree, so their build phases (the go binary
// under e2e/.tmp, dist/app from `npm run build`) must not overlap. The
// second command polls the first serve's /healthz before running serve.sh:
// healthz answers only once that run finished building and is serving, which
// serializes the two end to end. The poll budget stays under the webServer
// timeout so a first serve that never comes up fails in playwright's words,
// not as a silent node exit.
const waitMainServe = `node -e "const u='${apiURL('/healthz')}';const t0=Date.now();(function p(){fetch(u).then(()=>process.exit(0)).catch(()=>{if(Date.now()-t0>150000)process.exit(1);setTimeout(p,500)})})()"`

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
  webServer: [
    {
      command: `GADAK_E2E_PORT=${e2ePort} bash e2e/serve.sh`,
      url: apiURL('/healthz'),
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
      cwd: '..',
    },
    {
      command: `${waitMainServe} && GADAK_E2E_PORT=${linearPort} GADAK_SEED_DB=${LINEAR_SEED_DB} bash e2e/serve.sh`,
      url: linearApiURL('/healthz'),
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
      cwd: '..',
    },
  ],
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
