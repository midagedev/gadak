/**
 * WHAT the release take films — the surface under the camera.
 *
 * Split out of roundtrip.spec.ts so the two halves can move independently
 * (GDK-1159, lead call 2026-08-30): the spec is the choreography ("hand over
 * an issue, watch it cross the board"), and this file is the only place that
 * knows *which pixels* say so. When the kanban board lands (GDK-761), the
 * climax is re-shot against it by swapping this module — a `board.ts` with
 * the same six exports — and the spec, the seeding, the marks and the cut
 * pipeline are untouched.
 *
 * The rule that keeps that promise: **nothing here knows about beats, and
 * nothing in the spec knows about selectors.** If a helper needs a mark or a
 * hold, it belongs in the spec; if the spec needs a `data-testid`, it belongs
 * here.
 *
 * There is deliberately no interface and no registry. One implementation
 * exists and one more is expected; a plugin layer for two files is the exact
 * over-build this split is meant to avoid.
 */
import { expect, type Locator, type Page } from '@playwright/test'

/**
 * The route the board is filmed on.
 *
 * `ly=board` is the kanban stage (GDK-1175). `sc=new,inprogress,done` is not
 * decoration: the default view filters done OUT, so the last beat would end
 * with the card *vanishing* instead of landing — the trap two hero takes fell
 * into (hero-desk.spec.ts §7). `g=status_category` is explicit rather than
 * inherited so the three columns are New / In progress / Done whatever a
 * saved view or a future default does.
 */
export const BOARD_ROUTE = '/#/?sc=new,inprogress,done&g=status_category&ly=board'

/**
 * Dock height (GDK-1194 — the pane is a bottom dock, so the number the film
 * seeds is a height now; the old 648px width is gone with the side pane).
 * ~340px inside the 828px frame leaves the board its three rows of cards and
 * still gives the shell a dozen lines, and the dock spans the whole content
 * width so nothing it prints wraps.
 *
 * The film framing itself has not been re-cut for the dock — that is a
 * capture round, not this change.
 */
export const PANE_HEIGHT = '340'

/** The board is ready to be filmed. */
export async function boardReady(page: Page): Promise<void> {
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('board')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('board-card').first()).toBeVisible({ timeout: 30_000 })
}

/** The one issue the film follows, as a thing the camera can point at. */
export function card(page: Page, key: string): Locator {
  return page.locator(`[data-board-key="${key}"]`)
}

/**
 * Where the card sits right now: the key of the column it is in
 * (`new` | `inprogress` | `done` — the status_category axis the board is
 * grouped on, so this needs no display-name translation, CLAUDE.md).
 *
 * Null means "not rendered", never "moved": the column bodies scroll, so a
 * card outside the viewport is simply absent from the DOM.
 */
export async function columnOf(page: Page, key: string): Promise<string | null> {
  return page.evaluate((k) => {
    const card = document.querySelector<HTMLElement>(`[data-board-key="${k}"]`)
    return card?.closest<HTMLElement>('[data-board-column]')?.dataset.boardColumn ?? null
  }, key)
}

/**
 * True while the card is mid-flight from a move this browser did NOT make.
 *
 * The board flies a card for 260ms and glows its actor chip for 1.2s when the
 * move arrived on the mirror tick rather than from a click here (GDK-1175,
 * BoardView.svelte:130-139). That marker is the climax's whole proof: it is
 * the difference between "an agent moved this" and "someone clicked it".
 */
export function movedByOther(page: Page, key: string): Locator {
  return page.locator(`[data-board-key="${key}"][data-moved="1"]`)
}

/**
 * Bring the column a finished card landed in into view.
 *
 * On the list this was load-bearing — measured, the payoff row sat on the
 * frame's last twelve pixels and the film's final image arrived cropped. The
 * board puts all three columns in frame at once, so it is a no-op here.
 */
export async function revealDone(page: Page): Promise<void> {
  // The board's Done column is always on screen — there is nothing to reveal.
  // Kept so the spec's beats do not have to know which stage they are on.
  void page
}

/**
 * The truth behind the pixels: the issue's status_category, straight off the
 * bootstrap API. Every gate in the spec pairs a camera-side check with this
 * one, because a group header's text is what the camera sees and this is what
 * is true — and because a display name is 0 rows on a Korean account
 * (CLAUDE.md).
 */
export async function categoryOf(page: Page, key: string): Promise<string | null> {
  const res = await page.request.get('/api/v1/issues/bootstrap/')
  expect(res.ok(), `bootstrap GET: ${res.status()}`).toBeTruthy()
  const body = (await res.json()) as {
    issues?: Array<{ key?: string; status_category?: string }>
  }
  return (body.issues ?? []).find((i) => i.key === key)?.status_category ?? null
}

/**
 * The tab/row that names a session by its issue key, and the list of those
 * names.
 *
 * Today the terminal is a side pane and these are rows in its session strip;
 * with the bottom dock (GDK-1194) they become tabs in a ribbon. They live
 * here rather than in the spec so that swap is a change to this file only —
 * the choreography asks "the thing that names this session", never "the strip
 * row".
 */
export function sessionTab(page: Page, key: string): Locator {
  // Exact on the NAME, not `hasText` on the row. `hasText: 'STD-1'` is a
  // substring match, so it also matches the STD-14 row — and `.first()` then
  // hands back STD-14, since it is created earlier. Measured 2026-08-31: two
  // takes came back with STD-1's investigation sitting inside STD-14's shell
  // and STD-1 never selected at all, because every click meant for STD-1 had
  // been landing on its longer-keyed neighbour.
  return page.getByTestId('terminal-strip-row').filter({
    has: page.getByTestId('terminal-strip-name').filter({ hasText: new RegExp(`^${key}$`) }),
  })
}

export function sessionTabNames(page: Page): Promise<string[]> {
  return page.getByTestId('terminal-strip-name').allTextContents()
}

/**
 * The two ways INTO an issue's session that are not the session strip itself
 * (GDK-1196 / GDK-1197). The film's recovery beats use these because the
 * argument is "I come back from the issue", not "I click a terminal tab":
 * the card's hover-revealed glyph, and the ⌘K palette's shell row under the
 * issue. Selectors are pinned by e2e/session-entry.spec.ts — that spec is the
 * contract, this is the camera's handle on it.
 */
export function cardShellEnter(page: Page, key: string): Locator {
  return page
    .locator(`[data-testid="board-card"][data-board-key="${key}"]`)
    .getByTestId('board-card-shell-enter')
}

export function paletteShellRow(page: Page): Locator {
  return page.getByTestId('palette-shell-row')
}

/**
 * True when the card is fully above the dock — the frame the recovery beat
 * exists for is the card and *that card's shell* stacked in one image. A card
 * scrolled under the dock (or out of the DOM entirely, see columnOf) makes the
 * beat a terminal switch with no visible cause.
 */
export async function cardAboveDock(page: Page, key: string): Promise<string> {
  return page.evaluate((k) => {
    const c = document.querySelector<HTMLElement>(`[data-board-key="${k}"]`)
    const dock = document.querySelector<HTMLElement>('[data-testid="terminal-pane"]')
    if (!c) return 'card not rendered'
    if (!dock) return 'dock not rendered'
    const cb = c.getBoundingClientRect()
    const db = dock.getBoundingClientRect()
    // The geometry travels with the verdict: a bare false sends the next take
    // out to measure what this one already knew.
    return cb.bottom <= db.top + 1
      ? 'ok'
      : `card bottom ${Math.round(cb.bottom)} > dock top ${Math.round(db.top)}`
  }, key)
}

/** How many session tabs are clipped out of view. The chaos beat needs 0 —
 *  a roster that hides its own rows is not a roster (GDK-1193). The dock's
 *  strip scrolls sideways (TerminalStrip.svelte, `overflow-x-auto`), so the
 *  fold is the right edge now, not the bottom one. */
export function tabsBelowFold(page: Page): Promise<number> {
  return page.evaluate(() => {
    const strip = document.querySelector('[data-testid="terminal-strip"]')
    if (!strip) return -1
    const box = strip.getBoundingClientRect()
    return [...strip.querySelectorAll('[data-testid="terminal-strip-row"]')]
      .filter((r) => r.getBoundingClientRect().right > box.right + 1).length
  })
}
