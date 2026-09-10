import type { MessageKey } from '../i18n'

/*
 * One owner for refusal copy (GDK-1121): the server said "no" to *you*, and
 * the reason travels in the HTTP status plus the `error` code. The GDK-1120
 * incident measured what collapsing them costs — a packaged phone whose
 * every POST answered 403 forbidden_origin, while the screen said only the
 * network line, because the shared classifier folds every 401/403 into one
 * 'forbidden' cause (web/src/lib/terminal/protocol.ts — correct for the web,
 * which never sees forbidden_origin; the phone is the host that does). The
 * server side leaves one guard log line, so this screen is the only
 * user-visible diagnostic path.
 *
 * Two readers share this map: the terminal strip (Shell.svelte renders the
 * key on an unavailable pane) and api.ts errorMessage() (every other
 * surface's error line). Neither may spell a refusal sentence of its own.
 *
 * Honest limit, measured: the REST create path (POST sessions/) carries the
 * status and code, and that is where refusal kinds are decided. The
 * WebSocket upgrade cannot join — a browser close event carries no HTTP
 * status, so a 403 on the upgrade itself still reads as 'network'. A shell
 * that dies *after* opening is a dropped session, not a refusal.
 */

/** Which "no" the server said — each kind owns one catalog sentence. */
export type RefusalKind = 'origin' | 'pairing' | 'scope' | 'other'

/** The catalog key per kind. Keyed copy, never literal sentences. */
export const REFUSAL_KEYS: Record<RefusalKind, MessageKey> = {
  // 403 forbidden_origin: the request arrived with an Origin the guard does
  // not exempt. The pairing is fine; the requests are not. The desktop fix
  // is the paired-app origin exemption (GDK-1120) — an old serve is the
  // usual reason a phone still sees this.
  origin: 'terminal.refusal.origin',
  // 401 pairing_rejected: the offer/token itself was refused.
  pairing: 'terminal.refusal.pairing',
  // 403 forbidden_host / scope_rejected: the token cannot read this.
  scope: 'terminal.refusal.scope',
  // Any other 401/403, including a body with no code.
  other: 'terminal.refusal.other',
}

/**
 * (status, code) → kind, or null when this is not a refusal. Only 401/403
 * are refusal-class; everything else (network, 5xx, 404) keeps its existing
 * copy and is none of this module's business.
 */
export function classifyRefusal(status: number, code: string | null): RefusalKind | null {
  if (status !== 401 && status !== 403) return null
  if (code === 'forbidden_origin') return 'origin'
  if (code === 'pairing_rejected') return 'pairing'
  if (code === 'forbidden_host' || code === 'scope_rejected') return 'scope'
  return 'other'
}
