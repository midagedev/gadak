// Phone terminal renderer (GDK-865). xterm.js + fit addon only: the phone
// must not resolve ghostty-web (WASM, megabytes) or the webgl addon (paints
// a blank canvas headless — measured on the web side).
//
// The contract (TerminalRenderer, createUtf8StreamDecoder) lives in
// web/src/lib/terminal/protocol.ts so this file never imports
// web/src/lib/terminal/renderer.ts, which holds `import('ghostty-web')`.
// Chrome colours copy the web xterm branch's CSS variable *list*, not hex.

import {
  createUtf8StreamDecoder,
  type TerminalRenderer,
} from '../../../../web/src/lib/terminal/protocol'
import type { CursorKeyMode } from './keys'
import type { BufferType, MouseTrackingMode } from './scroll-gesture'

function cssVar(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

function fontFamily(): string {
  const raw = cssVar('--font-mono', '')
  return raw || 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'
}

/** Chrome colours only — ANSI palette stays the library default so a light
 *  paper theme does not invert black/white. Variable list copied from
 *  web/src/lib/terminal/renderer.ts chromeTheme (xterm branch). */
function chromeTheme(): {
  background: string
  foreground: string
  cursor: string
  cursorAccent: string
  selectionBackground: string
} {
  return {
    background: cssVar('--color-bg-base', ''),
    foreground: cssVar('--color-text-primary', ''),
    cursor: cssVar('--color-accent', ''),
    cursorAccent: cssVar('--color-bg-base', ''),
    selectionBackground: cssVar('--color-bg-active', ''),
  }
}

type TermHook = {
  /** Live DEC private modes; `modes` is xterm's public getter. */
  modes?: {
    applicationCursorKeysMode?: boolean
    /** xterm's union, kept as a plain string so a partial double still fits. */
    mouseTrackingMode?: string
  }
  buffer?: {
    active?: {
      type?: string
      length: number
      viewportY?: number
      baseY?: number
      getLine(y: number): { translateToString(trimRight?: boolean): string } | undefined
    }
  }
  scrollLines?(n: number): void
  cols: number
  rows: number
}

declare global {
  interface Window {
    /** Underlying xterm Terminal, tests only (GDK-865). */
    __gadakTerm?: TermHook
    /** Closes the live socket so e2e can watch reattach (GDK-865). */
    __gadakShellDrop?: () => void
  }
}

function exposeTerm(term: TermHook | undefined): void {
  if (typeof window === 'undefined') return
  if (term) window.__gadakTerm = term
  else delete window.__gadakTerm
}

/** Only reached when the stylesheet has not loaded; the token owns the value. */
const TERMINAL_FONT_SIZE_FALLBACK = 16

export function terminalFontSize(read: (name: string) => string = readCssVar): number {
  const px = Number.parseFloat(read('--text-terminal'))
  return Number.isFinite(px) && px > 0 ? px : TERMINAL_FONT_SIZE_FALLBACK
}

function readCssVar(name: string): string {
  return cssVar(name, '')
}

/*
 * GDK-899 — the scroll router's context reads, as pure filters. They exist
 * separately from the renderer object because the contract is "a term that
 * lacks the field reads the safe default, never throws": `window.__gadakTerm`
 * is typed TermHook precisely so a partial double can sit there, and the
 * wiring below optional-chains every read through these.
 */

/** Only xterm's four tracking modes count; anything else is no mouse interest. */
export function readMouseTrackingMode(raw: unknown): MouseTrackingMode {
  return raw === 'x10' || raw === 'vt200' || raw === 'drag' || raw === 'any' ? raw : 'none'
}

/** Alternate only when the active buffer says so — the default is the normal one. */
export function readBufferType(raw: unknown): BufferType {
  return raw === 'alternate' ? 'alternate' : 'normal'
}

/** A missing or non-finite offset is 0 — the indicator must not render NaN. */
export function readBufferOffset(raw: unknown): number {
  return typeof raw === 'number' && Number.isFinite(raw) ? raw : 0
}

export interface PhoneTerminalRenderer extends TerminalRenderer {
  /** Clear the local buffer so a ring replay is the scrollback, not a duplicate. */
  reset(): void
  /**
   * DECCKM as the application currently has it (GDK-899). The key bar
   * writes to the socket directly instead of going through xterm's
   * keyboard path, so it has to ask for this — xterm cannot tell it.
   */
  cursorKeyMode(): CursorKeyMode
  /** Live mouse-tracking mode for the scroll router (GDK-899). */
  mouseTrackingMode(): MouseTrackingMode
  /** Which buffer owns the screen — alternate-screen TUIs own their scroll. */
  bufferType(): BufferType
  /** Scroll the local viewport by n rows (xterm sign: negative = toward history). */
  scrollLines(n: number): void
  /** Viewport position for the scroll indicator thumb. */
  viewport(): { viewportY: number; baseY: number }
}

export async function createRenderer(): Promise<PhoneTerminalRenderer> {
  const [{ Terminal }, { FitAddon }] = await Promise.all([
    import('@xterm/xterm'),
    import('@xterm/addon-fit'),
    import('@xterm/xterm/css/xterm.css'),
  ])
  const theme = chromeTheme()
  const term = new Terminal({
    fontSize: terminalFontSize(),
    allowTransparency: false,
    // Client-side history, distinct from the serve's 256 KiB reconnect ring:
    // that ring is what a *reattaching* client replays, this is what the
    // person can scroll back through in one session.
    scrollback: 5000,
    cursorBlink: false,
    fontFamily: fontFamily(),
    theme,
    cols: 80,
    rows: 24,
    // Display only. Phone keystrokes go through the IME field and the key
    // bar (DESIGN.md §10.3); PTY echo is what paints. Local echo would
    // double every character.
    disableStdin: true,
  })
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  const decoder = createUtf8StreamDecoder()
  const encoder = new TextEncoder()
  let dataCb: ((bytes: Uint8Array) => void) | null = null
  let resizeCb: ((cols: number, rows: number) => void) | null = null
  const unData = term.onData((s) => {
    dataCb?.(encoder.encode(s))
  })
  const unResize = term.onResize(({ cols, rows }) => {
    resizeCb?.(cols, rows)
  })

  return {
    name: 'xterm',
    get cols() {
      return term.cols
    },
    get rows() {
      return term.rows
    },
    open(host: HTMLElement) {
      host.setAttribute('data-gadak-editable', '')
      host.style.height = '100%'
      host.style.width = '100%'
      term.open(host)
      if (term.element) {
        term.element.style.height = '100%'
        term.element.style.width = '100%'
      }
      exposeTerm(term)
    },
    write(data: Uint8Array | string) {
      term.write(decoder.push(data))
    },
    onData(cb) {
      dataCb = cb
    },
    onResize(cb) {
      resizeCb = cb
    },
    fit() {
      fitAddon.fit()
    },
    focus() {
      term.focus()
    },
    reset() {
      term.reset()
    },
    cursorKeyMode() {
      // Optional-chained on purpose: a test double that only implements
      // what it renders must read as 'normal', not throw at a keypress.
      return term.modes?.applicationCursorKeysMode ? 'application' : 'normal'
    },
    mouseTrackingMode() {
      return readMouseTrackingMode(term.modes?.mouseTrackingMode)
    },
    bufferType() {
      return readBufferType(term.buffer?.active?.type)
    },
    scrollLines(n: number) {
      term.scrollLines?.(n)
    },
    viewport() {
      return {
        viewportY: readBufferOffset(term.buffer?.active?.viewportY),
        baseY: readBufferOffset(term.buffer?.active?.baseY),
      }
    },
    dispose() {
      unData.dispose()
      unResize.dispose()
      fitAddon.dispose()
      term.dispose()
      exposeTerm(undefined)
    },
  }
}
