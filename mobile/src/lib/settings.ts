/*
 * Local pairing state and the last-good queue cache.
 *
 * The token is a credential: it lives in the device secure store (iOS
 * Keychain / Android Keystore / desktop OS keyring) behind the token_get/
 * token_set/token_del commands in src-tauri/src/lib.rs — this module is
 * their only caller. The plugin underneath (tauri-plugin-secure-storage)
 * is wrapped in Rust, so the JS side never speaks it directly and a plugin
 * swap touches src-tauri alone.
 *
 * endpoint and label are not secrets (they already appear in the UI) and
 * stay in localStorage. The token is therefore never logged and never
 * rendered; only this module reads it. Older scaffold builds kept the token
 * inside the localStorage pairing entry — readToken() migrates that copy
 * into the secure store once and deletes it from localStorage only after
 * the secure write succeeded.
 */

import { invoke } from '@tauri-apps/api/core'
import type { QueueRow } from './api'

export interface Pairing {
  endpoint: string
  label: string
  savedAt: string
}

export interface QueueCache {
  rows: QueueRow[]
  syncedAt: string
}

const PAIRING_KEY = 'gadak-mobile.pairing'
const CACHE_KEY = 'gadak-mobile.queue-cache'
// DEV/vitest only — the browser has no secure store. NOT the packaged
// app's token store (see readToken).
const DEV_TOKEN_KEY = 'gadak-mobile.token'

/** True inside a Tauri webview (packaged app or tauri-build preview). */
export function inTauriApp(): boolean {
  return typeof globalThis !== 'undefined' && '_TAURI_INTERNALS_' in globalThis
}

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

/** The token inside a legacy (scaffold) pairing entry, '' when absent. */
function legacyToken(): string {
  const p = readJSON<{ token?: unknown }>(PAIRING_KEY)
  return typeof p?.token === 'string' ? p.token : ''
}

/** Rewrites the pairing entry without its token field (post-migration). */
function stripLegacyToken(): void {
  const p = readJSON<Record<string, unknown>>(PAIRING_KEY)
  if (p === null || typeof p !== 'object') return
  delete p.token
  writeJSON(PAIRING_KEY, p)
}

// DEV-only fallbacks (no Tauri runtime → no Keychain; vitest included).
function devReadToken(): string {
  try {
    return localStorage.getItem(DEV_TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

function devWriteToken(token: string): void {
  try {
    localStorage.setItem(DEV_TOKEN_KEY, token)
  } catch {
    /* quota / private mode — the app runs unpaired */
  }
}

function devClearToken(): void {
  try {
    localStorage.removeItem(DEV_TOKEN_KEY)
  } catch {
    /* ignore */
  }
}

/**
 * The pairing token. In the packaged app this reads the secure store and
 * performs the one-time migration out of localStorage. Never rejects: an
 * unreachable store degrades to '' (requests go unauthenticated and the
 * gate's 401 pairing_rejected surfaces, instead of a boot-time crash).
 */
export async function readToken(): Promise<string> {
  if (!inTauriApp()) return devReadToken()
  let stored: string | null = null
  try {
    stored = await invoke<string | null>('token_get')
  } catch {
    /* store unreachable (locked device) — fall through to the legacy copy */
  }
  if (stored !== null && stored !== '') return stored
  const legacy = legacyToken()
  if (legacy === '') return ''
  try {
    await invoke('token_set', { token: legacy })
    stripLegacyToken()
  } catch {
    /* secure write failed — retried on the next read */
  }
  return legacy
}

/**
 * Saves the pairing. The token write can reject (secure store down) — the
 * caller surfaces that instead of claiming success; the meta is already
 * written, and a later successful save overwrites both halves.
 */
export async function savePairing(p: Omit<Pairing, 'savedAt'> & { token: string }): Promise<void> {
  writeJSON(PAIRING_KEY, { endpoint: p.endpoint, label: p.label, savedAt: new Date().toISOString() })
  if (inTauriApp()) {
    await invoke('token_set', { token: p.token })
  } else {
    devWriteToken(p.token)
  }
}

/**
 * Removes the pairing. Token deletion failures are swallowed on purpose:
 * without the meta the token is unreachable through the app, and the next
 * save overwrites it — blocking "unpair" on a Keychain hiccup would trade
 * a real UX stop for a theoretical residue.
 */
export async function clearPairing(): Promise<void> {
  try {
    localStorage.removeItem(PAIRING_KEY)
  } catch {
    /* ignore */
  }
  if (inTauriApp()) {
    try {
      await invoke('token_del')
    } catch {
      /* see above */
    }
  } else {
    devClearToken()
  }
}

/** Pairing meta (endpoint/label) — sync, from localStorage. */
export function readPairing(): Pairing | null {
  const p = readJSON<Pairing & { token?: unknown }>(PAIRING_KEY)
  if (!p || typeof p.endpoint !== 'string' || p.endpoint === '') return null
  return {
    endpoint: p.endpoint,
    label: typeof p.label === 'string' ? p.label : '',
    savedAt: typeof p.savedAt === 'string' ? p.savedAt : '',
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
