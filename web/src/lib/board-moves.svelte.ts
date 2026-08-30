/*
 * Which card moved (GDK-1175) — the reactive half. The judgment itself lives
 * in ./board-moves, which is plain TypeScript so it can be tested without the
 * svelte plugin.
 */

import { MOVE_FRESH_MS } from './board-moves'

export { MOVE_FRESH_MS, movedExternally } from './board-moves'

class ExternalMoves {
  /** key → when it was flagged. Reactive so the board's chip re-renders. */
  #at = $state(new Map<string, number>())

  /** Record a batch of externally-moved keys (called from the delta apply). */
  note(keys: readonly string[], nowMs: number = Date.now()): void {
    if (keys.length === 0) return
    const next = new Map(this.#at)
    for (const k of keys) next.set(k, nowMs)
    this.#at = next
  }

  /** The keys still fresh enough to animate and highlight. */
  fresh(nowMs: number = Date.now()): Set<string> {
    const out = new Set<string>()
    for (const [k, at] of this.#at) if (nowMs - at < MOVE_FRESH_MS) out.add(k)
    return out
  }

  /** Drop a key's flag once the board has spent it on one animation. */
  clear(key: string): void {
    if (!this.#at.has(key)) return
    const next = new Map(this.#at)
    next.delete(key)
    this.#at = next
  }
}

export const externalMoves = new ExternalMoves()
