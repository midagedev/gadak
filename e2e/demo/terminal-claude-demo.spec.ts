/**
 * Terminal-pane hero, live-Claude cut — scratch/terminal-hero.mp4 (0.18).
 *
 * Every earlier agent clip — agent.gif, claude-drive, claude-dashboards —
 * draws a *paper terminal* beside an app iframe, because gadak had no
 * terminal of its own and the agent in the story lived in some other window.
 * This take retires that composite: Claude Code runs in gadak's own pane, in
 * gadak's own window, and the board it moves is the one next to it.
 *
 * Nothing here is mocked and nothing is scripted output. The only things this
 * spec types are the two prompts a person would type; every command on screen
 * is Claude's own choice, and the list and the dashboard beside them are the
 * app reacting to what Claude actually wrote.
 *
 * The prompts are Korean on purpose. The mirror is English (examples/demo.db)
 * and the UI is English, so a Korean sentence landing on the right five rows
 * is the clip's second claim: the words you ask in are yours, the board is
 * still the team's.
 *
 * Beats:
 *   1. The list at rest — Epics, on a real mirror
 *   2. ⌘K → "Terminal" opens the pane; the rail names it and marks it Beta
 *   3. `claude` boots inside it
 *   4. "…담당한 이슈 중에 최근에 움직인 것 보여줘" — the list becomes that answer
 *   5. "이슈 라벨 비율 대시보드 만들어서 열어줘" — and the same pane paints a wall
 *
 * The artifact is deliberately not in docs/media: that directory is symlinked
 * into the website's public root, and the terminal is not announced on the
 * site or in the README while it ships Beta. See export-terminal.sh.
 *
 * Gated by GADAK_MEDIA=1, and driven by record-terminal-claude.sh — which
 * owns the serve, the isolated agent HOME and the frozen GADAK_HOME. Running
 * this config on its own attaches to whatever is on the port and will record
 * the operator's real home directory into the frame.
 *
 * Viewport and video size must stay 1440×900 (terminal-claude.config.ts) or
 * Playwright letterboxes the capture.
 */
import { test, expect, type Page } from '@playwright/test'
import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** Pause between beats so a human can read the frame. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/**
 * Focus the pane the way the e2e suite does: click the host, then focus the
 * helper textarea explicitly. The renderer paints on a canvas, so the click
 * alone lands on something that cannot hold a caret.
 */
async function focusPane(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    document
      .querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')
      ?.focus()
  })
}

/**
 * Type a prompt into Claude's TUI and submit it.
 *
 * Two deliberate pauses. The delay is a reading speed, not a throughput test.
 * The gap before Enter is what the tapes learned the hard way: Claude's input
 * box re-renders as it grows, and an Enter that lands mid-render is swallowed.
 */
async function ask(page: Page, prompt: string): Promise<void> {
  await focusPane(page)
  await page.keyboard.type(prompt, { delay: 55 })
  await beat(page, 900)
  await page.keyboard.press('Enter')
}

/** The whole terminal buffer, as text. */
async function readTerm(page: Page): Promise<string> {
  return page.evaluate(() => {
    const t = (
      window as unknown as {
        __gadakTerm?: {
          buffer: {
            active: {
              length: number
              getLine: (y: number) => { translateToString: (t?: boolean) => string } | undefined
            }
          }
        }
      }
    ).__gadakTerm
    if (!t) return ''
    const buf = t.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })
}

/**
 * The two prompts. Korean, and phrased the way someone actually asks — no
 * key, no JQL, no column names.
 *
 * The name is spelled in full on purpose. Take 1 asked for "다나" and Claude
 * stopped to ask which one it meant: the demo config's identity is
 * dana@example.com, the assignee in the mirror is Dana Whitfield, and the
 * changelog entries on those issues were written by Alex Kim. It was right to
 * ask, and a clarifying question is the one thing a 40-second clip has no
 * room for. Ambiguity in the prompt is the recorder's bug, not the model's.
 */
const ASK_ACTIVITY = 'Dana Whitfield이 담당한 이슈 중에 최근에 움직인 것 보여줘'
// "열어줘" is load-bearing, not politeness. Take 1 authored and saved a
// dashboard and stopped there, which is the correct reading of "만들어줘" and
// leaves the clip ending on a terminal. `dashboards open` is what takes the
// column, and it is a separate verb the skill teaches.
const ASK_DASHBOARD = '이슈 라벨 비율 대시보드 만들어서 열어줘'

test.describe('terminal claude demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the agent is in the window: two Korean prompts move the board', async ({ page }) => {
    // The pane's stored width is per-browser, and a fresh recording context
    // has none — the default ratio would open it at 634px here, and the take
    // wants the width to be a decision rather than a ratio. 640 is also the
    // first frame where the pane gets what it asks for: under the 1080-wide
    // cut the split clamped to ~428 (the list keeps a 390px minimum), which
    // is about 55 columns and wraps Claude's TUI into a column of stubs.
    await page.addInitScript(() => {
      try {
        localStorage.setItem('gadak.terminal.width', '640')
      } catch {
        /* private mode */
      }
    })
    await forceLocale(page, 'en')

    // Beat 1 — the list at rest.
    await page.goto('/#/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    const atRest = await page.getByTestId('list-count').textContent()
    await beat(page, 1600)

    // Beat 2 — ⌘K, "Terminal", Enter. The chord (Ctrl+`) is the shortcut a
    // regular carries; the palette is how the pane is *discovered*, and it
    // says on camera that this is a first-class command, not a hidden key.
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 700)
    await page.keyboard.type('terminal', { delay: 60 })
    await expect(palette.getByTestId('palette-action-terminal')).toBeVisible()
    await beat(page, 900)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // The beta mark came off in 0.19 (GDK-1024) — asserted absent, so a
    // revert cannot quietly put it back into a recording.
    await expect(page.getByTestId('terminal-beta')).toHaveCount(0)
    await beat(page, 1200)

    // Beat 3 — `claude`, in gadak's own shell. The TUI boot is the slow part;
    // the input box is what tells us it is ready to be typed into.
    await focusPane(page)
    await page.keyboard.type('claude', { delay: 90 })
    await beat(page, 500)
    await page.keyboard.press('Enter')
    await expect
      .poll(async () => readTerm(page), { timeout: 90_000, intervals: [1000] })
      .toMatch(/Welcome to Claude Code|for shortcuts|\? for shortcuts/i)
    await beat(page, 2000)

    // Beat 4 — a Korean sentence becomes the list. What Claude runs to get
    // there is its own; the skill teaches `views open`, and the app is
    // watching that handoff.
    await ask(page, ASK_ACTIVITY)
    await expect(page.getByTestId('list-count')).not.toHaveText(atRest ?? '', {
      timeout: 300_000,
    })
    await beat(page, 3000)

    // Beat 5 — and the same pane can paint a wall. dashboards open takes the
    // whole column, so this is the frame the clip ends on.
    await ask(page, ASK_DASHBOARD)
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 420_000 })
    await expect(page.getByTestId('dashboard-frame')).toBeVisible({ timeout: 60_000 })

    // The wall has to *render*, not just exist. The take before this one
    // saved and opened a dashboard whose every card read "undefined" and
    // "NaN%" — the datasource SQL was right and the page never got the rows.
    // A contract that only asks "is a dashboard open" passes that take, so
    // this asks what the eye asks.
    const wall = page.frameLocator('[data-testid="dashboard-frame"]').locator('body')
    await expect(wall).toContainText(/\d/, { timeout: 60_000 })
    await expect(wall).not.toContainText(/undefined|NaN/i)
    await beat(page, 4000)
  })
})
