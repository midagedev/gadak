// Keyboard-aware surfaces (DESIGN.md §4.2). In WKWebView — and iOS Safari —
// the software keyboard overlays the layout viewport: the page does not
// resize, so everything the covered band holds is unreachable. The
// VisualViewport API is the one honest measurement of the obscured band,
// and since GDK-1971 this module is its single owner: one formula
// (measure), two consumers —
//
//   keyboardInset    a Svelte action that translates ONE node (a composer,
//                    a sheet panel, the key bar) up by the band, because a
//                    composer must ride ABOVE the keys and padding cannot
//                    do that. Four call sites, unchanged.
//   bindKeyboardBand a root binding that publishes the band as the
//                    --keyboard-inset CSS variable on #app, so every scroll
//                    container can pad itself out of the band (Screen's
//                    main, the sheet panels) instead of each surface
//                    re-measuring it.
//
// The ?kb=<px> URL param is the dev/e2e probe (the ?hosted idiom,
// lib/runtime.ts): when present it replaces the VisualViewport as the
// band's source with a fixed inset, so a rig with no software keyboard can
// still drive the whole band path. Read per call and honored in every
// build, exactly as ?hosted is.
//
// window.__gadakKeyboard answers "what did the band measure last": the
// inset, the inputs it was derived from, and whether they came from the
// viewport or the probe.

/** The VisualViewport, as narrow as this module needs it. Tests inject
 *  fakes with the same shape (the back.ts / runtime.ts seam idiom). */
export type VvLike = {
  readonly height: number
  readonly offsetTop: number
  addEventListener: (type: 'resize' | 'scroll', listener: () => void) => void
  removeEventListener: (type: 'resize' | 'scroll', listener: () => void) => void
}

/** The window, ditto. */
export type WinLike = { readonly innerHeight: number }

/**
 * One formula, one owner (GDK-1971): the band the keyboard covers, in px.
 * innerHeight is the layout viewport (which iOS keeps fixed); vv.height is
 * what remains visible; offsetTop is how far the visible box slid (iOS
 * scrolls the visual viewport under focused fields). Clamped at 0 — a
 * hardware keyboard or Android's resizes-content (index.html) reads 0, and
 * the same code is then right on both platforms.
 */
export function measure(
  vv: { height: number; offsetTop: number },
  win: { innerHeight: number },
): number {
  return Math.max(0, win.innerHeight - vv.height - vv.offsetTop)
}

/** The probe value in a query string (?kb=300), or null when absent or
 *  not a non-negative number. Pure so a test needs no location. */
export function probeKbInset(search: string): number | null {
  const raw = new URLSearchParams(search).get('kb')
  if (raw === null) return null
  const n = Number(raw)
  return Number.isFinite(n) && n >= 0 ? n : null
}

/** The live probe: the page's search string, read per call so a stubbed
 *  location (tests) or a later navigation is honored. */
function probedInset(): number | null {
  if (typeof location === 'undefined') return null
  return probeKbInset(location.search)
}

/** What window.__gadakKeyboard holds after every measurement (Layer 3).
 *  vvHeight/offsetTop are null on the probe path — there was no viewport
 *  to read them from, and inventing one would be the lie this object
 *  exists to prevent. */
export type KeyboardDebug = {
  inset: number
  vvHeight: number | null
  innerHeight: number
  offsetTop: number | null
  at: number
  source: 'vv' | 'probe'
}

declare global {
  interface Window {
    __gadakKeyboard?: KeyboardDebug
  }
}

function publish(debug: KeyboardDebug): void {
  if (typeof window === 'undefined') return
  window.__gadakKeyboard = debug
}

/**
 * The action (unchanged contract for its four callers: Sheet, KeyBar, the
 * Detail and PageDetail composers): translate the node up by exactly the
 * band while the keyboard is up, and stamp data-keyboard-inset so app.css
 * can withdraw a bottom-most surface's home-indicator clearance while it
 * rides above the keys (GDK-902). No-op without a VisualViewport and
 * without the probe (headless capture, desktop browsers).
 */
export function keyboardInset(
  node: HTMLElement,
  vv: VvLike | null | undefined = window.visualViewport,
  win: WinLike = window,
) {
  const probe = probedInset()
  if (probe !== null) {
    node.style.transform = `translateY(-${probe}px)`
    node.dataset.keyboardInset = ''
    publish({
      inset: probe,
      vvHeight: null,
      innerHeight: win.innerHeight,
      offsetTop: null,
      at: Date.now(),
      source: 'probe',
    })
    return {
      destroy() {
        node.style.transform = ''
        delete node.dataset.keyboardInset
      },
    }
  }
  if (!vv) return
  const update = () => {
    const inset = measure(vv, win)
    node.style.transform = inset > 0 ? `translateY(-${inset}px)` : ''
    // GDK-902 2026-09-15 — the same measurement, published as a selector.
    //
    // A node that rides this action is bottom-most only while the band is
    // closed: with the keyboard up it has been translated above it, and the
    // clearance a bottom-most surface owes the home indicator becomes a dead
    // strip between the node and the keys (the defect
    // `.composer.safe-bottom:focus-within` exists to remove). CSS cannot ask
    // the VisualViewport, and `:focus-within` is the wrong question — the key
    // bar's focus lives in a sibling field, and a hardware keyboard would
    // answer "up" for a node still sitting on the home indicator. This action
    // already holds the honest answer, so it stamps it and app.css reads it.
    // Nothing else consumes the attribute; the sheets and composers that also
    // use this action are unaffected.
    if (inset > 0) node.dataset.keyboardInset = ''
    else delete node.dataset.keyboardInset
    publish({
      inset,
      vvHeight: vv.height,
      innerHeight: win.innerHeight,
      offsetTop: vv.offsetTop,
      at: Date.now(),
      source: 'vv',
    })
  }
  vv.addEventListener('resize', update)
  vv.addEventListener('scroll', update)
  update()
  return {
    destroy() {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
      node.style.transform = ''
      delete node.dataset.keyboardInset
    },
  }
}

/**
 * The root binding (GDK-1971): one subscription, one write. App binds this
 * to #app on mount; the band lands as --keyboard-inset where every scroller
 * and sheet panel reads it, and as data-keyboard-up for any future
 * selector that needs "the keyboard is up" on the frame itself (distinct
 * from the action's per-node data-keyboard-inset, which app.css already
 * owns). The probe replaces the viewport as the source when armed.
 */
export function bindKeyboardBand(
  root: HTMLElement,
  vv: VvLike | null | undefined = window.visualViewport,
  win: WinLike = window,
): () => void {
  const write = (inset: number, debug: KeyboardDebug): void => {
    root.style.setProperty('--keyboard-inset', `${inset}px`)
    if (inset > 0) root.dataset.keyboardUp = ''
    else delete root.dataset.keyboardUp
    publish(debug)
  }
  const reset = (): void => {
    root.style.removeProperty('--keyboard-inset')
    delete root.dataset.keyboardUp
  }
  const probe = probedInset()
  if (probe !== null) {
    write(probe, {
      inset: probe,
      vvHeight: null,
      innerHeight: win.innerHeight,
      offsetTop: null,
      at: Date.now(),
      source: 'probe',
    })
    return reset
  }
  if (!vv) return () => {}
  const update = () => {
    const inset = measure(vv, win)
    write(inset, {
      inset,
      vvHeight: vv.height,
      innerHeight: win.innerHeight,
      offsetTop: vv.offsetTop,
      at: Date.now(),
      source: 'vv',
    })
  }
  vv.addEventListener('resize', update)
  vv.addEventListener('scroll', update)
  update()
  return () => {
    vv.removeEventListener('resize', update)
    vv.removeEventListener('scroll', update)
    reset()
  }
}
