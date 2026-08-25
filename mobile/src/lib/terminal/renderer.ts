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
  buffer: {
    active: {
      length: number
      getLine(y: number): { translateToString(trimRight?: boolean): string } | undefined
    }
  }
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

export interface PhoneTerminalRenderer extends TerminalRenderer {
  /** Clear the local buffer so a ring replay is the scrollback, not a duplicate. */
  reset(): void
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
    dispose() {
      unData.dispose()
      unResize.dispose()
      fitAddon.dispose()
      term.dispose()
      exposeTerm(undefined)
    },
  }
}
