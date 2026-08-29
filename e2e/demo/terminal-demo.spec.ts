/**
 * Terminal-pane hero for scratch/terminal-hero.mp4 (0.18).
 *
 * The artifact is deliberately not in docs/media: that directory is symlinked
 * into the website's public root, and the terminal is not announced on the
 * site or in the README while it ships Beta. See export-terminal.sh.
 *
 * Every earlier split clip — agent.gif, tokens.gif — draws a *paper
 * terminal* beside an app iframe, because gadak had no terminal of its
 * own and the shell in the story lived in some other window. This take
 * exists to retire that composite: the shell is in the app now, so the
 * same beat happens in one frame with a real PTY.
 *
 * Nothing here is mocked. The commands are typed into the pane, run in
 * the session's own shell, and the list beside them is the app reacting
 * to what they printed.
 *
 * Beats:
 *   1. The list at rest — All open, on a real mirror
 *   2. ⌘K → "Terminal" opens the pane; the rail names it and marks it Beta
 *   3. `gadak sql … | gadak views open --keys -` — the answer becomes the
 *      list next to it
 *   4. `gadak views open --jql …` — the same promise from the query
 *      language a Jira user already has
 *   5. Ctrl+` again; the list keeps the view the shell put there
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1280×720
 * (see terminal.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** Pause between beats so a human can read the frame. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/**
 * Focus the pane the way the e2e suite does: click the host, then focus
 * the helper textarea explicitly. The renderer paints on a canvas, so the
 * click alone lands on something that cannot hold a caret.
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

/** Type at a readable speed — this is a recording, not a throughput test. */
async function typeLine(page: Page, line: string, delay = 26): Promise<void> {
  await focusPane(page)
  await page.keyboard.type(line, { delay })
  await beat(page, 500)
  await page.keyboard.press('Enter')
}

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

const STUCK_PIPE =
  'gadak sql --no-header "select key from issues where status_category=' +
  "'inprogress' order by status_changed_at asc limit 5\" | gadak views open --keys - --no-open"

const JQL_LINE =
  "gadak views open --no-open --jql 'project = NMA AND priority = High AND resolution is EMPTY'"

test.describe('terminal demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the shell is in the window: a pipe and a JQL become the list', async ({ page }) => {
    test.setTimeout(180_000)
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    await beat(page, 1400)

    // Beat 2 — ⌘K, "Terminal", Enter. The chord (Ctrl+`) is the shortcut a
    // regular carries; the palette is how the pane is *discovered*, and it
    // says on camera that this is a first-class command and not a hidden
    // key. The rail then carries the glyph and the beta mark, which is the
    // honest state of this pane in 0.18.
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 700)
    await page.keyboard.type('terminal', { delay: 60 })
    const paletteRow = palette.getByTestId('palette-action-terminal')
    await expect(paletteRow).toBeVisible()
    await beat(page, 900)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // The beta mark came off in 0.19 (GDK-1024) — asserted absent, so a
    // revert cannot quietly put it back into a recording.
    await expect(page.getByTestId('terminal-beta')).toHaveCount(0)
    await beat(page, 1100)

    // Beat 3 — any answer you can compute is a view.
    await typeLine(page, STUCK_PIPE)
    await expect.poll(async () => readTerm(page), { timeout: 30_000 }).toContain('ks=')
    await expect(page.getByTestId('list-count')).toHaveText(/\b5\b/, { timeout: 30_000 })
    await beat(page, 2200)

    // Beat 4 — and you do not have to leave JQL to get one. Two beats
    // because the pipe alone reads as "gadak wants SQL".
    await typeLine(page, 'clear')
    await beat(page, 400)
    await typeLine(page, JQL_LINE)
    await expect.poll(async () => readTerm(page), { timeout: 30_000 }).toContain('pj=NMA')
    await expect(page.getByTestId('list-count')).toBeVisible()
    await beat(page, 2400)

    // Beat 5 — the pane closes, the view it produced stays.
    await page.keyboard.press('Control+Backquote')
    await expect(pane).toBeHidden()
    await beat(page, 1600)

    expect(errors, `console errors: ${errors.join('\n')}`).toEqual([])
  })
})
