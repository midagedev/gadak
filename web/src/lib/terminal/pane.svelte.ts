/*
 * Terminal chrome state (open, overlay vs dock, persisted height).
 * Session bytes live in session.ts; this file is the pane's geometry.
 *
 * GDK-1815: the dock's grip is the same component the sidebar and list
 * seams use (shell/LayoutResizeHandle.svelte), which means the same arrow
 * keys, the same Backspace reset, the same slider role and the same
 * save-once-per-gesture cadence. What did NOT move is where the number
 * lives: this file's localStorage, not `ui.tokens.layout` in config.json.
 * That is deliberate and it is a product decision, not an oversight — what
 * belongs in the document the CLI and other tabs share is a question about
 * the document. `terminalHeightGrip()` below is the adapter that lets the
 * behaviour cross that line without the store crossing with it.
 */

import type { ResizeGrip } from '../layout-resize'
import {
  TERMINAL_HEIGHT_KEY,
  TERMINAL_MIN_HEIGHT_PX,
  TERMINAL_OVERLAY_MAX_PX,
  TERMINAL_SPLIT_WITH_DETAIL_MIN_PX,
  terminalIsNarrow,
  dockDefaultHeight,
  dockMaxHeight,
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
  return dockDefaultHeight(window.innerHeight)
}

function maxHeight(): number {
  if (typeof window === 'undefined') return Number.MAX_SAFE_INTEGER
  return dockMaxHeight(window.innerHeight)
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

  /** The dock's floor and ceiling right now, for the grip to announce. */
  get minHeightPx(): number {
    return TERMINAL_MIN_HEIGHT_PX
  }

  get maxHeightPx(): number {
    return maxHeight()
  }

  /*
   * The ceiling is enforced here rather than in CSS: the dock is a grid item
   * in an `auto` row, so a percentage max-height has no definite container to
   * resolve against. Clamping on read as well as on write (see heightPx)
   * keeps a height stored on a tall window from opening a dock taller than a
   * short one.
   */
  #clamp(px: number): number {
    return Math.min(maxHeight(), Math.max(TERMINAL_MIN_HEIGHT_PX, Math.round(px)))
  }

  /**
   * Paint a height without saving it (GDK-1815). The drag driver calls this
   * on every frame and `persistHeight` once, on release — the dock used to
   * write localStorage per pointermove, which is the cadence half of the
   * defect the layout grip's comment named and the dock's did not.
   */
  setHeight(px: number): void {
    this.#heightPx = this.#clamp(px)
  }

  persistHeight(px: number): void {
    const clamped = this.#clamp(px)
    this.#heightPx = clamped
    try {
      localStorage.setItem(TERMINAL_HEIGHT_KEY, String(clamped))
    } catch {
      /* private mode */
    }
  }

  /**
   * Back to the shipped height — the keyboard's Backspace and the grip's
   * double-click. Like the layout grip's reset it DELETES rather than writes
   * a number: `defaultHeight()` is a fraction of the window, so a pinned px
   * would be the right quarter on this display and the wrong one on the next.
   */
  resetHeight(): void {
    this.#heightPx = 0
    try {
      localStorage.removeItem(TERMINAL_HEIGHT_KEY)
    } catch {
      /* private mode */
    }
  }

  /** A cancelled gesture: put the paint back where the store says it is. */
  rollbackHeight(): void {
    this.#heightPx = readStoredHeight()
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

/**
 * The dock's top edge as a `ResizeGrip` (GDK-1815) — what
 * shell/LayoutResizeHandle.svelte needs to drive this seam with the same
 * pointer, arrow keys and reset it gives the sidebar and the list.
 *
 * Up is taller: the grip sits on the dock's TOP edge, so the height grows as
 * the pointer (or ArrowUp) goes up. `fromPointer` is therefore a delta from
 * where the gesture began, not an absolute like the layout grip's — the seam
 * still follows the hand, but the origin it is measured from is the window's
 * bottom rather than a box's left edge.
 */
export function terminalHeightGrip(): ResizeGrip {
  return {
    axis: 'terminal',
    orientation: 'vertical',
    min: () => terminalChrome.minHeightPx,
    max: () => terminalChrome.maxHeightPx,
    current: () => terminalChrome.heightPx,
    fromPointer: (event, start) => start.value + (start.clientY - event.clientY),
    paint: (px) => terminalChrome.setHeight(px),
    commit: (px) => terminalChrome.persistHeight(px),
    reset: () => terminalChrome.resetHeight(),
    rollback: () => terminalChrome.rollbackHeight(),
  }
}

/*
 * Standing debug read-out (GDK-1815, CLAUDE.md §9c), the same shape
 * viewport-regime.ts's `__gadakLayout()` has for the columns:
 * `__gadakTerminal()` answers "why is the dock this tall" in one line — the
 * height in force, the wish in localStorage behind it (0 = none, so the
 * default is in force), the two ends the grip clamps to, and whether the
 * pane is a dock at all right now. The scratch probe for this round was
 * `localStorage.getItem` plus two hand-computed ratios; this is that probe,
 * kept.
 */
export function terminalGeometryDebug(): {
  heightPx: number
  storedPx: number
  defaultPx: number
  minPx: number
  maxPx: number
  open: boolean
  narrow: boolean
} {
  return {
    heightPx: terminalChrome.heightPx,
    storedPx: readStoredHeight(),
    defaultPx: defaultHeight(),
    minPx: terminalChrome.minHeightPx,
    maxPx: terminalChrome.maxHeightPx,
    open: terminalChrome.open,
    narrow: terminalChrome.narrow,
  }
}

if (typeof window !== 'undefined') {
  ;(window as unknown as { __gadakTerminal?: () => unknown }).__gadakTerminal =
    terminalGeometryDebug
}
