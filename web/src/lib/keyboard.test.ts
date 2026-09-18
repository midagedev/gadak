import { afterEach, describe, expect, it } from 'vitest'
import {
  bindKeyboardBand,
  keyboardInset,
  measure,
  probeKbInset,
  type VvLike,
} from './keyboard'

/*
 * GDK-1971 — the covered band has one owner, and these tests pin the three
 * halves of that claim:
 *
 *   measure            the formula itself (clamp, offsetTop)
 *   bindKeyboardBand   the root binding: variable in px, flag set/cleared,
 *                      listeners removed on unbind, probe path
 *   keyboardInset      the action: same formula, same input, same number
 *
 * No browser here (vitest is node): the seams are the same fakes back.ts
 * and runtime.ts use — vv/win parameters for the measurement, stubbed
 * globalThis window/location for the probe and the debug surface.
 */

function fakeRoot() {
  const vars = new Map<string, string>()
  return {
    vars,
    style: {
      setProperty: (k: string, v: string) => void vars.set(k, v),
      removeProperty: (k: string) => void vars.delete(k),
    },
    dataset: {} as Record<string, string>,
  }
}

function fakeNode() {
  return {
    style: {} as Record<string, string>,
    dataset: {} as Record<string, string>,
  }
}

/** A VisualViewport fake whose height/offsetTop can move, firing the same
 *  `resize` event the real one fires when the keyboard opens. */
function fakeVv(height: number, offsetTop = 0) {
  const listeners: Record<'resize' | 'scroll', Set<() => void>> = {
    resize: new Set(),
    scroll: new Set(),
  }
  const vv = {
    height,
    offsetTop,
    addEventListener: (type: 'resize' | 'scroll', l: () => void) => {
      listeners[type].add(l)
    },
    removeEventListener: (type: 'resize' | 'scroll', l: () => void) => {
      listeners[type].delete(l)
    },
    /** Move the viewport and deliver the resize the real API would. */
    resizeTo: (h: number, t = 0): void => {
      ;(vv as { height: number }).height = h
      ;(vv as { offsetTop: number }).offsetTop = t
      for (const l of listeners.resize) l()
    },
    listenerCount: (type: 'resize' | 'scroll'): number => listeners[type].size,
  }
  return vv
}

function stubGlobals(search?: string): void {
  // No visualViewport on the stub in either shape: the vv path is exercised
  // through the parameter seam above, and the defaults must find none here.
  ;(globalThis as { window?: unknown }).window = { innerHeight: 874 }
  if (search !== undefined) {
    ;(globalThis as { location?: unknown }).location = { search }
  }
}

function clearStubbedGlobals(): void {
  delete (globalThis as { window?: unknown }).window
  delete (globalThis as { location?: unknown }).location
  delete (globalThis as { __gadakKeyboard?: unknown }).__gadakKeyboard
}

afterEach(clearStubbedGlobals)

describe('measure — one formula, one owner', () => {
  it('the band is what the keyboard covered: innerHeight minus the visible box', () => {
    expect(measure({ height: 700, offsetTop: 0 }, { innerHeight: 874 })).toBe(174)
  })

  it('offsetTop the viewport slid counts as covered too — a focused field near the top', () => {
    expect(measure({ height: 700, offsetTop: 10 }, { innerHeight: 874 })).toBe(164)
  })

  it('clamps at 0 — hardware keyboard, or Android under resizes-content', () => {
    expect(measure({ height: 874, offsetTop: 0 }, { innerHeight: 874 })).toBe(0)
    expect(measure({ height: 900, offsetTop: 0 }, { innerHeight: 874 })).toBe(0)
  })
})

describe('probeKbInset — the ?kb=<px> dev/e2e probe', () => {
  it('reads the fixed inset, ignores everything else on the string', () => {
    expect(probeKbInset('?kb=300')).toBe(300)
    expect(probeKbInset('?hosted&kb=120')).toBe(120)
  })

  it('absent, empty or negative is no probe — the viewport is the source', () => {
    expect(probeKbInset('')).toBeNull()
    expect(probeKbInset('?hosted')).toBeNull()
    expect(probeKbInset('?kb=abc')).toBeNull()
    expect(probeKbInset('?kb=-5')).toBeNull()
  })
})

describe('bindKeyboardBand — the root binding', () => {
  it('subscribes once and writes the band in px with the flag up', () => {
    const root = fakeRoot()
    const vv = fakeVv(700)
    const unbind = bindKeyboardBand(root as unknown as HTMLElement, vv as VvLike, {
      innerHeight: 874,
    })
    expect(vv.listenerCount('resize')).toBe(1)
    expect(vv.listenerCount('scroll')).toBe(1)
    expect(root.vars.get('--keyboard-inset')).toBe('174px')
    expect(root.dataset.keyboardUp).toBe('')
    unbind()
  })

  it('the flag comes down and the variable returns to 0 when the band closes', () => {
    const root = fakeRoot()
    const vv = fakeVv(700)
    const unbind = bindKeyboardBand(root as unknown as HTMLElement, vv as VvLike, {
      innerHeight: 874,
    })
    vv.resizeTo(874)
    expect(root.vars.get('--keyboard-inset')).toBe('0px')
    expect('keyboardUp' in root.dataset).toBe(false)
    unbind()
  })

  it('unbind removes both listeners and the writes', () => {
    const root = fakeRoot()
    const vv = fakeVv(700)
    const unbind = bindKeyboardBand(root as unknown as HTMLElement, vv as VvLike, {
      innerHeight: 874,
    })
    unbind()
    expect(vv.listenerCount('resize')).toBe(0)
    expect(vv.listenerCount('scroll')).toBe(0)
    expect(root.vars.has('--keyboard-inset')).toBe(false)
    expect('keyboardUp' in root.dataset).toBe(false)
    // A late viewport event finds no listener: the number cannot resurrect.
    vv.resizeTo(500)
    expect(root.vars.has('--keyboard-inset')).toBe(false)
  })

  it('?kb=300 writes 300px with no VisualViewport at all — the e2e rig path', () => {
    stubGlobals('?kb=300')
    const root = fakeRoot()
    const unbind = bindKeyboardBand(root as unknown as HTMLElement)
    expect(root.vars.get('--keyboard-inset')).toBe('300px')
    expect(root.dataset.keyboardUp).toBe('')
    unbind()
  })

  it('no viewport and no probe binds nothing and unbinds clean', () => {
    stubGlobals('')
    const root = fakeRoot()
    const unbind = bindKeyboardBand(root as unknown as HTMLElement)
    expect(root.vars.has('--keyboard-inset')).toBe(false)
    expect(() => unbind()).not.toThrow()
  })
})

describe('the action and the binding share one measure (GDK-1971)', () => {
  it('one input, one number, two consumers', () => {
    const vv = fakeVv(700, 10)
    const node = fakeNode()
    const root = fakeRoot()
    const action = keyboardInset(node as unknown as HTMLElement, vv as VvLike, {
      innerHeight: 874,
    })
    const unbind = bindKeyboardBand(root as unknown as HTMLElement, vv as VvLike, {
      innerHeight: 874,
    })
    expect(measure(vv, { innerHeight: 874 })).toBe(164)
    expect(node.style.transform).toBe('translateY(-164px)')
    expect(node.dataset.keyboardInset).toBe('')
    expect(root.vars.get('--keyboard-inset')).toBe('164px')
    // One event moves both: the band is one number, not two measurements.
    vv.resizeTo(874)
    expect(node.style.transform).toBe('')
    expect('keyboardInset' in node.dataset).toBe(false)
    expect(root.vars.get('--keyboard-inset')).toBe('0px')
    action?.destroy()
    unbind()
  })

  it('the action rides the probe too — the sheet clears the band under ?kb', () => {
    stubGlobals('?kb=300')
    const node = fakeNode()
    const action = keyboardInset(node as unknown as HTMLElement, undefined, { innerHeight: 874 })
    expect(node.style.transform).toBe('translateY(-300px)')
    expect(node.dataset.keyboardInset).toBe('')
    action?.destroy()
    expect(node.style.transform).toBe('')
    expect('keyboardInset' in node.dataset).toBe(false)
  })

  it('without a viewport and without the probe the action is a no-op (desktop, headless)', () => {
    stubGlobals('')
    const node = fakeNode()
    const action = keyboardInset(node as unknown as HTMLElement, undefined, { innerHeight: 874 })
    expect(action).toBeUndefined()
    expect(node.style.transform).toBeUndefined()
  })
})

describe('window.__gadakKeyboard — the last measurement, published', () => {
  /** The debug surface lives on the window (a browser global), so it is
   *  read through the stub publish() wrote to. */
  function lastDebug(): Record<string, unknown> {
    const w = (globalThis as { window?: { __gadakKeyboard?: unknown } }).window
    return w?.__gadakKeyboard as Record<string, unknown>
  }

  it('carries the inputs and the source on the viewport path', () => {
    const win = { innerHeight: 874 }
    ;(globalThis as { window?: unknown }).window = {}
    const vv = fakeVv(700, 10)
    const unbind = bindKeyboardBand(
      fakeRoot() as unknown as HTMLElement,
      vv as VvLike,
      win,
    )
    const g = lastDebug()
    expect(g.inset).toBe(164)
    expect(g.vvHeight).toBe(700)
    expect(g.offsetTop).toBe(10)
    expect(g.innerHeight).toBe(874)
    expect(g.source).toBe('vv')
    expect(typeof g.at).toBe('number')
    unbind()
  })

  it('names the probe and refuses to invent a viewport it never read', () => {
    stubGlobals('?kb=300')
    const unbind = bindKeyboardBand(fakeRoot() as unknown as HTMLElement)
    const g = lastDebug()
    expect(g.inset).toBe(300)
    expect(g.source).toBe('probe')
    expect(g.vvHeight).toBeNull()
    expect(g.offsetTop).toBeNull()
    unbind()
  })
})
