import { defineConfig, devices } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Browser checks against the static site output (dist/hosted, served at /).
 * Runs in CI (pages.yml, before the artifact is uploaded) and locally via
 * `make hosted-demo` (builds + tests) or
 * `npx playwright test --config e2e/hosted/playwright.config.ts` after a build.
 * first-frame.spec.ts is the GDK-55 390×844 readability gate; hosted.spec.ts
 * carries the GDK-52 "no verb it cannot answer" gate.
 *
 * Layout: dist/hosted is the document root — / is the landing page, /demo/ the
 * app, /backlog/ the backlog viewer (GDK-676, the gadak.dev apex).
 */
const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 90_000,
  expect: { timeout: 30_000 },
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'on-first-retry',
    locale: 'en-US',
  },
  webServer: {
    // Serve dist/hosted on :4173 as the document root. The script names its
    // own interpreter:
    // Playwright runs this string through /bin/sh, which is bash locally and
    // dash on the CI runner, and the inline version's `set -euo pipefail` died
    // there ("Illegal option -o pipefail") after passing every local run.
    command: 'bash e2e/hosted/serve.sh',
    url: 'http://127.0.0.1:4173/demo/',
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    cwd: root,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
