/*
 * Issue Navigator — watch set store ([personal], contract §2)
 *
 * Watch set (SvelteSet, optimistic toggle + rollback). Loaded when identity
 * appears; cleared when it disappears. Requires a configured credential for
 * toggles (`me.identified`).
 *
 * Reactivity: use svelte/reactivity SvelteSet so add/delete trigger updates.
 */

import { SvelteSet } from 'svelte/reactivity'
import * as api from '../lib/api'
import { me } from './me.svelte'

class WatchesStore {
  /* ── Watches ── */
  keys = new SvelteSet<string>()

  clear(): void {
    this.keys.clear()
  }

  async load(): Promise<void> {
    try {
      const res = await api.getWatches()
      this.keys.clear()
      for (const k of res.keys) this.keys.add(k)
    } catch (e) {
      console.warn('[me] 워치 로드 실패', e)
    }
  }

  has(key: string): boolean {
    return this.keys.has(key)
  }

  /** Optimistic toggle; rolls back on failure. Returns false when not identified. */
  async toggle(key: string): Promise<boolean> {
    if (!me.identified) return false
    const wasWatching = this.keys.has(key)
    if (wasWatching) this.keys.delete(key)
    else this.keys.add(key)
    try {
      if (wasWatching) await api.removeWatch(key)
      else await api.addWatch(key)
      return true
    } catch (e) {
      console.warn('[me] 워치 토글 실패(롤백)', e)
      if (wasWatching) this.keys.add(key)
      else this.keys.delete(key)
      return false
    }
  }
}

/** App-wide singleton. */
export const watches = new WatchesStore()
