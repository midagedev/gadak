/*
 * GDK-1986. Both device traces are in helper-textarea.ts's header: our field
 * took focus from the tap, the keyboard rose 320px, and xterm's mousedown
 * handler called focus() on its own helper 33-36ms later — after which iOS
 * put the keyboard away. Making the helper inert removed the FOCUSIN but not
 * the FOCUSOUT; the call itself is what this closes.
 */
import { describe, expect, test } from 'vitest'
import { neutraliseHelperTextarea, type HelperLike } from './helper-textarea'

function fakeHost(hasHelper: boolean) {
  const seen = { focused: 0, inert: undefined as boolean | undefined }
  const attrs: Record<string, string> = {}
  const target: HelperLike = {
    inert: false,
    focus() {
      seen.focused += 1
    },
    setAttribute(n: string, v: string) {
      attrs[n] = v
    },
    dataset: {},
  }
  return {
    seen,
    attrs,
    target,
    host: {
      querySelector: (sel: string) =>
        hasHelper && sel === 'textarea.xterm-helper-textarea' ? target : null,
    },
  }
}

describe("xterm's helper textarea on the phone (GDK-1986)", () => {
  test('focus() on it stops doing anything — the call xterm makes, closed', () => {
    const { seen, target, host } = fakeHost(true)
    target.focus()
    expect(seen.focused).toBe(1) // it did something before

    expect(neutraliseHelperTextarea(host)).toBe(true)
    target.focus()
    target.focus()
    expect(seen.focused).toBe(1) // and nothing after
  })

  test('it is also made inert, by property and by attribute', () => {
    const { attrs, target, host } = fakeHost(true)
    expect(neutraliseHelperTextarea(host)).toBe(true)
    expect(target.inert).toBe(true)
    expect(attrs.inert).toBe('')
  })

  test('it stamps that it ran, so a device probe can read it back', () => {
    const { target, host } = fakeHost(true)
    neutraliseHelperTextarea(host)
    expect(target.dataset?.gadakNeutralised).toBe('')
  })

  test('a host without one says so instead of reporting success', () => {
    const { host } = fakeHost(false)
    expect(neutraliseHelperTextarea(host)).toBe(false)
  })

  test("it asks for the helper by xterm's own class, not by tag alone", () => {
    // A bare `textarea` would match the screen's own `.ime` — the field this
    // exists to protect — and neutralise it, which is the defect inverted.
    const asked: string[] = []
    neutraliseHelperTextarea({
      querySelector: (sel: string) => {
        asked.push(sel)
        return null
      },
    })
    expect(asked).toEqual(['textarea.xterm-helper-textarea'])
  })
})
