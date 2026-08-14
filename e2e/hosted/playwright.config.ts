import { defineConfig, devices } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Browser checks against the static hosted-demo output (dist/hosted under /gadak/).
 * Not part of CI — run via `make hosted-demo` (builds + tests) or
 * `npx playwright test --config e2e/hosted/playwright.config.ts` after a build.
 *
 * Layout: dist/pages/gadak/ holds the built demo; `npx serve dist/pages` makes
 * http://127.0.0.1:4173/gadak/ resolve correctly with GADAK_BASE_PATH=/gadak/.
 */
const root = join(dirname(fileURLToPath(import.meta.url)), '../..')
const pagesRoot = join(root, 'dist', 'pages')

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
    // Stage dist/hosted under dist/pages/gadak so the /gadak/ base path works,
    // then serve dist/pages on :4173.
    command: [
      'set -euo pipefail',
      `ROOT="${root}"`,
      'test -f "$ROOT/dist/hosted/index.html" || { echo "run make hosted-demo first" >&2; exit 1; }',
      'rm -rf "$ROOT/dist/pages"',
      'mkdir -p "$ROOT/dist/pages/gadak"',
      'cp -R "$ROOT/dist/hosted/." "$ROOT/dist/pages/gadak/"',
      'npx --yes serve "$ROOT/dist/pages" -l 4173 --no-port-switching',
    ].join(' && '),
    url: 'http://127.0.0.1:4173/gadak/',
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
