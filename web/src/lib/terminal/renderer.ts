/*
 * One renderer seam for the terminal pane (GDK-864).
 *
 * ghostty-web is the default (libghostty-vt WASM). xterm.js + webgl + fit
 * is the fallback, loaded only when asked (`?term=xterm` / localStorage) or
 * when ghostty's import() rejects. Both enter through dynamic import() so
 * the unused renderer — and the WASM — stay out of the main bundle.
 *
 * Kind owner, in this order: URL query `term` > localStorage
 * `gadak.terminal.renderer` > ghostty. Nothing else writes the stored kind;
 * a runtime fallback does not persist.
 */

export type RendererKind = 'ghostty' | 'xterm'

export const RENDERER_STORAGE_KEY = 'gadak.terminal.renderer'

export interface TerminalRenderer {
  open(host: HTMLElement): void
  write(data: Uint8Array | string): void
  onData(cb: (bytes: Uint8Array) => void): void
  onResize(cb: (cols: number, rows: number) => void): void
  fit(): void
  focus(): void
  dispose(): void
  readonly cols: number
  readonly rows: number
  readonly name: RendererKind
}

/** Streaming UTF-8 decoder so a 256 KiB ring replay that splits a character
 *  across two write() calls still renders one glyph. fatal:false matches
 *  "bytes from a PTY, never a contract that they are well-formed". */
export function createUtf8StreamDecoder(): {
  push(data: Uint8Array | string): string
} {
  const dec = new TextDecoder('utf-8', { fatal: false })
  return {
    push(data: Uint8Array | string): string {
      if (typeof data === 'string') return data
      return dec.decode(data, { stream: true })
    },
  }
}

export function resolveRendererKind(opts?: {
  search?: string
  storage?: { getItem(key: string): string | null } | null
}): RendererKind {
  const search =
    opts?.search ?? (typeof location !== 'undefined' ? location.search : '')
  const q = new URLSearchParams(search).get('term')
  if (q === 'xterm' || q === 'ghostty') return q
  const storage =
    opts && 'storage' in opts
      ? opts.storage
      : typeof localStorage !== 'undefined'
        ? localStorage
        : null
  const stored = storage?.getItem(RENDERER_STORAGE_KEY)
  if (stored === 'xterm' || stored === 'ghostty') return stored
  return 'ghostty'
}

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
    /** Underlying ghostty-web / xterm Terminal, tests only (GDK-864). */
    __gadakTerm?: TermHook
  }
}

function exposeTerm(term: TermHook | undefined): void {
  if (typeof window === 'undefined') return
  if (term) window.__gadakTerm = term
  else delete window.__gadakTerm
}

let ghosttyFallbackLogged = false

function logGhosttyFallback(err: unknown): void {
  if (ghosttyFallbackLogged) return
  ghosttyFallbackLogged = true
  console.warn('gadak: ghostty-web failed to load, using xterm', err)
}

/**
 * The terminal's text size, from the token so a person or an agent can set
 * it the way they set every other dimension in this app
 * (`gadak config set ui.tokens.type.terminal 15px`). It defaults to the body
 * baseline, so an untouched install has one text size.
 *
 * Read at renderer creation: a token change lands on the next open, not
 * mid-session. Live re-fit is a follow-up, not a promise made here.
 */
export function terminalFontSize(read: (name: string) => string = readCssVar): number {
  const px = Number.parseFloat(read('--text-terminal'))
  return Number.isFinite(px) && px > 0 ? px : TERMINAL_FONT_SIZE_FALLBACK
}

/** The injectable half, so a node test can ask without a document — the same
 *  seam shape resolveRendererKind uses for its storage. */
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

async function createGhosttyRenderer(): Promise<TerminalRenderer> {
  const { init, Terminal, FitAddon } = await import('ghostty-web')
  await init()
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
  // ghostty: true consumes the event (does not send it to the PTY).
  term.attachCustomKeyEventHandler((ev) => isToggleChord(ev))

  return {
    name: 'ghostty',
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
  // xterm: false means "do not process" (the opposite of ghostty's true).
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

export async function createRenderer(
  kind?: RendererKind,
): Promise<TerminalRenderer> {
  const requested = kind ?? resolveRendererKind()
  if (requested === 'xterm') return createXtermRenderer()
  try {
    return await createGhosttyRenderer()
  } catch (err) {
    logGhosttyFallback(err)
    return createXtermRenderer()
  }
}
