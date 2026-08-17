/*
 * Recent-use history (picker ranking: assignee / transition / new issue).
 *
 * Owner: local.db via GET/POST recents/ (same rows gadak sql reads as
 * local.recents). localStorage is a first-paint / hosted-demo cache only —
 * never the authority. A one-shot absorb copies leftover cache keys into
 * the server so the promotion day does not look like data loss.
 *
 * Per kind: recent values newest-first, de-duped, cap 10.
 * Record only successful actions (caller's responsibility).
 *
 * kind examples: 'assignee', 'transition:<project>', 'create-project',
 *          'create-type:<project>', 'label'
 */

import { config } from './config'
import { recentKindPrefix } from './storage'

const MAX = 10
const HYDRATE_MS = 2000

/** In-memory cache. Seeded from localStorage, replaced by the server. */
const cache = new Map<string, string[]>()

function absorbedFlagKey(): string {
  return recentKindPrefix().replace(/:$/, '-absorbed')
}

function readLS(kind: string): string[] {
  try {
    const raw = localStorage.getItem(recentKindPrefix() + kind)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

function writeLS(kind: string, values: string[]): void {
  try {
    localStorage.setItem(recentKindPrefix() + kind, JSON.stringify(values))
  } catch {
    /* localStorage unavailable — ignore */
  }
}

function dumpLS(): Record<string, string[]> {
  const prefix = recentKindPrefix()
  const out: Record<string, string[]> = {}
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (!key || !key.startsWith(prefix)) continue
      const kind = key.slice(prefix.length)
      if (!kind) continue
      const values = readLS(kind)
      if (values.length) out[kind] = values
    }
  } catch {
    /* ignore */
  }
  return out
}

function seedFromLS(): void {
  const dump = dumpLS()
  for (const [kind, values] of Object.entries(dump)) {
    cache.set(kind, values.slice(0, MAX))
  }
}

function applyItems(items: { kind: string; value: string }[]): void {
  cache.clear()
  for (const it of items) {
    if (!it.kind || !it.value) continue
    const cur = cache.get(it.kind) ?? []
    if (!cur.includes(it.value) && cur.length < MAX) {
      cur.push(it.value)
      cache.set(it.kind, cur)
    }
  }
}

function persistCache(): void {
  const prefix = recentKindPrefix()
  try {
    const toRemove: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (key && key.startsWith(prefix)) toRemove.push(key)
    }
    for (const key of toRemove) localStorage.removeItem(key)
    for (const [kind, values] of cache) writeLS(kind, values)
  } catch {
    /* ignore */
  }
}

type RecentsDoc = { items?: { kind: string; value: string; used_at?: string }[] }

async function recentsJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(config().apiBase + path, {
    credentials: 'same-origin',
    ...init,
  })
  if (!res.ok) throw new Error(`recents ${init?.method ?? 'GET'} → ${res.status}`)
  return (await res.json()) as T
}

async function hydrateFromServer(): Promise<void> {
  const dump = dumpLS()
  let doc = await recentsJSON<RecentsDoc>('recents/')
  const flag = absorbedFlagKey()
  try {
    if (localStorage.getItem(flag) == null) {
      if (Object.keys(dump).length) {
        doc = await recentsJSON<RecentsDoc>('recents/absorb/', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ kinds: dump }),
        })
      }
      localStorage.setItem(flag, '1')
    }
  } catch {
    // Absorb failed — keep the localStorage cache and retry next load.
    if (localStorage.getItem(flag) == null && Object.keys(dump).length) return
  }
  applyItems(doc.items ?? [])
  persistCache()
}

function withTimeout(p: Promise<void>, ms: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const t = window.setTimeout(() => reject(new Error('recents hydrate timeout')), ms)
    p.then(
      () => {
        window.clearTimeout(t)
        resolve()
      },
      (err) => {
        window.clearTimeout(t)
        reject(err)
      },
    )
  })
}

function startHydrate(): Promise<void> {
  if (typeof window === 'undefined' || typeof fetch !== 'function') return Promise.resolve()
  seedFromLS()
  return withTimeout(hydrateFromServer(), HYDRATE_MS).catch(() => {
    /* keep the localStorage cache; server is unreachable (hosted demo / offline) */
  })
}

const ready: Promise<void> = startHydrate()

/** Resolves when the server hydrate (and one-shot absorb) has finished or timed out. */
export function whenReady(): Promise<void> {
  return ready
}

/** Recent values for a kind (newest first). Empty array if none. */
export function recentOf(kind: string): string[] {
  const hit = cache.get(kind)
  if (hit) return hit.slice()
  return readLS(kind)
}

/** Record a value at the front (de-dup, cap MAX). Empty values ignored. */
export function recordRecent(kind: string, value: string): void {
  if (!value) return
  const next = [value, ...recentOf(kind).filter((v) => v !== value)].slice(0, MAX)
  cache.set(kind, next)
  writeLS(kind, next)
  if (typeof fetch !== 'function') return
  void recentsJSON('recents/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, value }),
  }).catch(() => {
    /* cache already updated; next hydrate merges */
  })
}
