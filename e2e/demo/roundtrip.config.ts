import { defineConfig, devices } from '@playwright/test'

/**
 * The 0.19 round-trip cut (GDK-1159): one issue, one shell, one window —
 * `gadak claim STD-7` typed in gadak's own terminal pane moves the card on the
 * board in the same breath, the issue's body hands a command back to that
 * shell, and `gadak close` lands the card in Done.
 *
 * Why 1472×994. The frame this feeds is 1920×1296, and 1472×994 is that frame
 * at 1.304× — the capture fills the delivery frame with no bars and no crop,
 * so nothing is enlarged in post. The scale factor is what matters, not 16:9:
 * the frame is 20% taller than the 828/1080 pair it replaces (maker's call —
 * a taller frame gives the dock more scrollback, and a vertical crop is
 * allowed on the feed this ships to). That scale is the whole readability fix
 * from the v0.19 post-mortem (release-video.md): the
 * hero's 1440×900 could only scale by 1.2 before it ran out of frame width, and
 * a row of body text landed at 1.1% of frame height. Measured here: the detail
 * body's 21.06px line becomes 28px in the delivered frame (2.2%), and the terminal's own text
 * — the line the film is about — is raised to 19px by the fixture
 * (`ui.tokens.type.--text-terminal`, a shipped setting, not a camera trick),
 * landing at 30px, above the 28px the body line reaches.
 *
 * Width is not free to shrink, and the board set the floor. The three columns
 * are 288px each (BoardColumn.svelte — not tokenised), the pane is 400, and
 * the sidebar is narrowed to its own shipped 208px value: 208 + 400 + 864 =
 * 1472 exactly. A pixel less and the Done column — where the film ends — is
 * off frame. It also clears TERMINAL_SPLIT_WITH_DETAIL_MIN_PX (1420 =
 * VIEWPORT_DOCKED_MIN_PX 1100 + TERMINAL_MIN_WIDTH_PX 320, layout.ts), below
 * which the pane stops being a split and covers the detail panel whole —
 * measured at 1100, where beat 2 (the ▶ and the shell it lands in) could not
 * be in one frame at all.
 *
 * No webServer block, same as hero-desk.config.ts: record-roundtrip.sh owns the
 * serve, because the PTY inherits an environment Playwright cannot express.
 * Port 7794 is this league's assignment and sits inside serveProbePorts()'
 * 7777-7797 sweep (cmd/gadak/views.go), so `gadak` typed in the pane finds the
 * serve it is filmed against.
 */
const PORT = process.env.GADAK_E2E_PORT || '7794'

export default defineConfig({
  testDir: '.',
  testMatch: 'roundtrip.spec.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  // Four live Claude Code sessions boot (~1m30s each, in flight together)
  // and then investigate before the first frame of the cut is even usable.
  // The budget is theirs; the polls (SHELL_POLL_MS 4s, ROSTER_POLL_MS 2s)
  // and the holds are rounding error next to it.
  timeout: 1_500_000,
  expect: { timeout: 30_000 },
  outputDir: '../.tmp/test-results-roundtrip',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    locale: 'en-US',
    colorScheme: (process.env.GADAK_RT_SCHEME as 'light' | 'dark') || 'light',
    viewport: { width: 1472, height: 994 },
    deviceScaleFactor: 2,
    video: { mode: 'on', size: { width: 1472, height: 994 } },
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
        colorScheme: (process.env.GADAK_RT_SCHEME as 'light' | 'dark') || 'light',
        viewport: { width: 1472, height: 994 },
        deviceScaleFactor: 2,
      },
    },
  ],
})
