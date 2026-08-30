/*
 * Terminal chrome state (open, overlay vs dock, persisted height).
 * Session bytes live in session.ts; this file is the pane's geometry.
 */

import {
  TERMINAL_DEFAULT_HEIGHT_RATIO,
  TERMINAL_HEIGHT_KEY,
  TERMINAL_MAX_HEIGHT_RATIO,
  TERMINAL_MIN_HEIGHT_PX,
  TERMINAL_OVERLAY_MAX_PX,
  TERMINAL_SPLIT_WITH_DETAIL_MIN_PX,
  terminalIsNarrow,
} from './layout'

// Geometry constants live in ./layout (no runes there); re-exported because
// the pane and its tests already look here.
export * from './layout'

function readStoredHeight(): number {
  try {
    const raw = localStorage.getItem(TERMINAL_HEIGHT_KEY)
    const n = raw ? Number(raw) : NaN
    if (Number.isFinite(n) && n >= TERMINAL_MIN_HEIGHT_PX) return Math.round(n)
  } catch {
    /* private mode */
  }
  return 0
}

function defaultHeight(): number {
  if (typeof window === 'undefined') return TERMINAL_MIN_HEIGHT_PX
  return Math.max(
    TERMINAL_MIN_HEIGHT_PX,
    Math.round(window.innerHeight * TERMINAL_DEFAULT_HEIGHT_RATIO),
  )
}

function maxHeight(): number {
  if (typeof window === 'undefined') return Number.MAX_SAFE_INTEGER
  return Math.max(
    TERMINAL_MIN_HEIGHT_PX,
    Math.round(window.innerHeight * TERMINAL_MAX_HEIGHT_RATIO),
  )
}

class TerminalChrome {
  open = $state(false)
  /** Overlay rather than split — see terminalIsNarrow for the two reasons. */
  narrow = $state(false)
  /** 0 means "use 40% of the window on next read". */
  #heightPx = $state(0)
  /** A docked detail panel is the fourth surface competing for the row. */
  #detailDocked = $state(false)

  constructor() {
    if (typeof window === 'undefined') return
    this.narrow = terminalIsNarrow(window.innerWidth, false)
    this.#heightPx = readStoredHeight()
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

  get heightPx(): number {
    const want = this.#heightPx >= TERMINAL_MIN_HEIGHT_PX ? this.#heightPx : defaultHeight()
    return Math.min(want, maxHeight())
  }

  /*
   * The ceiling is enforced here rather than in CSS: the dock is a grid item
   * in an `auto` row, so a percentage max-height has no definite container to
   * resolve against. Clamping on read as well as on write (see heightPx)
   * keeps a height stored on a tall window from opening a dock taller than a
   * short one.
   */
  persistHeight(px: number): void {
    const clamped = Math.min(maxHeight(), Math.max(TERMINAL_MIN_HEIGHT_PX, Math.round(px)))
    this.#heightPx = clamped
    try {
      localStorage.setItem(TERMINAL_HEIGHT_KEY, String(clamped))
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
