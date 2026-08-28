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

/**
 * The terminal's font stack, from a token of its own (GDK-1043): WebKit
 * resolves ui-monospace to SF Mono, whose box-glyph ink (15.31css at 13px)
 * undershoots the 16css cell xterm derives — a 1px seam at every row
 * boundary — while Menlo joins by overshoot on both engines. --font-mono
 * stays the app-wide face (code chips, tables) where box grids never occur.
 * Falls back to it, then to a literal, so jsdom (no stylesheet) still gets
 * a stack. Injectable reader, same shape as terminalFontSize.
 */
export function fontFamily(read: (name: string) => string = readCssVar): string {
  const terminal = read('--font-mono-terminal')
  if (terminal) return terminal
  const raw = read('--font-mono')
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

/*
 * GDK-1095: a key that arrives with a real keyCode while an IME composition
 * is open — Backspace, a Ctrl+letter chord, an arrow — makes xterm 6.0.0's
 * CompositionHelper send the composed text twice: the keydown path
 * finalizes and sends without recording what it sent, and compositionend
 * then resends the whole buffer (measured: Ctrl+A during 한글 composition
 * writes the syllable to the PTY twice). Enter is the one upstream-guarded
 * key (its CR branch clears the textarea), and ordinary IME keystrokes
 * arrive as keyCode 229, which xterm already ignores. Everything else is
 * blocked before xterm processes it — the custom key handler runs ahead of
 * CompositionHelper.keydown — so the IME keeps owning those keys and the
 * composition is sent exactly once, by compositionend.
 */
export function stealsFromComposition(ev: KeyboardEvent): boolean {
  return ev.type === 'keydown' && ev.isComposing && ev.keyCode !== 229 && ev.keyCode !== 13
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
  /** Live xterm options — the behavior seam's readback (GDK-896 R2).
   *  Optional like xterm's own ITerminalOptions; a set option is always
   *  materialized by the time a test reads it back. */
  options: { scrollback?: number; cursorBlink?: boolean }
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

/**
 * Terminal behavior the create response carries (GDK-896 R2). The values
 * live in the Go config (`terminal.scrollback`, `terminal.cursorBlink`);
 * the pane hands them to the renderer the moment it has them — there is no
 * second opinion on this side (fallbacks sit next to the response, in
 * ./session).
 */
export interface TerminalBehavior {
  scrollback: number
  cursorBlink: boolean
}

/** The renderer the pane drives: the shared contract plus the behavior
 *  seam. Mobile's own renderer does not carry it — behavior follows the
 *  create response, which is the web pane's path. */
export type BehaviorTerminalRenderer = TerminalRenderer & {
  /** Applies create-session behavior to the live terminal. xterm takes
   *  options after construction (scrollback reallocates the visible
   *  buffer), so this lands between the create response and the first
   *  replay byte — nothing is frozen at open. */
  applyBehavior(b: TerminalBehavior): void
}

function termOptions() {
  return {
    fontSize: terminalFontSize(),
    allowTransparency: false,
    // No scrollback/cursorBlink here: behavior comes from the create
    // response (GDK-896 R2), applied via applyBehavior once the pane knows
    // it — the server is the single owner of those values, not this file.
  }
}

async function createXtermRenderer(): Promise<BehaviorTerminalRenderer> {
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
  // reach the app's own handler, and a composition-stealing key (GDK-1095)
  // must stay with the IME.
  term.attachCustomKeyEventHandler((ev) => !isToggleChord(ev) && !stealsFromComposition(ev))
  // Exposed from creation, not open(): the hook mirrors the live terminal,
  // which exists before a host does (unit tests applyBehavior with no DOM).
  exposeTerm(term)

  return {
    name: 'xterm',
    get cols() {
      return term.cols
    },
    get rows() {
      return term.rows
    },
    applyBehavior(b: TerminalBehavior) {
      term.options.scrollback = b.scrollback
      term.options.cursorBlink = b.cursorBlink
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

export async function createRenderer(): Promise<BehaviorTerminalRenderer> {
  return createXtermRenderer()
}
