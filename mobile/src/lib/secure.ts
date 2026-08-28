// Token storage. The packaged app's single owner is the iOS Keychain via
// the Rust commands token_get / token_set / token_del
// (mobile/src-tauri/src/lib.rs). The token never touches localStorage in a
// packaged build and never appears in a log or an error.
//
// Two slots, addressed by kind:
//   serve     — Keychain "pairing-token" / dev "gadak.dev.token"
//               Frozen: renaming the serve key silently unpairs every phone.
//   terminal  — Keychain "pairing-token-terminal" / dev "gadak.dev.token.terminal"
// An unknown kind is an error, not a silent default. Missing kind is serve
// so existing call sites keep compiling.
//
// A slot may also be host-keyed (GDK-1097 B1): passing a roster host id
// (hosts.ts — 'local' or 'paired:'+8 hex) addresses "<base key>@<hostId>"
// in the Keychain and the same shape in the dev localStorage slots.
// Omitting the host id keeps the frozen legacy slot — the address every
// pre-B1 phone's pairing lives at. Migration (store.svelte.ts boot) moves
// a legacy pairing into its host slot only after a verified copy; an
// invalid host id is an error here (slot-string injection guard), never a
// silent fallback.
//
// Dev rule: the dev webview (vite on :5180 inside the dev shell) must never
// MUTATE the device Keychain — that entry belongs to the developer's own
// pairing. Dev writes go to a dev-only localStorage slot; reads prefer that
// slot and fall back to a read-only Keychain peek so a freshly started dev
// webview can adopt the shell's existing pairing. One dev slot per kind.

const IS_DEV = import.meta.env.DEV

export type TokenKind = 'serve' | 'terminal'

const DEV_SLOT_SERVE = 'gadak.dev.token'
const DEV_SLOT_TERMINAL = 'gadak.dev.token.terminal'

/**
 * Dev localStorage key for a kind. Serve's key is frozen with the Keychain
 * slot. A host-keyed dev slot is `<dev slot>@<hostId>` — the same shape as
 * the Keychain composition in mobile/src-tauri/src/lib.rs token_slot.
 */
export function devSlot(kind: TokenKind = 'serve', hostId?: string): string {
  const base = kind === 'terminal' ? DEV_SLOT_TERMINAL : DEV_SLOT_SERVE
  return hostId === undefined ? base : `${base}@${hostId}`
}

/**
 * Roster host ids (hosts.ts): 'local' or 'paired:' + 8 lowercase hex.
 * Kept in lockstep with the Rust-side valid_host_id — a crafted id must
 * never splice arbitrary text into a Keychain or localStorage key.
 */
export function isValidHostId(hostId: string): boolean {
  return /^local$|^paired:[0-9a-f]{8}$/.test(hostId)
}

function requireHostId(hostId: string | undefined): string | undefined {
  if (hostId === undefined) return undefined
  if (!isValidHostId(hostId)) throw new Error('unknown host id')
  return hostId
}

/** Missing kind is serve. Anything else that is not a known kind throws. */
export function parseTokenKind(kind?: string | null): TokenKind {
  const k = kind ?? 'serve'
  if (k === 'serve' || k === 'terminal') return k
  throw new Error('unknown token kind')
}

function hasTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

async function invokeToken(cmd: 'token_get' | 'token_set' | 'token_del', args?: Record<string, unknown>) {
  const { invoke } = await import('@tauri-apps/api/core')
  return invoke<string | null>(cmd, args)
}

export async function tokenGet(kind: TokenKind = 'serve', hostId?: string): Promise<string | null> {
  const k = parseTokenKind(kind)
  const host = requireHostId(hostId)
  if (IS_DEV) {
    const stored = localStorage.getItem(devSlot(k, host))
    if (stored) return stored
    if (!hasTauri()) return null
    try {
      // Read-only peek at the shell's LEGACY Keychain slot: the dev
      // webview never writes host-keyed Keychain entries, so there is no
      // host-keyed entry to peek.
      return await invokeToken('token_get', { kind: k })
    } catch {
      return null
    }
  }
  if (!hasTauri()) return null
  return await invokeToken('token_get', { kind: k, host })
}

export async function tokenSet(
  token: string,
  kind: TokenKind = 'serve',
  hostId?: string,
): Promise<void> {
  const k = parseTokenKind(kind)
  const host = requireHostId(hostId)
  if (IS_DEV) {
    localStorage.setItem(devSlot(k, host), token)
    return
  }
  await invokeToken('token_set', { token, kind: k, host })
}

export async function tokenDel(kind: TokenKind = 'serve', hostId?: string): Promise<void> {
  const k = parseTokenKind(kind)
  const host = requireHostId(hostId)
  if (IS_DEV) {
    localStorage.removeItem(devSlot(k, host))
    return
  }
  await invokeToken('token_del', { kind: k, host })
}
