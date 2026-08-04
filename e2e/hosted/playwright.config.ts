import { defineConfig, devices } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Browser checks against the static hosted-demo output (dist/hosted under /scry/).
 * Not part of CI — run via `make hosted-demo` (builds + tests) or
 * `npx playwright test --config e2e/hosted/playwright.config.ts` after a build.
 *
 * Layout: dist/pages/scry/ holds the built demo; `npx serve dist/pages` makes
 * http://127.0.0.1:4173/scry/ resolve correctly with SCRY_BASE_PATH=/scry/.
 */
const root = join(dirname(fileURLToPath(import.meta.url)), '../..')
const pagesRoot = join(root, 'dist', 'pages')

export default defineConfig({
  testDir: '.',
  testMatch: 'hosted.spec.ts',
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
    // Stage dist/hosted under dist/pages/scry so the /scry/ base path works,
    // then serve dist/pages on :4173.
    command: [
      'set -euo pipefail',
      `ROOT="${root}"`,
      'test -f "$ROOT/dist/hosted/index.html" || { echo "run make hosted-demo first" >&2; exit 1; }',
      'rm -rf "$ROOT/dist/pages"',
      'mkdir -p "$ROOT/dist/pages/scry"',
      'cp -R "$ROOT/dist/hosted/." "$ROOT/dist/pages/scry/"',
      'npx --yes serve "$ROOT/dist/pages" -l 4173 --no-port-switching',
    ].join(' && '),
    url: 'http://127.0.0.1:4173/scry/',
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
