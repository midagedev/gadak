import { describe, expect, it } from 'vitest'
import {
  scrollGesture,
  type ScrollGestureContext,
  type MouseTrackingMode,
  type BufferType,
} from './scroll-gesture'

const dec = new TextDecoder()

function ctx(over: Partial<ScrollGestureContext> = {}): ScrollGestureContext {
  return {
    buffer: 'normal',
    mouse: 'none',
    cell: { col: 40, row: 12 },
    ...over,
  }
}

function bytes(g: ReturnType<typeof scrollGesture>): string {
  if (g.kind !== 'inject') throw new Error(`expected inject, got ${g.kind}`)
  return dec.decode(g.bytes)
}

describe('scrollGesture routing', () => {
  it('is a no-op for zero lines', () => {
    expect(scrollGesture(0, ctx())).toEqual({ kind: 'none' })
  })

  it('normal buffer, no mouse tracking → local scrollback', () => {
    expect(scrollGesture(-3, ctx())).toEqual({ kind: 'scrollback', lines: -3 })
    expect(scrollGesture(5, ctx())).toEqual({ kind: 'scrollback', lines: 5 })
  })

  it('alternate screen, no mouse tracking → hint (use the arrow keys)', () => {
    expect(scrollGesture(-1, ctx({ buffer: 'alternate' }))).toEqual({ kind: 'hint' })
    expect(scrollGesture(1, ctx({ buffer: 'alternate' }))).toEqual({ kind: 'hint' })
  })

  it('never injects arrows — arrow navigation is the dedicated key bar', () => {
    // The one alternate/no-mouse case is a hint, not an ESC[A. No context
    // produces a cursor-key sequence.
    const g = scrollGesture(-1, ctx({ buffer: 'alternate', mouse: 'none' }))
    expect(g.kind).toBe('hint')
  })

  it('mouse tracking (vt200/drag/any) → SGR wheel, whatever the buffer', () => {
    // 64 = wheel up (lines<0), 65 = down; cell 40,12
    expect(bytes(scrollGesture(-1, ctx({ mouse: 'vt200' })))).toBe('\x1b[<64;40;12M')
    expect(bytes(scrollGesture(1, ctx({ mouse: 'drag' })))).toBe('\x1b[<65;40;12M')
    expect(bytes(scrollGesture(-1, ctx({ mouse: 'any', buffer: 'alternate' })))).toBe(
      '\x1b[<64;40;12M',
    )
  })

  it('a wheel in the alternate screen also hints the arrow keys (crush ignores the wheel)', () => {
    const alt = scrollGesture(-1, ctx({ mouse: 'vt200', buffer: 'alternate' }))
    expect(alt).toMatchObject({ kind: 'inject', hint: true })
    // A normal-buffer wheel has scrollback to fall back on, so no hint.
    const normal = scrollGesture(-1, ctx({ mouse: 'vt200', buffer: 'normal' }))
    expect(normal).toMatchObject({ kind: 'inject', hint: false })
  })

  it('x10 is press-only: not wheel — alternate is a hint, normal is scrollback', () => {
    expect(scrollGesture(-1, ctx({ mouse: 'x10', buffer: 'alternate' }))).toEqual({ kind: 'hint' })
    expect(scrollGesture(-1, ctx({ mouse: 'x10', buffer: 'normal' }))).toEqual({
      kind: 'scrollback',
      lines: -1,
    })
  })

  it('unencodable wide-terminal cell never sends a corrupt report — scrollback in normal, hint in alt', () => {
    const wideNormal = ctx({ mouse: 'vt200', cell: { col: 100000, row: 12 } })
    expect(scrollGesture(-1, wideNormal)).toEqual({ kind: 'scrollback', lines: -1 })
    const wideAlt = ctx({ mouse: 'vt200', buffer: 'alternate', cell: { col: 100000, row: 12 } })
    expect(scrollGesture(-1, wideAlt)).toEqual({ kind: 'hint' })
  })

  it('repeats one wheel report per line and clamps a fling to 32 notches', () => {
    const g = scrollGesture(-100, ctx({ mouse: 'any' }))
    expect(dec.decode((g as { bytes: Uint8Array }).bytes)).toBe('\x1b[<64;40;12M'.repeat(32))
  })
})

// Exhaustiveness note: the mode/buffer matrix above is the contract. A new
// MouseTrackingMode value that changes routing must add a row here — the encoder
// is where GDK-899's invisible defects live.
const _modes: MouseTrackingMode[] = ['none', 'x10', 'vt200', 'drag', 'any']
const _buffers: BufferType[] = ['normal', 'alternate']
void _modes
void _buffers
