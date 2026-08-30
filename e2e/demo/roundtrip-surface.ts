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
 * Pane width. 400px is what is left after the board's three 288px columns and
 * the narrowed sidebar inside a 1472px frame — the board, not the terminal,
 * sets this number now. It holds ~35 columns at the fixture's 19px, which
 * fits `gadak claim STD-7` and `bound to session …` unwrapped. From the
 * moment the detail panel docks, the layout hands the pane its
 * TERMINAL_MIN_WIDTH_PX floor of 320 whatever is stored here.
 */
export const PANE_WIDTH = '648'

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
