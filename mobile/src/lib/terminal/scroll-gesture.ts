// What a vertical touch drag on the phone terminal means — a local scrollback
// move, a wheel report to a mouse-aware TUI, or nothing (GDK-899).
//
// The phone deliberately separates *input* from *display*: the terminal host is
// display-only (`disableStdin`), and every key — Esc, Ctrl, and the arrows —
// is a dedicated key-bar button (`keys.ts`). A touch on the display is a
// *scroll gesture*, so it only ever does what those keys cannot: scroll the
// local scrollback, or send a wheel report. It never injects arrows — arrow
// navigation is the key bar's job, and a swipe that walked the cursor would
// duplicate a dedicated key with a worse affordance.
//
// That leaves one case with nothing to do: an alternate-screen TUI that does
// not track the mouse (a pager with mouse off, an agent CLI that ignores the
// wheel). There is no scrollback there and no wheel target, so the gesture is a
// `hint` — the UI shows "use the arrow keys", which is exactly what scrolls it.
//
// Pure over plain data — the renderer holds the live modes and passes them in.
// Direction convention (matches xterm's `scrollLines`): `lines < 0` scrolls UP
// toward history, `lines > 0` scrolls DOWN. The pixels → lines conversion and
// its sign live at the call site; this module only encodes the decision.

/** xterm's public mouse-tracking mode (`term.modes.mouseTrackingMode`). */
export type MouseTrackingMode = 'none' | 'x10' | 'vt200' | 'drag' | 'any'
export type BufferType = 'normal' | 'alternate'

export type ScrollGestureContext = {
  buffer: BufferType
  mouse: MouseTrackingMode
  /** 1-based cell under the gesture, for SGR wheel reports. The centre is fine. */
  cell: { col: number; row: number }
}

export type ScrollGesture =
  | { kind: 'none' }
  | { kind: 'scrollback'; lines: number }
  // A wheel report to a mouse-aware TUI. `hint` is set in the alternate screen:
  // there is no scrollback to fall back on, and some agent CLIs (crush,
  // measured) enable mouse tracking yet ignore the wheel — so if the swipe did
  // nothing, the arrow keys are the recourse and the UI says so. In the normal
  // buffer a wheel is a bonus over the scrollback that still exists, so no hint.
  | { kind: 'inject'; bytes: Uint8Array; hint: boolean }
  // Alternate screen with no scrollback and no wheel target: touch cannot
  // scroll it at all. The UI hints the arrow keys, which can.
  | { kind: 'hint' }

const utf8 = new TextEncoder()

// A fling can produce hundreds of rows; bound one gesture's notches so a flick
// never floods the PTY with thousands of wheel reports (orca uses the same cap).
const MAX_NOTCHES = 32

/**
 * Whether the app wants wheel reports. Any tracking mode except `x10`, which is
 * press-only — it reports neither motion nor wheel. `none` is a plain TUI with
 * no mouse interest.
 */
function wantsWheel(mode: MouseTrackingMode): boolean {
  return mode !== 'none' && mode !== 'x10'
}

function safeCoord(v: number): boolean {
  return Number.isInteger(v) && v >= 1 && v <= 9999
}

/**
 * SGR wheel report: `ESC [ < 64|65 ; col ; row M`. 64 is wheel-up, 65 down.
 * Null when the cell cannot be encoded — a very wide terminal — so the caller
 * sends nothing rather than a corrupt report.
 */
function wheelSeq(lines: number, cell: { col: number; row: number }): Uint8Array | null {
  if (!safeCoord(cell.col) || !safeCoord(cell.row)) return null
  const code = lines < 0 ? 64 : 65
  return utf8.encode(`\x1b[<${code};${cell.col};${cell.row}M`)
}

function repeat(seq: Uint8Array, count: number): Uint8Array {
  const out = new Uint8Array(seq.length * count)
  for (let i = 0; i < count; i++) out.set(seq, i * seq.length)
  return out
}

/**
 * Route a scroll of `lines` rows given the live modes.
 *
 * - A mouse-aware app (any tracking but `x10`) → a wheel report, whatever the
 *   buffer. Unencodable on a wide terminal → `none` (never a corrupt report).
 * - A plain alternate-screen TUI → `hint`: touch cannot scroll it; the arrow
 *   keys can.
 * - Otherwise (normal buffer) → the terminal's own scrollback.
 */
export function scrollGesture(lines: number, ctx: ScrollGestureContext): ScrollGesture {
  if (lines === 0) return { kind: 'none' }

  const alternate = ctx.buffer === 'alternate'

  if (wantsWheel(ctx.mouse)) {
    const seq = wheelSeq(lines, ctx.cell)
    // Unencodable on a wide terminal: never a corrupt report. In the alternate
    // screen the arrow keys are still the recourse, so hint; in the normal
    // buffer, fall back to the scrollback that exists.
    if (!seq) return alternate ? { kind: 'hint' } : { kind: 'scrollback', lines }
    const count = Math.min(Math.abs(lines), MAX_NOTCHES)
    return { kind: 'inject', bytes: repeat(seq, count), hint: alternate }
  }

  if (alternate) return { kind: 'hint' }

  return { kind: 'scrollback', lines }
}
