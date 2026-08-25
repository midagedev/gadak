// DEV-only self-driving tour for media capture (lead tool, 2026-08-25).
//
// Synthetic touches from the host (cliclick) proved unreliable against the
// dev shell — the WKScrollView anchor mood shifts per launch (see the
// safe-area note in App.svelte), so screen→page coordinates cannot be
// trusted across takes. This drives the same journey from inside instead:
// store calls and DOM clicks, deterministic timings, no coordinates.
//
// Arming: create `mobile/public/__demo-tour__` (served at /__demo-tour__ by
// vite, absent in a packaged build) and relaunch the app. Delete the file to
// disarm. The module is imported from main.ts in DEV builds only and does
// nothing when the flag file is missing.
//
// The dark-mode flip stays outside (xcrun simctl ui booted appearance dark)
// — the app follows the system appearance; the tour leaves a steady beat
// for it between the search and the final queue scroll.

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
  await wait(2200) // hold: light queue, top

  glide(560) // queue scroll, two beats
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
  switchTab('queue')
  // beat for the external dark flip (t≈21.5s from tour start)
  await wait(3000)
  glide(560) // one scroll in the dark
  await wait(1600)
  glide(560)
}

export function armDemoTourInDev(): void {
  if (!import.meta.env.DEV) return
  void fetch('/__demo-tour__', { method: 'HEAD' })
    .then((r) => {
      if (r.ok) void tour()
    })
    .catch(() => {})
}
