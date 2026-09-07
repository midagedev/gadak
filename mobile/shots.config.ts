import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gateWebServers, mobileAPIPort, mobileUIPort, UI_ORIGIN } from './e2e/serve'

/*
 * Capture harness, separate from the gate (GDK-904).
 *
 * `playwright.config.ts` next door is the *gate*: it measures geometry and
 * fails. This one produces pictures for a review round and asserts almost
 * nothing, so the two must not share a testDir — a capture walk that a CI
 * runner executes is wasted minutes, and a gate that a review round skips
 * is a gate nobody runs. Same fixture and same viewport; only testDir and
 * the reporter differ.
 *
 * The servers themselves come from mobile/e2e/serve.ts, shared with the gate
 * (GDK-1540). That file is the single owner of both ports and of the stamp
 * that says which worktree built the bundle. Before it, this config and the
 * gate's each hardcoded 5182/7899 with `reuseExistingServer: true`, so a
 * capture walk started while a gate was up quietly photographed whatever
 * that gate's tree was serving — and on a dev server, an edit under either
 * tree reloaded the page under both. Run a walk beside a gate by giving one
 * of them GADAK_MOBILE_E2E_PORT / GADAK_MOBILE_API_PORT.
 *
 * Run: npm run shots -- --grep <label>   (or bare, for the whole walk)
 * Out: scratch/mobile-shots/<cycle>/ with a MANIFEST naming the source
 *      tree, because a verdict on a stale capture is worse than no verdict
 *      (incident: stale-capture-vision-fix).
 */
const mobileDir = dirname(fileURLToPath(import.meta.url))

export { SERVE_ORIGIN, UI_ORIGIN } from './e2e/serve'

export default defineConfig({
  testDir: './shots',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 20_000 },
  globalSetup: join(mobileDir, 'e2e', 'serve.ts'),
  outputDir: join(mobileDir, 'test-results', `shots-${mobileUIPort()}-${mobileAPIPort()}`),
  use: {
    baseURL: UI_ORIGIN,
    viewport: { width: 402, height: 874 },
    deviceScaleFactor: 3,
    isMobile: true,
    hasTouch: true,
    locale: 'en-US',
    trace: 'off',
    screenshot: 'off',
  },
  webServer: gateWebServers(),
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
