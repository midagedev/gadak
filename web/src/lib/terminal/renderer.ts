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
import {
  createUtf8StreamDecoder,
  TERMINAL_ANSI_VARS,
  TERMINAL_CHROME_VARS,
  watchChromeVars,
  type TerminalAnsiSlot,
} from './protocol'
import type { TerminalRenderer } from './protocol'
import { findIssueKeyMatches } from './issue-links'

export { createUtf8StreamDecoder }
export type { TerminalRenderer }

/** A token as it computes on `scope` — the pane's host once there is one.
 *  The dock re-declares the palette under itself (GDK-1357), so a read off
 *  <html> would hand xterm the page's ground, not the dock's. */
function cssVar(name: string, fallback: string, scope?: Element | null): string {
  if (typeof document === 'undefined') return fallback
  const v = getComputedStyle(scope ?? document.documentElement)
    .getPropertyValue(name)
    .trim()
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

/** xterm's own sixteen — the fallback when a token is missing (jsdom, an
 *  older stylesheet), and what the dark palettes declare (GDK-1358). */
const XTERM_DEFAULT_ANSI: Record<TerminalAnsiSlot, string> = {
  black: '#2e3436',
  red: '#cc0000',
  green: '#4e9a06',
  yellow: '#c4a000',
  blue: '#3465a4',
  magenta: '#75507b',
  cyan: '#06989a',
  white: '#d3d7cf',
  brightBlack: '#555753',
  brightRed: '#ef2929',
  brightGreen: '#8ae234',
  brightYellow: '#fce94f',
  brightBlue: '#729fcf',
  brightMagenta: '#ad7fa8',
  brightCyan: '#34e2e2',
  brightWhite: '#eeeeec',
}

type ChromeTheme = {
  background: string
  foreground: string
  cursor: string
  cursorAccent: string
  selectionBackground: string
} & Record<TerminalAnsiSlot, string>

/** The five chrome slots plus the sixteen ANSI slots, every one read from a
 *  token so the palette that owns the ground also owns the ink on it
 *  (GDK-1358 — xterm's defaults are unreadable on paper). Variable names
 *  come from the shared lists in ./protocol (GDK-1109); the fallbacks are
 *  web-owned hexes. */
function chromeTheme(scope?: Element | null): ChromeTheme {
  const ansi = {} as Record<TerminalAnsiSlot, string>
  for (const slot of Object.keys(TERMINAL_ANSI_VARS) as TerminalAnsiSlot[]) {
    ansi[slot] = cssVar(TERMINAL_ANSI_VARS[slot], XTERM_DEFAULT_ANSI[slot], scope)
  }
  return {
    background: cssVar(TERMINAL_CHROME_VARS.background, '#f4efe4', scope),
    foreground: cssVar(TERMINAL_CHROME_VARS.foreground, '#1c1812', scope),
    cursor: cssVar(TERMINAL_CHROME_VARS.cursor, '#2a4159', scope),
    cursorAccent: cssVar(TERMINAL_CHROME_VARS.cursorAccent, '#f4efe4', scope),
    selectionBackground: cssVar(TERMINAL_CHROME_VARS.selectionBackground, '#cfc0a4', scope),
    ...ansi,
  }
}

/*
 * The keys that must escape the VT and reach the app (GDK-864 Ctrl+`, then
 * GDK-1250's four Ctrl+Shift chords). One function, both halves read it:
 * this is the door, lib/commands.ts's registry is where those chords go.
 *
 * The Shift chords match on ev.code — the physical key — because with Shift
 * held ev.key is whatever the layout prints on it ('[' becomes '{' on a US
 * layout and something else on every other one), and the binding is to the
 * key, not the glyph. Their shiftless forms are not app keys: Ctrl+[ is the
 * PTY's ESC, Ctrl+] its GS, Ctrl+O its ^O, so they stay xterm's to process.
 */
function isAppChord(ev: KeyboardEvent): boolean {
  if (!ev.ctrlKey || ev.metaKey || ev.altKey) return false
  if (ev.shiftKey) {
    return (
      ev.code === 'BracketLeft' ||
      ev.code === 'BracketRight' ||
      ev.code === 'Backquote' ||
      ev.code === 'KeyO'
    )
  }
  return ev.key === '`' || ev.code === 'Backquote'
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
  options: {
    scrollback?: number
    cursorBlink?: boolean
    /** Live chrome, so a test can ask what the pane is actually painted in
     *  rather than screenshot it (GDK-1156). Spelled out slot by slot
     *  rather than as an index signature: xterm's own ITheme has none, and
     *  a Record here would stop the real Terminal from satisfying the hook. */
    theme?: {
      background?: string
      foreground?: string
      cursor?: string
      cursorAccent?: string
      selectionBackground?: string
    } & Partial<Record<TerminalAnsiSlot, string>>
  }
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
  /** Empties the screen and the scrollback. The pane calls this when it
   *  switches sessions (GDK-1153): the next session's ring replay is a
   *  complete scrollback of its own, so leaving the previous one above it
   *  would splice two shells into one history. */
  reset(): void
  /**
   * Underlines the issue keys in this terminal's output and opens them
   * (GDK-1160). `projects` is asked per line rather than captured, because
   * the mirror's project list arrives after the pane does; `open` is the
   * app's existing verb for showing an issue, not a second router.
   * Returns the disposer.
   */
  registerIssueLinks(opts: {
    projects: () => Iterable<string>
    open: (key: string) => void
  }): () => void
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
  // Constructed before a host exists, so the first theme is the document's;
  // open() replaces it with the host's own computed tokens (GDK-1357).
  let scope: HTMLElement | null = null
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
  // false means "do not process" — an app chord must escape the VT and reach
  // the app's own handler, and a composition-stealing key (GDK-1095) must
  // stay with the IME.
  term.attachCustomKeyEventHandler((ev) => !isAppChord(ev) && !stealsFromComposition(ev))
  // GDK-1156: the chrome follows the theme for the life of the pane, not
  // just at construction. xterm takes a whole theme object at runtime, so
  // this is a replace, not a patch — chrome and ANSI slots alike, since
  // GDK-1358 made the sixteen tokens too.
  const chromeWatch = watchChromeVars(
    () => JSON.stringify(chromeTheme(scope)),
    () => {
      term.options.theme = chromeTheme(scope)
    },
  )
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
    reset() {
      term.reset()
      term.clear()
    },
    /*
     * One provider, asked per line. xterm hands the provider a 1-based
     * buffer line number and expects 1-based inclusive x coordinates back
     * — the same convention @xterm/addon-web-links works in, which is why
     * this needs no addon of its own: registerLinkProvider is core API.
     *
     * Single line, no wrap stitching: a key broken across a wrap is not
     * offered rather than guessed at. `activate` runs the app's own open
     * verb; there is no second route to an issue in this app and this does
     * not add one.
     */
    registerIssueLinks({ projects, open }) {
      const disposable = term.registerLinkProvider({
        provideLinks(bufferLineNumber, callback) {
          const line = term.buffer.active.getLine(bufferLineNumber - 1)
          const text = line?.translateToString(true)
          if (!text) {
            callback(undefined)
            return
          }
          const matches = findIssueKeyMatches(text, projects())
          if (matches.length === 0) {
            callback(undefined)
            return
          }
          callback(
            matches.map((m) => ({
              text: m.key,
              range: {
                start: { x: m.start + 1, y: bufferLineNumber },
                end: { x: m.end, y: bufferLineNumber },
              },
              activate: () => open(m.key),
            })),
          )
        },
      })
      return () => disposable.dispose()
    },
    open(host: HTMLElement) {
      host.setAttribute('data-gadak-editable', '')
      host.style.height = '100%'
      host.style.width = '100%'
      term.open(host)
      scope = host
      term.options.theme = chromeTheme(scope)
      chromeWatch.sync()
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
      chromeWatch.stop()
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
