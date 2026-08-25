import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const mobileDir = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(mobileDir, '..')

// Own ports and own home. e2e/ hardcodes 127.0.0.1:7877 with a single
// e2e/.tmp/home and Playwright workers: 1 — this gate must not contend
// with that set (CLAUDE.md). The API port is 7899 because
// mobile/vite.config.ts proxies /api there and that file is outside this
// round's whitelist. The UI port is 5182 so a developer `npm run dev` on
// 5180 is left alone. Home is `gadak demo`'s own temp dir (not e2e/.tmp).
export const UI_ORIGIN = 'http://127.0.0.1:5182'
export const SERVE_ORIGIN = 'http://127.0.0.1:7899'

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
  webServer: [
    {
      command:
        'CGO_ENABLED=0 go build -o /tmp/gadak-mobile-viewport ./cmd/gadak && /tmp/gadak-mobile-viewport demo --addr 127.0.0.1:7899 --no-open',
      url: `${SERVE_ORIGIN}/healthz`,
      // 7899 is vite's hardcoded /api proxy (vite.config.ts, outside this
      // round's whitelist). A developer `gadak demo --addr 127.0.0.1:7899`
      // is the same fixture; reuse it. CI runners start with the port free
      // and still execute the command.
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
