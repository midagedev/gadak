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

/*
 * GDK-951 — locked must not read as armed at arm's length.
 *
 * Both states used to paint the same ground (--color-accent-subtle) and the
 * same ink (--color-accent-text), and differ only in a stroke: a 1px inset
 * ring against a 2px bottom rule. At the distance a phone is actually held,
 * with a thumb over the key, one stroke weight against another is not a
 * state — a person cannot tell "the next letter is Ctrl-something" from
 * "every letter is Ctrl-something until I say stop", which is the difference
 * between ^C and a shell full of control bytes.
 *
 * So the axis this pins is FILL, not stroke: the two classes must paint
 * different grounds, and locked must invert its glyph against that ground.
 * A stroke-only difference passes every colour-contrast check and still
 * fails the only question the strip is asked.
 *
 * Source contract, in this directory's established style (there is no DOM
 * mount harness under mobile/src). Tokens only — the strings asserted are
 * var() names, so a palette change moves both states together and this
 * gate keeps holding.
 */
describe('GDK-951 — armed and locked differ in fill, not only in stroke', () => {
  /** The declaration block of a single CSS rule, by selector. */
  function ruleBody(selector: string): string {
    const at = keybar.indexOf(`${selector} {`)
    expect(at, `KeyBar.svelte must declare ${selector}`).toBeGreaterThan(-1)
    return keybar.slice(at + selector.length + 2, keybar.indexOf('}', at))
  }

  /** One property's value inside a rule body, or null when unset. */
  function decl(body: string, prop: string): string | null {
    const m = new RegExp(`(?:^|;|\\n)\\s*${prop}\\s*:\\s*([^;]+);`).exec(body)
    return m ? m[1].trim() : null
  }

  const armed = ruleBody('.key.armed')
  const locked = ruleBody('.key.locked')

  it('both states paint a ground, and the two grounds are not the same', () => {
    const a = decl(armed, 'background')
    const l = decl(locked, 'background')
    expect(a, 'armed must declare a background').not.toBeNull()
    expect(l, 'locked must declare a background').not.toBeNull()
    expect(l, 'locked and armed must not share one fill').not.toBe(a)
  })

  it('locked inverts its glyph against its own fill; armed keeps accent ink on a tint', () => {
    // The pair is theme-safe by construction: --color-accent-text is the
    // accent thread that always contrasts the ground, --color-bg-base is
    // always the ground, so the inversion holds in light, dark, ink and
    // ember without a fifth colour being invented for it.
    expect(decl(locked, 'background')).toBe('var(--color-accent-text)')
    expect(decl(locked, 'color')).toBe('var(--color-bg-base)')
    expect(decl(armed, 'background')).toBe('var(--color-accent-subtle)')
    expect(decl(armed, 'color')).toBe('var(--color-accent-text)')
  })

  it('no new colour: every value in either state is a token', () => {
    for (const [name, body] of [
      ['armed', armed],
      ['locked', locked],
    ] as const) {
      expect(body, `${name} must not carry a hex literal`).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
      expect(body, `${name} must not carry an rgb()/hsl() literal`).not.toMatch(/\b(rgb|hsl)a?\(/)
    }
  })

  it('the state survives a stroke being removed — fill alone still separates them', () => {
    // Strip every box-shadow and the two blocks must still disagree. This
    // is the exact regression: a future round tidying the shadows away
    // would have silently restored the defect.
    const noStroke = (s: string): string => s.replace(/box-shadow\s*:[^;]+;/g, '')
    expect(noStroke(locked).trim()).not.toBe(noStroke(armed).trim())
    expect(decl(noStroke(locked), 'background')).not.toBe(decl(noStroke(armed), 'background'))
  })
})
