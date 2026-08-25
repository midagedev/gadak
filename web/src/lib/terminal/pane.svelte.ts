/*
 * Terminal chrome state (open, overlay vs split, persisted width).
 * Session bytes live in session.ts; this file is the pane's geometry.
 */

export const TERMINAL_WIDTH_KEY = 'gadak.terminal.width'
export const TERMINAL_MIN_WIDTH_PX = 320
export const TERMINAL_DEFAULT_RATIO = 0.44
/*
 * The split's ceiling, as a percentage of the *track it lives in* — not of
 * the window (2026-08-25, GDK-864 lead review). The pane is a flex child of
 * the layout's main track, which is capped at 1360px, while the default
 * width is 44% of the window. On a wide display those two disagree: 44% of
 * 2560px is 1126px inside a 1360px track, leaving the issue list ~230px.
 * A percentage max-width resolves against the parent, so the list keeps its
 * share no matter how wide the display is, and dragging past the cap simply
 * saturates.
 */
export const TERMINAL_SPLIT_MAX_PCT = 60
/** Below 900px the pane is a full-width overlay instead of a split. */
export const TERMINAL_OVERLAY_MAX_PX = 899

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
  /** Viewport ≤ 899px: overlay, not a split. Independent of the detail-panel
   *  docked floor (1100). */
  narrow = $state(false)
  /** 0 means "use 44% of the window on next read". */
  #widthPx = $state(0)

  constructor() {
    if (typeof window === 'undefined') return
    this.narrow = window.innerWidth <= TERMINAL_OVERLAY_MAX_PX
    this.#widthPx = readStoredWidth()
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

  /** Subscribe to the 900px overlay breakpoint. Call from App onMount. */
  start(): () => void {
    if (typeof window === 'undefined') return () => {}
    const mq = window.matchMedia(`(max-width: ${TERMINAL_OVERLAY_MAX_PX}px)`)
    const apply = () => {
      this.narrow = mq.matches
    }
    mq.addEventListener('change', apply)
    apply()
    return () => mq.removeEventListener('change', apply)
  }
}

export const terminalChrome = new TerminalChrome()
