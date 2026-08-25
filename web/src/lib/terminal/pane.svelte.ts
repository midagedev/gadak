/*
 * Terminal chrome state (open, overlay vs split, persisted width).
 * Session bytes live in session.ts; this file is the pane's geometry.
 */

import {
  TERMINAL_DEFAULT_RATIO,
  TERMINAL_MIN_WIDTH_PX,
  TERMINAL_OVERLAY_MAX_PX,
  TERMINAL_SPLIT_WITH_DETAIL_MIN_PX,
  TERMINAL_WIDTH_KEY,
  terminalIsNarrow,
} from './layout'

// Geometry constants live in ./layout (no runes there); re-exported because
// the pane and its tests already look here.
export * from './layout'

function readStoredWidth(): number {
  try {
    const raw = localStorage.getItem(TERMINAL_WIDTH_KEY)
    const n = raw ? Number(raw) : NaN
    if (Number.isFinite(n) && n >= TERMINAL_MIN_WIDTH_PX) return Math.round(n)
  } catch {
    /* private mode */
  }
  return 0
}

function defaultWidth(): number {
  if (typeof window === 'undefined') return TERMINAL_MIN_WIDTH_PX
  return Math.max(
    TERMINAL_MIN_WIDTH_PX,
    Math.round(window.innerWidth * TERMINAL_DEFAULT_RATIO),
  )
}

class TerminalChrome {
  open = $state(false)
  /** Overlay rather than split — see terminalIsNarrow for the two reasons. */
  narrow = $state(false)
  /** 0 means "use 44% of the window on next read". */
  #widthPx = $state(0)
  /** A docked detail panel is the fourth surface competing for the row. */
  #detailDocked = $state(false)

  constructor() {
    if (typeof window === 'undefined') return
    this.narrow = terminalIsNarrow(window.innerWidth, false)
    this.#widthPx = readStoredWidth()
  }

  /**
   * App.svelte tells the pane when the detail panel is docked beside it. The
   * pane cannot read that itself: whether the panel is docked or overlaid is
   * the viewport regime's call, and there is one owner of that question.
   */
  setDetailDocked(docked: boolean): void {
    if (this.#detailDocked === docked) return
    this.#detailDocked = docked
    this.#applyNarrow()
  }

  #applyNarrow(): void {
    if (typeof window === 'undefined') return
    this.narrow = terminalIsNarrow(window.innerWidth, this.#detailDocked)
  }

  get widthPx(): number {
    return this.#widthPx >= TERMINAL_MIN_WIDTH_PX ? this.#widthPx : defaultWidth()
  }

  persistWidth(px: number): void {
    const max =
      typeof window === 'undefined' ? px : Math.max(TERMINAL_MIN_WIDTH_PX, window.innerWidth - 200)
    const clamped = Math.min(max, Math.max(TERMINAL_MIN_WIDTH_PX, Math.round(px)))
    this.#widthPx = clamped
    try {
      localStorage.setItem(TERMINAL_WIDTH_KEY, String(clamped))
    } catch {
      /* private mode */
    }
  }

  toggle(): void {
    this.open = !this.open
  }

  /**
   * Watch both overlay thresholds. Call from App onMount.
   *
   * Two media queries rather than a resize listener, because both edges are
   * width thresholds and the browser already owns them; the upper one only
   * matters while the detail panel is docked, which #applyNarrow reads.
   */
  start(): () => void {
    if (typeof window === 'undefined') return () => {}
    const queries = [
      window.matchMedia(`(max-width: ${TERMINAL_OVERLAY_MAX_PX}px)`),
      window.matchMedia(`(max-width: ${TERMINAL_SPLIT_WITH_DETAIL_MIN_PX - 1}px)`),
    ]
    const apply = () => this.#applyNarrow()
    for (const mq of queries) mq.addEventListener('change', apply)
    apply()
    return () => {
      for (const mq of queries) mq.removeEventListener('change', apply)
    }
  }
}

export const terminalChrome = new TerminalChrome()
