// System back (DESIGN.md §2). Every screen has an explicit way out;
// the hardware / edge-swipe back is that same edge, not a second one.
//
// One owner: this module. Sheets register the same onclose their Cancel
// and scrim already call; the detail push is closeIssue() from the store.
// Components do not install popstate listeners.
//
// The History API is a trap, not a stack. We keep a sentinel entry so
// Android's default (finish the activity) never fires, and a popstate
// runs the same close the visible control would. Linked-issue navigation
// replaces the detail key in the store (DESIGN.md §2) — this module
// replaces the frame's key too (GDK-1970), so one back still returns to
// the list.
//
// At the root (no sheet, no push layer, no detail) back is a no-op —
// the list under its owner IS the root and DESIGN.md §2 gives it no exit.
// Unpair is the explicit way out of a pairing. Consuming the gesture is
// what stops the activity from finishing.
//
// GDK-902: the palette is a fourth thing back can close, and it is NOT
// registered here as a sheet — sheets outrank the detail in peekBack, and
// a row tapped out of the palette opens a Detail over it. Its order lives
// in the store's closeTop (detail → layer → palette), which App binds.
//
// GDK-1970: the hosted phone is a browser page, and iOS Safari's edge
// swipe first slides to the previous entry's snapshot, then fires
// popstate — with the detail living only in store state, that previous
// entry is whatever the browser had below the page, and a second swipe
// leaves the app. So the detail is now a real history entry: a
// { gadakDetail } frame pushed over the sentinel with a #/KEY hash, the
// URL kept in step by syncDetail (which App feeds from app.detail), and
// the ← control closing through back() so the address follows the screen.
// The gesture and the control remain one edge — both end at the sentinel.
//
// History is an injectable seam so a unit test can fire a pop without
// a browser (same shape as ime.ts / keys.ts).

export type BackKind = 'sheet' | 'detail' | 'root'

export type BackSnapshot = {
  sheetCount: number
  hasDetail: boolean
}

export function peekBack(snap: BackSnapshot): BackKind {
  if (snap.sheetCount > 0) return 'sheet'
  if (snap.hasDetail) return 'detail'
  return 'root'
}

/** The store's app.detail shape, re-declared so this module owns history
 *  without importing the store. */
export type DetailRef = { kind: 'issue' | 'page'; key: string }

export type HistorySeam = {
  readonly state: unknown
  pushState: (data: unknown, unused: string, url?: string) => void
  replaceState: (data: unknown, unused: string, url?: string) => void
  back: () => void
  /** The current hash without reaching for a `location` — tests inject
   *  their own entry stack, and the browser adapter reads the one true
   *  hash source here so no other module has to. */
  readHash: () => string
}

/** The adapter App binds: window.history is the seam, plus the one read
 *  (the hash) the History object cannot make itself. Lives here so every
 *  history call in the app has this file as its single owner. */
export function browserHistory(
  loc: { hash: string } = window.location,
  h: History = window.history,
): HistorySeam {
  return {
    get state() {
      return h.state
    },
    pushState: (data, unused, url) => h.pushState(data, unused, url),
    replaceState: (data, unused, url) => h.replaceState(data, unused, url),
    back: () => h.back(),
    readHash: () => loc.hash,
  }
}

export type PopTarget = {
  addEventListener: (type: 'popstate', listener: () => void) => void
  removeEventListener: (type: 'popstate', listener: () => void) => void
}

const SENTINEL = { gadakBack: true as const }

function isSentinel(state: unknown): boolean {
  return (
    typeof state === 'object' &&
    state !== null &&
    (state as { gadakBack?: unknown }).gadakBack === true
  )
}

function asDetailFrame(state: unknown): DetailRef | null {
  if (typeof state !== 'object' || state === null) return null
  const d = (state as { gadakDetail?: unknown }).gadakDetail
  if (typeof d !== 'object' || d === null) return null
  const { kind, key } = d as { kind?: unknown; key?: unknown }
  if (kind !== 'issue' && kind !== 'page') return null
  if (typeof key !== 'string' || key === '') return null
  return { kind, key }
}

/** The history state for a detail. Built from the two primitives, never
 *  the store's object itself: History structured-clones its state, and a
 *  Svelte $state proxy does not survive the clone (measured 2026-09-17 —
 *  DataCloneError, state-only fallback, no hash). */
function detailState(d: DetailRef): { gadakDetail: DetailRef } {
  return { gadakDetail: { kind: d.kind, key: d.key } }
}

/** The hash a detail frame carries. Hash only — the pathname stays /m/ or
 *  /, and a pushState with a URL can throw in custom-scheme webviews. */
export function detailHash(d: DetailRef): string {
  return d.kind === 'issue' ? `#/${d.key}` : `#/page/${d.key}`
}

/** The detail a cold URL names, or null. Project keys are [A-Z]+-\d+;
 *  page keys are whatever the wiki gave, non-empty. */
export function parseDetailHash(hash: string): DetailRef | null {
  const issue = /^#\/([A-Z][A-Z0-9_]*-\d+)$/.exec(hash)
  if (issue) return { kind: 'issue', key: issue[1] }
  const page = /^#\/page\/(.+)$/.exec(hash)
  if (page && page[1] !== '') return { kind: 'page', key: page[1] }
  return null
}

export type BackStack = {
  registerSheet: (close: () => void) => () => void
  dismissSheets: () => void
  peek: (hasDetail: boolean) => BackKind
  perform: (hasDetail: boolean, closeDetail: () => void) => BackKind
  bind: (
    history: HistorySeam,
    target: PopTarget,
    hasDetail: () => boolean,
    closeDetail: () => void,
    detail?: () => DetailRef | null,
    openDetail?: (kind: 'issue' | 'page', key: string) => void,
  ) => () => void
  /** The effect App feeds app.detail into: keeps the history frame and
   *  the URL in step with the store (GDK-1970). No-op before bind. */
  syncDetail: (detail: DetailRef | null) => void
  sheetCount: () => number
}

export function createBackStack(): BackStack {
  const sheets: Array<() => void> = []
  // The seam bind installed — syncDetail is a no-op until it exists, and
  // unbind retires it again.
  let seam: HistorySeam | null = null
  // The detail the last syncDetail saw, so "changed key" (replace) is
  // distinguishable from "became null" (the ← control's back()).
  let lastDetail: DetailRef | null = null
  // One-shot: the next popstate is ours (the ← control's back()), not the
  // user's — swallow it so it cannot close a layer under the user.
  let ignoreNextPop = false

  const registerSheet = (close: () => void): (() => void) => {
    sheets.push(close)
    return () => {
      const i = sheets.lastIndexOf(close)
      if (i >= 0) sheets.splice(i, 1)
    }
  }

  const dismissSheets = (): void => {
    for (const close of sheets.slice().reverse()) close()
  }

  const peek = (hasDetail: boolean): BackKind => peekBack({ sheetCount: sheets.length, hasDetail })

  const perform = (hasDetail: boolean, closeDetail: () => void): BackKind => {
    const kind = peek(hasDetail)
    if (kind === 'sheet') {
      const close = sheets[sheets.length - 1]
      close?.()
    } else if (kind === 'detail') {
      closeDetail()
    }
    return kind
  }

  const bind = (
    history: HistorySeam,
    target: PopTarget,
    hasDetail: () => boolean,
    closeDetail: () => void,
    detail: () => DetailRef | null = () => null,
    openDetail: (kind: 'issue' | 'page', key: string) => void = () => {},
  ): (() => void) => {
    const arm = () => {
      try {
        if (!isSentinel(history.state)) history.pushState(SENTINEL, '')
      } catch {
        // Custom-scheme webviews may reject pushState. The listener still
        // runs if the shell delivers popstate some other way.
      }
    }
    const pushFrame = (d: DetailRef): void => {
      try {
        history.pushState(detailState(d), '', detailHash(d))
      } catch {
        // Same webview caveat as the sentinel: fall back to state-only.
      }
    }
    seam = history
    // Read the cold link before anything below rewrites the URL.
    const cold = parseDetailHash(history.readHash())
    // A reload lands on the entry the page left: if that entry is a detail
    // frame (the hash reopened below), arming on top of it would stack the
    // sentinel OVER the frame — the first swipe closes the detail, the
    // second pops onto the old frame and reopens it. Clear the frame first
    // so the stack rebuilds as root → sentinel → frame.
    if (asDetailFrame(history.state) !== null) {
      try {
        // '#' clears the hash without naming a path this module does not
        // own (location.hash reads a lone '#' as '').
        history.replaceState(null, '', '#')
      } catch {
        // Same webview caveat; the cold link below still opens the detail.
      }
    }
    arm()
    // Cold link (GDK-1970): the URL this page was opened on can name a
    // detail. Push the frame before opening so the store's own effect
    // finds the state already matching and does not stack a second entry.
    if (cold) {
      pushFrame(cold)
      openDetail(cold.kind, cold.key)
    }
    const onPop = () => {
      if (ignoreNextPop) {
        ignoreNextPop = false
        arm()
        return
      }
      const frame = asDetailFrame(history.state)
      if (frame !== null) {
        // Forward (or a reload pop) onto a detail frame: the screen
        // follows the URL. No arm here — the sentinel this frame was
        // pushed over is the entry below it, and a sentinel pushed on
        // top would strand the frame behind it.
        if (detail() === null) openDetail(frame.kind, frame.key)
        return
      }
      try {
        perform(hasDetail(), closeDetail)
      } finally {
        arm()
      }
    }
    target.addEventListener('popstate', onPop)
    return () => {
      target.removeEventListener('popstate', onPop)
      seam = null
    }
  }

  const syncDetail = (detail: DetailRef | null): void => {
    const prev = lastDetail
    lastDetail = detail
    const history = seam
    if (!history) return
    if (detail === null) {
      // Closed by the visible control, not by the gesture, while the URL
      // still names the detail: make the address follow the screen. The
      // pop this causes must be swallowed (onPop's one-shot).
      if (prev !== null && asDetailFrame(history.state) !== null) {
        ignoreNextPop = true
        try {
          history.back()
        } catch {
          ignoreNextPop = false
        }
      }
      return
    }
    const frame = asDetailFrame(history.state)
    if (frame === null) {
      // Opened over the sentinel (or the root): one frame, hash named.
      try {
        history.pushState(detailState(detail), '', detailHash(detail))
      } catch {
        // State-only fallback, same webview caveat.
      }
    } else if (frame.kind !== detail.kind || frame.key !== detail.key) {
      // Linked-issue navigation: replace, so one back still reaches the
      // list (the header above already promised this).
      try {
        history.replaceState(detailState(detail), '', detailHash(detail))
      } catch {
        // State-only fallback, same webview caveat.
      }
    }
    // Else the state already names this detail — the browser forward or
    // the cold link landed here; writing again would stack entries.
  }

  return {
    registerSheet,
    dismissSheets,
    peek,
    perform,
    bind,
    syncDetail,
    sheetCount: () => sheets.length,
  }
}

/** The process-wide owner App and Sheet share. Tests use createBackStack. */
export const systemBack = createBackStack()
