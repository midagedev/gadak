import { defineConfig, devices } from '@playwright/test'

/**
 * Terminal-pane hero, live-Claude cut (0.18) — the Twitter clip.
 *
 * The shell in the window is not typing gadak commands: it is running Claude
 * Code, and Claude is the one calling gadak. That is the whole point of the
 * pane — the agent and the board it drives are finally in one frame, where
 * every earlier clip had to draw a paper terminal beside an app iframe.
 *
 * 1440x1080 (4:3). The first cut of this used 1080x1350, the 4:5 social
 * frame the tokens / dashboards cuts use — and that was the wrong frame for
 * *this* subject. Those clips shoot one column; this one shoots a shell and
 * the board it drives, side by side, and a portrait crop squeezes a
 * three-column app until neither half is readable. 4:3 still takes far more
 * of a phone timeline than 16:9 and gives the terminal its full width beside
 * a list that is still a list. Playwright letterboxes when the video size and
 * the viewport disagree, so the two move together or not at all.
 *
 * No webServer block: record-terminal-claude.sh owns the serve, because the
 * PTY has to inherit an environment Playwright cannot express — the isolated
 * agent HOME (a throwaway login), the frozen demo GADAK_HOME, the neutral
 * cwd, and the CLAUDE_* unsets. It starts the serve from that environment and
 * this config attaches to it.
 *
 * Port 7794, not the suite's 7877: `views open` hands its hash through a
 * *one-shot* file that the first poller to ask consumes, so any other client
 * on the same serve — a desktop window, another tab — silently eats it and
 * the list never moves (measured 2026-08-26: written and gone in 240ms with
 * the desktop app open). 7794 is inside serveProbePorts()' 7777-7797 sweep
 * (cmd/gadak/views.go) so gadak still finds the serve from inside the pane.
 */
const PORT = process.env.GADAK_E2E_PORT || '7794'

export default defineConfig({
  testDir: '.',
  testMatch: 'terminal-claude-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  // A live model turn is the long pole, not the app. Two prompts at up to
  // 300s each plus boot and holds.
  timeout: 900_000,
  expect: { timeout: 60_000 },
  outputDir: '../.tmp/test-results-terminal-claude',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: 1440, height: 1080 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1440, height: 1080 },
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
        viewport: { width: 1440, height: 1080 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
