/*
 * Two-way binding between query params and app state.
 *
 * One protocol, written once. A param and the state it mirrors can each move
 * first — a pasted link or a back button moves the URL, opening a panel moves
 * the state — and the binding has to let either overwrite the other without the
 * two chasing each other in a loop. The value last written in *either*
 * direction is what tells them apart: whichever side no longer matches it is
 * the side that moved, and the other one follows.
 *
 * That shadow value used to be a `let` beside each effect in App, one per
 * param, which is how the protocol ended up transcribed three times and how its
 * correctness came to rest on the order those `let`s were initialised. Here it
 * is a closure variable, so there is exactly one of it per binding and no way
 * to read it from anywhere the protocol does not run.
 *
 * Seeding is the part worth being careful about: the shadow starts at the URL's
 * own value, never at the state's. State that was set up *from* the URL before
 * binding (a deep link that also promotes a screen, say) is then correctly seen
 * as a change the state made, and gets written out to the URL. Seed from the
 * state instead and the first pass reads that promotion as a param the URL has
 * dropped, and closes what it just opened.
 *
 * Writes are `replace`, so a selection does not fill the back stack; a real
 * navigation is still the user's own.
 */

import { untrack } from 'svelte'
import { router, setParams } from './router.svelte'

/** The value of each bound param — `null` meaning absent, both in the URL and
 *  in whatever state mirrors it. */
export type ParamValues<K extends string> = Record<K, string | null>

function readUrl<K extends string>(params: readonly K[]): ParamValues<K> {
  const out = {} as ParamValues<K>
  for (const p of params) out[p] = router.params.get(p)
  return out
}

function differs<K extends string>(
  a: ParamValues<K>,
  b: ParamValues<K>,
  params: readonly K[],
): boolean {
  return params.some((p) => a[p] !== b[p])
}

/**
 * Bind a group of params that move together.
 *
 * A group rather than N bindings when the params are only meaningful as a set —
 * three of them describing one screen, where applying them one at a time would
 * show a half-decided state. `read` reports what the params would be for the
 * current state; `write` applies a set of param values to the state.
 *
 * Call during component initialisation (it registers an `$effect`).
 */
export function bindParams<K extends string>(spec: {
  params: readonly K[]
  read: () => ParamValues<K>
  write: (next: ParamValues<K>) => void
}): void {
  const { params, read, write } = spec
  // Seeded from the URL, before the first pass — see the note above.
  let synced = readUrl(params)

  $effect(() => {
    const url = readUrl(params)
    // Read unconditionally: this is what makes the effect re-run when the state
    // moves, not only when the URL does.
    const state = read()

    if (differs(url, synced, params)) {
      synced = url
      // The write is a write, not a read: untracked so applying it cannot
      // subscribe this effect to whatever the setters happen to touch.
      untrack(() => write(url))
      return
    }
    if (differs(state, synced, params)) {
      synced = state
      setParams(state, true)
    }
  })
}

/** One param, one value. Sugar over {@link bindParams}. */
export function bindParam(spec: {
  param: string
  read: () => string | null
  write: (next: string | null) => void
}): void {
  const { param, read, write } = spec
  bindParams({
    params: [param],
    read: () => ({ [param]: read() }) as ParamValues<string>,
    write: (next) => write(next[param]),
  })
}
