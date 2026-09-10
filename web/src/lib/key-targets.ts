/*
 * The DOM targets the global chords aim at (GDK-693).
 *
 * keymap.svelte.ts used to find them with document.querySelector at dispatch
 * time — the last global-selector dispatcher. A selector answers for the whole
 * document, which was only ever harmless because the chords' `when` guards
 * happen to match the mounted view; the registry instead answers with exactly
 * what the owning component registered, so an unmounted target is a null the
 * dispatch already treats as a no-op (the focus-narrow precedent: no field
 * means the key is not spent).
 *
 * Keys are the same testid vocabulary the elements already carry
 * (NARROW_FIELD_TESTID / DETAIL_TESTID values), so a registration site reads
 * as "this is the element that testid names". The one exception is the
 * terminal strip, where many rows share one testid and the *selected* row is
 * the target — it registers under its own name below.
 *
 * Plain .ts on purpose: no runes, no store imports, no window access — the
 * registry is a Map the keymap's node tests can exercise directly. Last
 * registration wins, and destroy only clears the slot when it still holds
 * that element, so a remount that outlives its predecessor cannot unregister
 * the successor.
 */
import type { ActionReturn } from 'svelte/action'

/** The terminal strip's selected row — the focus target of Ctrl+Shift+`. */
export const TERMINAL_STRIP_SELECTED = 'terminal-strip-selected'

const targets = new Map<string, HTMLElement>()

/** The live element registered under `key`, or null when none is mounted. */
export function keyTarget(key: string): HTMLElement | null {
  return targets.get(key) ?? null
}

/** Everything currently registered, keyed — the debug read (`stats()` idiom). */
export function keyTargets(): ReadonlyMap<string, HTMLElement> {
  return targets
}

/**
 * Register this element under `key` for as long as it is mounted. A null/empty
 * key registers nothing, which is how a conditional target (the strip's
 * selected row) says "not it" reactively: the action's update runs on every
 * param change, so flipping the condition moves the registration.
 */
export function asKeyTarget(
  node: HTMLElement,
  key: string | null | undefined,
): ActionReturn<string | null | undefined> {
  let held: string | null = null
  const set = (next: string | null | undefined): void => {
    if (held !== null && targets.get(held) === node) targets.delete(held)
    held = next ? next : null
    if (held !== null) targets.set(held, node)
  }
  set(key)
  return {
    update: set,
    destroy: () => set(null),
  }
}
