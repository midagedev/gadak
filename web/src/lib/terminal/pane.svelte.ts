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
  TERMINAL_MODE_KEY,
  TERMINAL_OVERLAY_MAX_PX,
  type TerminalMode,
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

function readStoredMode(): TerminalMode {
  try {
    const raw = localStorage.getItem(TERMINAL_MODE_KEY)
    if (raw === 'dock' || raw === 'full') return raw
  } catch {
    /* private mode */
  }
  return ''
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
  /** Overlay sheet rather than the dock band. The reader's pinned mode when
   *  there is one, the window's width when there is not (GDK-1835). */
  narrow = $state(false)
  /** 0 means "use 40% of the window on next read". */
  #heightPx = $state(0)
  /** '' = follow the window. Persisted beside the height, same owner. */
  #mode = $state<TerminalMode>('')

  /*
   * The roster can be rendered in two places — the dock's own left column,
   * and the app sidebar while the pane is the full-screen sheet (GDK-1835) —
   * so the two verbs it offers cannot live inside whichever copy happens to
   * be mounted. The pane publishes them here on mount and clears them on
   * destroy; a roster with a null verb renders the row disabled rather than
   * guessing, which is what a pane that has not booted actually means.
   */
  newSession = $state<(() => void) | null>(null)
  /** The shell the pane is on can be restarted from its status (GDK-991). */
  restartable = $state(false)
  restart = $state<(() => void) | null>(null)

  constructor() {
    if (typeof window === 'undefined') return
    this.#mode = readStoredMode()
    this.#applyNarrow()
    this.#heightPx = readStoredHeight()
  }

  /** '' when the reader has not pinned one — the window decides. */
  get mode(): TerminalMode {
    return this.#mode
  }

  /*
   * The control is a toggle, not a cycle: whatever the pane is showing now,
   * pressing it pins the other one. Pinning what the window already chose is
   * still a pin — the point of GDK-1835 is that resizing the window must not
   * undo the reader's answer.
   */
  toggleMode(): void {
    this.#writeMode(this.narrow ? 'dock' : 'full')
  }

  /** Back to the window's verdict — the palette's "follow the window". */
  resetMode(): void {
    this.#writeMode('')
  }

  #writeMode(mode: TerminalMode): void {
    this.#mode = mode
    try {
      if (mode) localStorage.setItem(TERMINAL_MODE_KEY, mode)
      else localStorage.removeItem(TERMINAL_MODE_KEY)
    } catch {
      /* private mode */
    }
    this.#applyNarrow()
  }

  #applyNarrow(): void {
    if (typeof window === 'undefined') return
    this.narrow = this.#mode ? this.#mode === 'full' : terminalIsNarrow(window.innerWidth)
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
    const mq = window.matchMedia(`(max-width: ${TERMINAL_OVERLAY_MAX_PX}px)`)
    const apply = () => this.#applyNarrow()
    mq.addEventListener('change', apply)
    apply()
    return () => {
      mq.removeEventListener('change', apply)
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
