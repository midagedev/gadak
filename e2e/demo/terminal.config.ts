import { defineConfig, devices } from '@playwright/test'

/**
 * Terminal-pane hero (0.18). Separate from e2e/demo/playwright.config.ts so
 * `make media-web` cannot pick up this video.webm by accident.
 *
 * Run via `make media-terminal` (sets GADAK_MEDIA=1).
 *
 * 1080x1350 (4:5), not the 1280x800 the other demo configs use: this take is
 * posted to Twitter, and a vertical frame is what the timeline gives the most
 * height to. It also suits the subject — the terminal is a bottom pane, so
 * stacking list over shell is the app's own layout, not a promo composite.
 * Same numbers as the tokens/dashboards vertical cuts (promo-split.ts), which
 * is where the 4:5 social frame was measured. Playwright letterboxes when the
 * video size and the viewport disagree, so the two move together or not at all.
 *
 * Port 7793, not the suite's 7877: `views open` hands off through a
 * *one-shot* file that the first poller to ask consumes, so any other client
 * on the same serve — a desktop window, another tab — silently eats the hash
 * and the list never moves. Measured 2026-08-26: with the desktop app open on
 * 7877 the file was written and gone in 240ms, and three takes failed with a
 * clean terminal and an unchanged list. 7793 is inside serveProbePorts()'
 * 7777-7797 sweep (cmd/gadak/views.go) so the CLI still finds the serve.
 *
 * The webServer line does three things the other demo configs do not, all
 * of them about what the PTY inherits:
 *
 *   PATH  — the pane's shell must find the `gadak` this take builds
 *           (e2e/serve.sh puts it in e2e/.tmp), or the commands on camera
 *           would have to be absolute paths.
 *   SHELL — /bin/sh, not the recorder's login shell. A personal zsh brings
 *           its own prompt, colors and plugins into the frame: not
 *           reproducible, and a hostname or a directory is exactly the kind
 *           of thing MEDIA.md's privacy rule exists to keep out.
 *   PS1/ENV/HISTFILE — a bare `$ ` prompt, no startup file, no history on
 *           disk. `ENV=/dev/null` is what stops sh from sourcing one.
 *   GADAK_E2E_ORIGIN=builtin — the mirror is migrated onto the built-in
 *           tracker so the `gadak claim` beat is a write that lands; the
 *           fixture's Jira credential is fake and a claim against it fails
 *           before the roster can pick the key up (GDK-1353).
 */
export default defineConfig({
  testDir: '.',
  testMatch: 'terminal-demo.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 240_000,
  expect: { timeout: 60_000 },
  outputDir: 'test-results-terminal',
  use: {
    baseURL: 'http://127.0.0.1:7793',
    locale: 'en-US',
    colorScheme: 'light',
    viewport: { width: 1080, height: 1350 },
    deviceScaleFactor: 2,
    video: {
      mode: 'on',
      size: { width: 1080, height: 1350 },
    },
    launchOptions: { slowMo: 30 },
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
    trace: 'on',
  },
  webServer: {
    command:
      'GADAK_E2E_PORT=7793 GADAK_FRESHEN=1 GADAK_E2E_ORIGIN=builtin PATH="$PWD/e2e/.tmp:$PATH" SHELL=/bin/sh ' +
      'ENV="$PWD/e2e/demo/prompt.sh" HISTFILE=/dev/null bash e2e/serve.sh',
    url: 'http://127.0.0.1:7793/healthz',
    reuseExistingServer: false,
    timeout: 180_000,
    cwd: '../..',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        colorScheme: 'light',
        viewport: { width: 1080, height: 1350 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
