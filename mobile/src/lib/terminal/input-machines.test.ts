import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { LOCK_WINDOW_MS, StickyModifiers } from 'touch-remote-input'
import {
  encoderMods,
  modifierIdForBarKey,
  stepsForBarKey,
  stickySlots,
} from './keys'

/*
 * Recurrence layer for GDK-898: the phone key bar runs the shared
 * touch-remote-input machines. The library's vectors are the specification
 * for the machines themselves; these tests pin the adapter this app owns —
 * ModifierId → {ctrl,alt} for the encoder, PTY pending: 'not-needed' on the
 * flush barrier, and KeyBar showing idle/armed/locked as three states.
 */

const src = join(dirname(fileURLToPath(import.meta.url)), '../..')

function read(rel: string): string {
  return readFileSync(join(src, rel), 'utf8')
}

describe('encoderMods — PTY adapter', () => {
  it('maps control → ctrl and alt → alt', () => {
    expect(encoderMods([])).toEqual({ ctrl: false, alt: false })
    expect(encoderMods(['control'])).toEqual({ ctrl: true, alt: false })
    expect(encoderMods(['alt'])).toEqual({ ctrl: false, alt: true })
    expect(encoderMods(['control', 'alt'])).toEqual({ ctrl: true, alt: true })
  })

  it('drops shift and meta: a PTY has no meaning for them', () => {
    expect(encoderMods(['shift'])).toEqual({ ctrl: false, alt: false })
    expect(encoderMods(['meta'])).toEqual({ ctrl: false, alt: false })
    expect(encoderMods(['control', 'shift', 'meta'])).toEqual({ ctrl: true, alt: false })
    expect(encoderMods(['alt', 'shift'])).toEqual({ ctrl: false, alt: true })
  })
})

describe('StickyModifiers via the encoder adapter (vectors/sticky)', () => {
  it('single tap arms only that slot (single-tap-arms-only-that-slot)', () => {
    const sticky = new StickyModifiers()
    sticky.tap('control', 0)
    expect(sticky.slot('control')).toBe('armed')
    expect(sticky.slot('alt')).toBe('idle')
    expect(encoderMods(sticky.activeModifiers())).toEqual({ ctrl: true, alt: false })
    expect(stickySlots(sticky)).toEqual({ ctrl: 'armed', alt: 'idle' })
  })

  it('double-tap at exactly LOCK_WINDOW_MS locks (lock-boundary-is-inclusive-at-400)', () => {
    const sticky = new StickyModifiers()
    sticky.tap('control', 0)
    sticky.tap('control', LOCK_WINDOW_MS)
    expect(sticky.slot('control')).toBe('locked')
    expect(encoderMods(sticky.activeModifiers())).toEqual({ ctrl: true, alt: false })
    expect(stickySlots(sticky)).toEqual({ ctrl: 'locked', alt: 'idle' })
  })

  it('retap at 401 rearms and moves the clock (retap-at-401-rearms-and-moves-the-clock)', () => {
    const sticky = new StickyModifiers()
    sticky.tap('alt', 0)
    sticky.tap('alt', LOCK_WINDOW_MS + 1)
    expect(sticky.slot('alt')).toBe('armed')
    sticky.tap('alt', LOCK_WINDOW_MS + 1 + 200)
    expect(sticky.slot('alt')).toBe('locked')
  })

  it('consume spends armed and spares locked (consume-spends-armed-and-spares-locked)', () => {
    const sticky = new StickyModifiers()
    sticky.tap('control', 0)
    sticky.tap('alt', 0)
    sticky.tap('alt', 50)
    expect(stickySlots(sticky)).toEqual({ ctrl: 'armed', alt: 'locked' })
    sticky.consume()
    expect(sticky.slot('control')).toBe('idle')
    expect(sticky.slot('alt')).toBe('locked')
    expect(encoderMods(sticky.activeModifiers())).toEqual({ ctrl: false, alt: true })
  })

  it('tapping a locked modifier releases it', () => {
    const sticky = new StickyModifiers()
    sticky.tap('control', 0)
    sticky.tap('control', 100)
    expect(sticky.slot('control')).toBe('locked')
    sticky.tap('control', 5100)
    expect(sticky.slot('control')).toBe('idle')
    expect(encoderMods(sticky.activeModifiers())).toEqual({ ctrl: false, alt: false })
  })

  it('bar-key names map onto the library modifier ids the encoder understands', () => {
    expect(modifierIdForBarKey('ctrl')).toBe('control')
    expect(modifierIdForBarKey('alt')).toBe('alt')
    expect(modifierIdForBarKey('esc')).toBeNull()
    expect(modifierIdForBarKey('tab')).toBeNull()
  })
})

describe('stepsForBarKey — PTY flush barrier (vectors/flush)', () => {
  it('commit-marked precedes emit-key when a composition is open', () => {
    expect(stepsForBarKey('esc', true)).toEqual([
      { op: 'commit-marked' },
      { op: 'emit-key', key: 'esc', mods: [] },
    ])
  })

  it("pending is not-needed: a PTY never drops the control (the library's failure rows)", () => {
    expect(stepsForBarKey('tab', false)).toEqual([
      { op: 'emit-key', key: 'tab', mods: [] },
    ])
    expect(stepsForBarKey('esc', true, ['control'])).toEqual([
      { op: 'commit-marked' },
      { op: 'emit-key', key: 'esc', mods: ['control'] },
    ])
    for (const step of stepsForBarKey('up', true, ['alt'])) {
      expect(step.op).not.toBe('drop-control')
    }
  })
})

describe('KeyBar — three slot states, not colour alone', () => {
  const keybar = read('ui/KeyBar.svelte')
  const styles = keybar.slice(keybar.indexOf('<style>'))

  it('takes per-slot idle/armed/locked, not a boolean', () => {
    expect(keybar).toMatch(/StickySlots|SlotState/)
    expect(keybar).toMatch(/data-slot/)
    expect(styles).toMatch(/\.key\.armed\s*\{/)
    expect(styles).toMatch(/\.key\.locked\s*\{/)
  })

  it('armed and locked each carry a shape, not only a colour', () => {
    const armed = styles.match(/\.key\.armed\s*\{[^}]+\}/)?.[0]
    const locked = styles.match(/\.key\.locked\s*\{[^}]+\}/)?.[0]
    expect(armed, 'armed rule').toBeTruthy()
    expect(locked, 'locked rule').toBeTruthy()
    expect(armed).toMatch(/box-shadow/)
    expect(locked).toMatch(/box-shadow|border-bottom/)
    expect(armed).not.toEqual(locked)
    expect(armed).not.toMatch(/#[0-9a-fA-F]{3,8}/)
    expect(locked).not.toMatch(/#[0-9a-fA-F]{3,8}/)
  })

  it('does not shrink the 44pt tap floor', () => {
    const heights = [...styles.matchAll(/min-height:\s*([^;]+);/g)].map((m) => m[1].trim())
    for (const value of heights) {
      expect(value).toBe('var(--spacing-control)')
    }
  })
})

describe('Shell — the screen drives the machines, it is not the machine', () => {
  const shell = read('screens/Shell.svelte')

  it('holds StickyModifiers and passes Date.now() into tap', () => {
    expect(shell).toMatch(/new StickyModifiers\s*\(/)
    expect(shell).toMatch(/\.tap\([^)]*Date\.now\(\)/)
    expect(shell).toMatch(/\.consume\s*\(/)
  })

  it('asks for barrier steps instead of committing inline', () => {
    const start = shell.indexOf('function onBarKey')
    const end = shell.indexOf('function flushIme', start)
    const fn = shell.slice(start, end)
    expect(fn).toMatch(/stepsForBarKey|barrierSteps/)
    expect(fn).toMatch(/commit-marked/)
    expect(fn).toMatch(/ime\.composing/)
    expect(fn).toMatch(/compositionend/)
    expect(fn.indexOf('composing')).toBeLessThan(fn.indexOf('bytesForBarKey'))
    expect(fn.indexOf('compositionend')).toBeLessThan(fn.indexOf('bytesForBarKey'))
  })

  it('keeps the Chrome trailing-input guard in the screen', () => {
    expect(shell).toMatch(/lastComposeEmit/)
  })
})
