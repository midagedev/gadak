// DEV-only self-driving tour for media capture (lead tool, 2026-08-25).
//
// Synthetic touches from the host (cliclick) proved unreliable against the
// dev shell — the WKScrollView anchor mood shifts per launch (see the
// safe-area note in App.svelte), so screen→page coordinates cannot be
// trusted across takes. This drives the same journey from inside instead:
// store calls and DOM clicks, deterministic timings, no coordinates.
//
// Arming: open the DEV origin with `?demo-tour` (any value) and reload.
// A HEAD/file probe at /__demo-tour__ was abandoned: mobile/public/ does
// not exist, and vite's SPA fallback serves index.html with 200 for every
// unmatched path, so r.ok was always true and the tour drove every
// `npm run dev` boot with no way to disarm. A query parameter cannot be
// forged by a route. Omit the param to disarm. Packaged builds never run
// this (`import.meta.env.DEV` below). The module is imported from main.ts
// in DEV builds only.
//
// The dark-mode flip stays outside (xcrun simctl ui booted appearance dark)
// — the app follows the system appearance; the tour leaves a steady beat
// for it between the search and the final Issues scroll.

import { app, openIssue, closeIssue, switchTab } from './store.svelte'

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** Smooth-scroll the visible screen's scroller by `dy`, over ~600ms. */
function glide(dy: number): void {
  const el = document.querySelector<HTMLElement>(
    '.detail-layer .scroll, .detail-layer [data-scroll], .pane:not(.off) .scroll, .pane:not(.off) [data-scroll], .detail-layer main, .pane:not(.off) main',
  )
  const scroller = el ?? scrollableIn(document.body)
  scroller?.scrollBy({ top: dy, behavior: 'smooth' })
}

/** First descendant that actually scrolls vertically. */
function scrollableIn(root: HTMLElement): HTMLElement | null {
  const all = root.querySelectorAll<HTMLElement>('*')
  for (const el of all) {
    if (el.scrollHeight > el.clientHeight + 40) {
      const oy = getComputedStyle(el).overflowY
      if (oy === 'auto' || oy === 'scroll') return el
    }
  }
  return null
}

async function tour(): Promise<void> {
  await wait(2200) // hold: light Issues list, top

  glide(560) // list scroll, two beats
  await wait(1400)
  glide(560)
  await wait(1400)
  glide(-1120) // back to top
  await wait(1200)

  const firstRow = document.querySelector<HTMLButtonElement>('.pane:not(.off) button.row')
  if (firstRow) firstRow.click()
  else openIssue('NMS-134') // demo fixture's first Highest row
  await wait(2600)
  glide(480) // read the body
  await wait(1800)
  glide(-480)
  await wait(900)
  closeIssue()
  await wait(1300)

  switchTab('search')
  await wait(2000)
  switchTab('pairing')
  await wait(2200)
  switchTab('issues')
  // beat for the external dark flip (t≈21.5s from tour start)
  await wait(3000)
  glide(560) // one scroll in the dark
  await wait(1600)
  glide(560)
}

/** True only in DEV when the page URL carries `?demo-tour`. */
export function isDemoTourArmed(): boolean {
  if (!import.meta.env.DEV) return false
  if (typeof location === 'undefined') return false
  return new URLSearchParams(location.search).has('demo-tour')
}

export function armDemoTourInDev(): void {
  if (!import.meta.env.DEV) return
  if (!isDemoTourArmed()) return
  console.info('gadak demo tour: driving the app (armed by ?demo-tour)')
  void tour()
}
