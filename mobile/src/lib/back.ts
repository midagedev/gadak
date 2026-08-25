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
// replaces the detail key in the store (DESIGN.md §2) — this module does
// not push a frame for it, so one back still returns to the list.
//
// At the root (no sheet, no detail) back is a no-op. The visible edge
// on a tab is the tab bar, which does not leave the app; Unpair is the
// explicit way out of a pairing. Consuming the gesture is what stops
// the activity from finishing.
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

export type HistorySeam = {
  readonly state: unknown
  pushState: (data: unknown, unused: string) => void
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
  ) => () => void
  sheetCount: () => number
}

export function createBackStack(): BackStack {
  const sheets: Array<() => void> = []

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
  ): (() => void) => {
    const arm = () => {
      try {
        if (!isSentinel(history.state)) history.pushState(SENTINEL, '')
      } catch {
        // Custom-scheme webviews may reject pushState. The listener still
        // runs if the shell delivers popstate some other way.
      }
    }
    arm()
    const onPop = () => {
      try {
        perform(hasDetail(), closeDetail)
      } finally {
        arm()
      }
    }
    target.addEventListener('popstate', onPop)
    return () => target.removeEventListener('popstate', onPop)
  }

  return {
    registerSheet,
    dismissSheets,
    peek,
    perform,
    bind,
    sheetCount: () => sheets.length,
  }
}

/** The process-wide owner App / Sheet / TabBar share. Tests use createBackStack. */
export const systemBack = createBackStack()
