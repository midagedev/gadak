/*
 * The terminal pane's renderer (GDK-864).
 *
 * xterm.js + fit addon, entered through dynamic import() so the library
 * stays out of the main bundle until a terminal is actually opened. It is
 * the only renderer: the WASM default it replaced lost a measured shootout
 * (GDK-1041) — parity on throughput, IME composition and alt-screen,
 * 189 KB gzip heavier.
 */

/*
 * The renderer contract and the stream decoder moved to ./protocol (GDK-865):
 * the phone imports that module directly, and this one is web-only renderer
 * code (DOM hosts, cssVar, the xterm dynamic import). Re-exported here
 * because this is where the web app already looks.
 */
import { createUtf8StreamDecoder } from './protocol'
import type { TerminalRenderer } from './protocol'

export { createUtf8StreamDecoder }
export type { TerminalRenderer }

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
 *  paper theme does not invert black/white. */
function chromeTheme(): {
  background: string
  foreground: string
  cursor: string
  cursorAccent: string
  selectionBackground: string
} {
  return {
    background: cssVar('--color-bg-base', '#f4efe4'),
    foreground: cssVar('--color-text-primary', '#1c1812'),
    cursor: cssVar('--color-accent', '#2e4560'),
    cursorAccent: cssVar('--color-bg-base', '#f4efe4'),
    selectionBackground: cssVar('--color-bg-active', '#cfc0a4'),
  }
}

function isToggleChord(ev: KeyboardEvent): boolean {
  return (
    ev.ctrlKey &&
    !ev.metaKey &&
    !ev.altKey &&
    (ev.key === '`' || ev.code === 'Backquote')
  )
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
    /** Underlying xterm Terminal, tests only (GDK-864). */
    __gadakTerm?: TermHook
  }
}

function exposeTerm(term: TermHook | undefined): void {
  if (typeof window === 'undefined') return
  if (term) window.__gadakTerm = term
  else delete window.__gadakTerm
}

/**
 * The terminal's text size, from the token so a person or an agent can set
 * it the way they set every other dimension in this app
 * (`gadak config set ui.tokens.type.terminal 15px` — GDK-853; it merges that
 * one key, so the rest of the ladder survives). It defaults to the body
 * baseline, so an untouched install has one text size.
 *
 * Read at renderer creation: a token change lands on the next open, not
 * mid-session. Live re-fit is a follow-up, not a promise made here.
 */
export function terminalFontSize(read: (name: string) => string = readCssVar): number {
  const px = Number.parseFloat(read('--text-terminal'))
  return Number.isFinite(px) && px > 0 ? px : TERMINAL_FONT_SIZE_FALLBACK
}

/** The injectable half, so a node test can ask without a document. */
function readCssVar(name: string): string {
  return cssVar(name, '')
}

/** Only reached when the stylesheet has not loaded; the token owns the value. */
const TERMINAL_FONT_SIZE_FALLBACK = 13

function termOptions() {
  return {
    fontSize: terminalFontSize(),
    allowTransparency: false,
    // Client-side history, distinct from the serve's 256 KiB reconnect ring:
    // that ring is what a *reattaching* client replays, this is what the
    // person can scroll back through in one session.
    scrollback: 5000,
    cursorBlink: false,
  }
}

async function createXtermRenderer(): Promise<TerminalRenderer> {
  const [{ Terminal }, { FitAddon }] = await Promise.all([
    import('@xterm/xterm'),
    import('@xterm/addon-fit'),
    import('@xterm/xterm/css/xterm.css'),
  ])
  const theme = chromeTheme()
  const term = new Terminal({
    ...termOptions(),
    fontFamily: fontFamily(),
    theme,
    cols: 80,
    rows: 24,
  })
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  // No WebGL addon. It was tried (2026-08-25) and painted a blank pane in
  // Chromium even with preserveDrawingBuffer:true, so it is not installed
  // either — a dependency nothing imports is dead weight, not a "one-line
  // fallback". To try again: npm i @xterm/addon-webgl, then
  // `term.loadAddon(new WebglAddon(true))`. The DOM renderer paints, and
  // its glyphs match the app's own text at 2×.
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
  // false means "do not process" — the toggle chord must escape the VT and
  // reach the app's own handler.
  term.attachCustomKeyEventHandler((ev) => !isToggleChord(ev))

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
    dispose() {
      unData.dispose()
      unResize.dispose()
      fitAddon.dispose()
      term.dispose()
      exposeTerm(undefined)
    },
  }
}

export async function createRenderer(): Promise<TerminalRenderer> {
  return createXtermRenderer()
}
