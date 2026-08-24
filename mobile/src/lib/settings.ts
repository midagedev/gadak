/*
 * Local pairing state and the last-good queue cache.
 *
 * 1차 scaffold: localStorage (webview local storage). The Keychain plugin is
 * the planned upgrade — recorded in the plugin gap map — because localStorage
 * is not device-encrypted storage for a credential like the pairing token.
 * The token is therefore never logged and never rendered; only this module
 * reads it.
 */

import type { QueueRow } from './api'

export interface Pairing {
  endpoint: string
  token: string
  label: string
  savedAt: string
}

export interface QueueCache {
  rows: QueueRow[]
  syncedAt: string
}

const PAIRING_KEY = 'gadak-mobile.pairing'
const CACHE_KEY = 'gadak-mobile.queue-cache'

function readJSON<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key)
    return raw === null ? null : (JSON.parse(raw) as T)
  } catch {
    return null
  }
}

function writeJSON(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    /* private mode / quota — the app runs uncached, same as unpaired */
  }
}

export function readPairing(): Pairing | null {
  const p = readJSON<Pairing>(PAIRING_KEY)
  if (!p || typeof p.endpoint !== 'string' || p.endpoint === '') return null
  return { endpoint: p.endpoint, token: typeof p.token === 'string' ? p.token : '', label: p.label ?? '', savedAt: p.savedAt ?? '' }
}

export function savePairing(p: Omit<Pairing, 'savedAt'>): void {
  writeJSON(PAIRING_KEY, { ...p, savedAt: new Date().toISOString() })
}

export function clearPairing(): void {
  try {
    localStorage.removeItem(PAIRING_KEY)
  } catch {
    /* ignore */
  }
}

export function readQueueCache(): QueueCache | null {
  const c = readJSON<QueueCache>(CACHE_KEY)
  if (!c || !Array.isArray(c.rows)) return null
  return { rows: c.rows, syncedAt: typeof c.syncedAt === 'string' ? c.syncedAt : '' }
}

export function writeQueueCache(rows: QueueRow[]): void {
  writeJSON(CACHE_KEY, { rows, syncedAt: new Date().toISOString() })
}
