import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/*
 * Capture harness, separate from the gate (GDK-904).
 *
 * `playwright.config.ts` next door is the *gate*: it measures geometry and
 * fails. This one produces pictures for a review round and asserts almost
 * nothing, so the two must not share a testDir — a capture walk that a CI
 * runner executes is wasted minutes, and a gate that a review round skips
 * is a gate nobody runs. Same ports, same fixture, same viewport; only
 * testDir and the reporter differ.
 *
 * Run: npm run shots -- --grep <label>   (or bare, for the whole walk)
 * Out: scratch/mobile-shots/<cycle>/ with a MANIFEST naming the source
 *      tree, because a verdict on a stale capture is worse than no verdict
 *      (incident: stale-capture-vision-fix).
 */
const mobileDir = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(mobileDir, '..')

export const UI_ORIGIN = 'http://127.0.0.1:5182'
export const SERVE_ORIGIN = 'http://127.0.0.1:7899'

export default defineConfig({
  testDir: './shots',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 20_000 },
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
  webServer: [
    {
      command:
        'CGO_ENABLED=0 go build -o /tmp/gadak-mobile-viewport ./cmd/gadak && /tmp/gadak-mobile-viewport demo --addr 127.0.0.1:7899 --no-open',
      url: `${SERVE_ORIGIN}/healthz`,
      reuseExistingServer: true,
      timeout: 180_000,
      cwd: repoRoot,
    },
    {
      command: 'npx vite --port 5182 --strictPort --host 127.0.0.1',
      url: `${UI_ORIGIN}/`,
      reuseExistingServer: true,
      timeout: 60_000,
      cwd: mobileDir,
    },
  ],
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
