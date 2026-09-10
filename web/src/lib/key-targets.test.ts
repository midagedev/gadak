/*
 * GDK-693: the chord-target registry's own semantics. keymap.svelte.ts
 * dispatches through keyTarget() with null meaning "not mounted — do not
 * spend the key", so the registration lifecycle is behaviour, not wiring:
 * last-registration-wins, a null key holds nothing, and destroy must not
 * unregister a successor. Plain node objects stand in for elements — the
 * registry only ever compares identity.
 */
import { describe, expect, test } from 'vitest'
import {
  TERMINAL_STRIP_SELECTED,
  asKeyTarget,
  keyTarget,
  keyTargets,
} from './key-targets'

function el(name: string): HTMLElement {
  return { name } as unknown as HTMLElement
}

describe('asKeyTarget registration (GDK-693)', () => {
  test('registers under the key and answers; last registration wins', () => {
    const a = el('a')
    const b = el('b')
    const before = keyTargets().size
    asKeyTarget(a, 'k')
    expect(keyTarget('k')).toBe(a)
    asKeyTarget(b, 'k')
    expect(keyTarget('k')).toBe(b)
    expect(keyTargets().size - before, 'one slot, not two').toBe(1)
  })

  test('update moves the registration and clears the old slot', () => {
    const a = el('a')
    const action = asKeyTarget(a, 'first')
    expect(keyTarget('first')).toBe(a)
    // ActionReturn types update/destroy optional; the action always returns
    // both, so the calls are asserted, not guarded.
    action.update!('second')
    expect(keyTarget('first')).toBeNull()
    expect(keyTarget('second')).toBe(a)
  })

  test('a null key registers nothing — the conditional-target shape', () => {
    const a = el('a')
    // The registry is module-global, so sizes are read relative to the
    // start of this test, not to zero.
    const before = keyTargets().size
    const action = asKeyTarget(a, null)
    expect(keyTargets().size).toBe(before)
    // The strip's selected row flips between registered and not through
    // update(null) / update(key), the way row.selected drives it.
    action.update!(TERMINAL_STRIP_SELECTED)
    expect(keyTarget(TERMINAL_STRIP_SELECTED)).toBe(a)
    action.update!(null)
    expect(keyTarget(TERMINAL_STRIP_SELECTED)).toBeNull()
    expect(keyTargets().size).toBe(before)
  })

  test('destroy clears only the slot it still holds', () => {
    const a = el('a')
    const b = el('b')
    const first = asKeyTarget(a, 'k')
    const second = asKeyTarget(b, 'k')
    // b overwrote a; a's teardown must not take the slot away from b.
    first.destroy!()
    expect(keyTarget('k')).toBe(b)
    second.destroy!()
    expect(keyTarget('k')).toBeNull()
  })

  test('an unknown key answers null, never undefined', () => {
    expect(keyTarget('never-registered')).toBeNull()
  })
})
