/*
 * Issue Navigator — favorites store ([personal], contract §2)
 *
 * Favorites (mirror DB via GET/PUT/DELETE favorites/; localStorage fallback
 * for hosted demo which answers 501 demo_read_only on writes).
 *
 * Favorites are an exception to credential gating: the loopback mirror is
 * single-user and never 401s them. Read-only features work with no credential.
 *
 * Reactivity: use svelte/reactivity SvelteSet so add/delete trigger updates.
 */

import { SvelteSet } from 'svelte/reactivity'
import * as api from '../lib/api'
import { STORAGE_KEYS } from '../lib/storage'

function favoritesKey(): string {
  return STORAGE_KEYS.favorites
}
function favoritesOrderKey(): string {
  return STORAGE_KEYS.favoritesOrder
}

function loadArray(key: string): string[] {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

function saveArray(key: string, arr: string[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(arr))
  } catch (e) {
    console.warn(`[me] ${key} 저장 실패`, e)
  }
}

class FavoritesStore {
  /* ── Favorites (server; localStorage only as hosted-demo / offline fallback) ── */
  keys = new SvelteSet<string>()
  /**
   * true while the favorites API is unreachable or read-only (hosted demo
   * service worker returns 501 demo_read_only on writes). In that mode we
   * persist to localStorage so the static demo keeps working.
   */
  #local = false

  /**
   * GET favorites/. On success, one-shot migrate any leftover localStorage
   * keys into the mirror then drop the local key. On failure (hosted demo SW
   * returns 404 for unknown GETs, or network down), load from localStorage so
   * the static demo keeps working.
   */
  async load(): Promise<void> {
    try {
      const res = await api.getFavorites()
      this.keys.clear()
      // Server returns add-order only. Prefer user drag order when present, then
      // append keys that exist only on the server (added in another window).
      const wanted = new Set(res.keys)
      for (const k of loadArray(favoritesOrderKey())) {
        if (wanted.delete(k)) this.keys.add(k)
      }
      for (const k of res.keys) {
        if (wanted.has(k)) this.keys.add(k)
      }
      this.#local = false
      await this.#migrateLocalToServer()
    } catch (e) {
      // Hosted demo has no writable favorites API; fall back to localStorage.
      console.warn('[me] 즐겨찾기 서버 로드 실패 — localStorage 폴백', e)
      this.#local = true
      this.keys.clear()
      for (const key of loadArray(favoritesKey())) this.keys.add(key)
    }
  }

  /** One-shot: local gadak:favorites → server, then clear the local key. */
  async #migrateLocalToServer(): Promise<void> {
    const local = loadArray(favoritesKey())
    if (!local.length) return
    for (const key of local) {
      if (this.keys.has(key)) continue
      try {
        await api.addFavorite(key)
        this.keys.add(key)
      } catch (e) {
        // Write rejected (e.g. 501 demo_read_only) — keep local path.
        console.warn('[me] 즐겨찾기 이관 실패 — localStorage 유지', e)
        this.#local = true
        saveArray(favoritesKey(), [...new Set([...this.keys, ...local])])
        return
      }
    }
    try {
      localStorage.removeItem(favoritesKey())
    } catch {
      /* private mode */
    }
  }

  has(key: string): boolean {
    return this.keys.has(key)
  }

  /**
   * Optimistic toggle. Server write on success; on any write failure (501
   * demo_read_only, network, …) do not roll back — persist to localStorage
   * so the hosted demo keeps working. Only write localStorage when the
   * server cannot own the set (avoids dual-source drift).
   */
  async toggle(key: string): Promise<void> {
    const wasFavorite = this.keys.has(key)
    if (wasFavorite) this.keys.delete(key)
    else this.keys.add(key)

    if (this.#local) {
      saveArray(favoritesKey(), [...this.keys])
      return
    }

    try {
      if (wasFavorite) await api.removeFavorite(key)
      else await api.addFavorite(key)
    } catch (e) {
      // Hosted demo / offline: keep the optimistic state and store locally.
      console.warn('[me] 즐겨찾기 서버 쓰기 실패 — localStorage 폴백', e)
      this.#local = true
      saveArray(favoritesKey(), [...this.keys])
    }
  }

  reorder(sourceKey: string, targetKey: string): void {
    if (sourceKey === targetKey) return
    const ordered = [...this.keys]
    const sourceIndex = ordered.indexOf(sourceKey)
    const targetIndex = ordered.indexOf(targetKey)
    if (sourceIndex < 0 || targetIndex < 0) return

    const [moved] = ordered.splice(sourceIndex, 1)
    const insertAt = ordered.indexOf(targetKey) + (sourceIndex < targetIndex ? 1 : 0)
    ordered.splice(insertAt, 0, moved)
    this.keys.clear()
    for (const key of ordered) this.keys.add(key)
    // Always persist order locally — server favorites has no order column, and
    // session-only drag order reshuffles on every refresh (regression).
    saveArray(favoritesOrderKey(), ordered)
    if (this.#local) saveArray(favoritesKey(), ordered)
  }
}

/** App-wide singleton. */
export const favorites = new FavoritesStore()
