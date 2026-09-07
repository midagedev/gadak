import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gateWebServers, mobileAPIPort, mobileUIPort, UI_ORIGIN } from './e2e/serve'

const mobileDir = dirname(fileURLToPath(import.meta.url))

// Own ports, own home, and — since GDK-1540 — its own *built* bundle.
//
// The gate serves `vite build` output through `vite preview`, not the dev
// server. A dev server watches mobile/src and web/src and reloads the page
// when either changes, which is correct for `npm run dev` and fatal for a
// gate: in a tree where more than one round is working, an edit to a screen
// throws every spec in flight back to the first tab and they die on click
// timeouts that name nothing. A preview server has no watcher, so the bytes
// under a spec are the bytes that were built when the gate started.
//
// Ports come from mobile/e2e/serve.ts (GADAK_MOBILE_E2E_PORT /
// GADAK_MOBILE_API_PORT, defaulting to 5182 / 7899), so two gates can run at
// once by setting them; `reuseExistingServer` is still on but is now backed
// by the stamp check in that file's globalSetup, which refuses a server this
// worktree did not build. e2e/ next door hardcodes 127.0.0.1:7877 with its
// own home and workers: 1 — this gate must not contend with that set
// (CLAUDE.md). Home is `gadak demo`'s own temp dir, not e2e/.tmp.
export { SERVE_ORIGIN, UI_ORIGIN } from './e2e/serve'

export default defineConfig({
  testDir: './e2e',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['github']] : 'list',
  timeout: 90_000,
  expect: { timeout: 20_000 },
  // Provenance check + the `served … stamp … ui :… api :…` line.
  globalSetup: join(mobileDir, 'e2e', 'serve.ts'),
  // Port-keyed so two gates in two worktrees do not overwrite each other's
  // traces — the same reason the bundle dir and the demo binary are keyed.
  outputDir: join(mobileDir, 'test-results', `${mobileUIPort()}-${mobileAPIPort()}`),
  use: {
    baseURL: UI_ORIGIN,
    viewport: { width: 402, height: 874 },
    deviceScaleFactor: 3,
    isMobile: true,
    hasTouch: true,
    locale: 'en-US',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: gateWebServers(),
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
