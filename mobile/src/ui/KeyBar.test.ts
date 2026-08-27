import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { StickyModifiers } from 'glasskeys'
import {
  bytesForBarKey,
  modifierIdForBarKey,
  stepsForBarKey,
  stickySlots,
} from '../lib/terminal/keys'

/*
 * GDK-953 — the panic exit, wired end to end. The machine was never broken:
 * glasskeys owns clear() and its vector ("Any UI that offers lock must also
 * offer this"). What broke was that this app's strip offered lock with no
 * way back to idle, so what is pinned here is the wiring on both sides of
 * the seam — KeyBar draws the control, Shell routes it to sticky.clear()
 * before any emission path — plus the encoder-level fact that clear is not
 * an emission. This directory's established component-test style is source
 * contracts over the .svelte files with real calls into the shared machine
 * (input-machines.test.ts does the same); there is no DOM mount harness
 * under mobile/src, and the live-DOM path is exercised by the capture walk
 * in scratch/gdk953.
 */

const here = dirname(fileURLToPath(import.meta.url))
const keybar = readFileSync(join(here, 'KeyBar.svelte'), 'utf8')
const shell = readFileSync(join(here, '..', 'screens', 'Shell.svelte'), 'utf8')

describe('GDK-953 — KeyBar draws the panic exit', () => {
  it('clear is its own bar key with a label that names the exit', () => {
    // Not a borrowed kind: a new meaning on an existing key mislabels the
    // icon, the audit class and the encoder all at once.
    //
    // The label names the target, not the action. `Clear` and `Reset` are
    // both refused here on purpose: each is an actual terminal command
    // (`clear` / Ctrl-L, `reset`), so beside Esc/Tab/Ctrl/Alt either one
    // reads as "wipe the screen" rather than "let go of the modifiers"
    // (look verdict, 2026-08-27).
    expect(keybar).toContain("{ key: 'clear', label: 'No Mods' }")
    expect(keybar).not.toMatch(/label: '(Clear|Reset)'/)
  })

  it('is persistent and honestly disabled while every slot is idle', () => {
    expect(keybar).toMatch(/anyActive\s*=\s*\$derived\(/)
    expect(keybar).toMatch(/mods\.ctrl !== 'idle'/)
    expect(keybar).toMatch(/mods\.alt !== 'idle'/)
    expect(keybar).toMatch(/disabled=\{item\.key === 'clear' && !anyActive\}/)
    // Visibly disabled, in the sibling idiom (.act:disabled, .status:disabled).
    expect(keybar).toMatch(/\.key:disabled\s*\{/)
  })

  it('rides the same .key loop as every other control — no bespoke shape', () => {
    // The 44pt floor and aria-label come from the shared {#each KEYS}
    // rendering; a special-cased clear button would have to re-earn them.
    const each = keybar.slice(keybar.indexOf('{#each KEYS'), keybar.indexOf('{/each}'))
    expect(each).toContain('class="key"')
    expect(each).toContain('onpointerdown={(e) => press(e, item.key)}')
    expect(each).not.toMatch(/\{#if item\.key === 'clear'\}/)
  })
})

describe('GDK-953 — Shell routes clear to sticky.clear()', () => {
  const start = shell.indexOf('function onBarKey')
  const end = shell.indexOf('function flushIme', start)
  const fn = shell.slice(start, end)

  it('handles clear before the modifier and emission paths, and never reaches steps', () => {
    expect(fn.indexOf("key === 'clear'")).toBeGreaterThan(-1)
    expect(fn.indexOf("key === 'clear'")).toBeLessThan(fn.indexOf('modifierIdForBarKey'))
    expect(fn.indexOf("key === 'clear'")).toBeLessThan(fn.indexOf('stepsForBarKey'))
  })

  it('the clear branch clears, syncs and refocuses the IME like the other branches', () => {
    const branch = fn.slice(fn.indexOf("key === 'clear'"), fn.indexOf('const mod'))
    expect(branch).toContain('sticky.clear()')
    expect(branch).toContain('syncMods()')
    // A dropped focus dismisses the keyboard — its own defect (GDK-953 §2).
    expect(branch).toContain('imeEl?.focus()')
    expect(branch).toContain('return')
  })

  it('replay of the wired branch: an armed/locked mix returns all slots to idle', () => {
    const sticky = new StickyModifiers()
    sticky.tap('control', 0) // armed — the state that had no way out
    sticky.tap('alt', 0)
    sticky.tap('alt', 50) // locked
    expect(stickySlots(sticky)).toEqual({ ctrl: 'armed', alt: 'locked' })
    sticky.clear() // exactly what the branch calls
    expect(stickySlots(sticky)).toEqual({ ctrl: 'idle', alt: 'idle' })
  })
})

describe('GDK-953 — clear is not an emission', () => {
  it('sends no bytes under any modifier mix', () => {
    expect(bytesForBarKey('clear', { ctrl: true, alt: true }, 'application')).toHaveLength(0)
  })

  it('produces no barrier steps, not even over an open composition', () => {
    expect(stepsForBarKey('clear', true, ['control'])).toEqual([])
  })

  it('is not a modifier: the modifier branch must not claim it', () => {
    expect(modifierIdForBarKey('clear')).toBeNull()
  })
})
