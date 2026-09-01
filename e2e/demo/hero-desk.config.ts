import { defineConfig, devices } from '@playwright/test'

/**
 * Hero desk bits 1 & 6 (0.19, GDK-1037 B-loop: "자리를 비워도 일은 계속된다")
 * — one continuous take: prompt the agent in gadak's own terminal pane, close
 * the pane (detach), let the agent finish unseen, reopen (reattach, same
 * session), end on the board with the issue done.
 *
 * Sister of terminal-claude.config.ts — same window shape (1440×900 16:10,
 * a laptop window, which is what "at my desk" means on camera), same video
 * size (Playwright letterboxes when the two disagree), same single chromium
 * project.
 *
 * No webServer block: record-hero-desk.sh owns the serve, because the PTY has
 * to inherit an environment Playwright cannot express — the throwaway agent
 * HOME, a local-origin (writable, seed-only) GADAK_HOME, and the CLAUDE_* /
 * GADAK_E2E_PORT unsets. It starts the serve from that environment and this
 * config attaches to it.
 *
 * Port 7794, the same assignment the spec gave this league (7877/7891/7892/
 * 7795 are other rounds'). It sits inside serveProbePorts()' 7777-7797 sweep
 * (cmd/gadak/views.go) so `gadak` inside the pane still finds the serve, and
 * off the e2e suite's 7877 so a parallel suite run cannot eat anything here.
 *
 * The timeout is generous on purpose: a live model turn is the long pole, and
 * the away-wait alone is 45-70s of real clock.
 */
const PORT = process.env.GADAK_E2E_PORT || '7794'

export default defineConfig({
  testDir: '.',
  testMatch: 'hero-desk.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 900_000,
  expect: { timeout: 60_000 },
  outputDir: '../.tmp/test-results-hero-desk',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1440, height: 900 },
    },
    launchOptions: { slowMo: 0 },
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
    trace: 'off',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        colorScheme: 'light',
        viewport: { width: 1440, height: 900 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
