// Token storage. The packaged app's single owner is the iOS Keychain via
// the Rust commands token_get / token_set / token_del
// (mobile/src-tauri/src/lib.rs). The token never touches localStorage in a
// packaged build and never appears in a log or an error.
//
// Dev rule: the dev webview (vite on :5180 inside the dev shell) must never
// MUTATE the device Keychain — that entry belongs to the developer's own
// pairing. Dev writes go to a dev-only localStorage slot; reads prefer that
// slot and fall back to a read-only Keychain peek so a freshly started dev
// webview can adopt the shell's existing pairing.

const IS_DEV = import.meta.env.DEV
const DEV_SLOT = 'gadak.dev.token'

function hasTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

async function invokeToken(cmd: 'token_get' | 'token_set' | 'token_del', args?: Record<string, unknown>) {
  const { invoke } = await import('@tauri-apps/api/core')
  return invoke<string | null>(cmd, args)
}

export async function tokenGet(): Promise<string | null> {
  if (IS_DEV) {
    const dev = localStorage.getItem(DEV_SLOT)
    if (dev) return dev
    if (!hasTauri()) return null
    try {
      return await invokeToken('token_get')
    } catch {
      return null
    }
  }
  if (!hasTauri()) return null
  return await invokeToken('token_get')
}

export async function tokenSet(token: string): Promise<void> {
  if (IS_DEV) {
    localStorage.setItem(DEV_SLOT, token)
    return
  }
  await invokeToken('token_set', { token })
}

export async function tokenDel(): Promise<void> {
  if (IS_DEV) {
    localStorage.removeItem(DEV_SLOT)
    return
  }
  await invokeToken('token_del')
}
