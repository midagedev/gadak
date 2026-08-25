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
// Dev rule: the dev webview (vite on :5180 inside the dev shell) must never
// MUTATE the device Keychain — that entry belongs to the developer's own
// pairing. Dev writes go to a dev-only localStorage slot; reads prefer that
// slot and fall back to a read-only Keychain peek so a freshly started dev
// webview can adopt the shell's existing pairing. One dev slot per kind.

const IS_DEV = import.meta.env.DEV

export type TokenKind = 'serve' | 'terminal'

const DEV_SLOT_SERVE = 'gadak.dev.token'
const DEV_SLOT_TERMINAL = 'gadak.dev.token.terminal'

/** Dev localStorage key for a kind. Serve's key is frozen with the Keychain slot. */
export function devSlot(kind: TokenKind = 'serve'): string {
  return kind === 'terminal' ? DEV_SLOT_TERMINAL : DEV_SLOT_SERVE
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

export async function tokenGet(kind: TokenKind = 'serve'): Promise<string | null> {
  const k = parseTokenKind(kind)
  if (IS_DEV) {
    const stored = localStorage.getItem(devSlot(k))
    if (stored) return stored
    if (!hasTauri()) return null
    try {
      return await invokeToken('token_get', { kind: k })
    } catch {
      return null
    }
  }
  if (!hasTauri()) return null
  return await invokeToken('token_get', { kind: k })
}

export async function tokenSet(token: string, kind: TokenKind = 'serve'): Promise<void> {
  const k = parseTokenKind(kind)
  if (IS_DEV) {
    localStorage.setItem(devSlot(k), token)
    return
  }
  await invokeToken('token_set', { token, kind: k })
}

export async function tokenDel(kind: TokenKind = 'serve'): Promise<void> {
  const k = parseTokenKind(kind)
  if (IS_DEV) {
    localStorage.removeItem(devSlot(k))
    return
  }
  await invokeToken('token_del', { kind: k })
}
