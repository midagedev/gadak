// Pairing-offer decoder — the TypeScript half of internal/pairing/offer.go.
// Lockstep is enforced by offer.test.ts reading the same golden vectors
// (internal/pairing/testdata/offer-vectors.json) the Go tests read.
//
// The offer carries the token, so it is a credential: errors describe the
// problem but never quote the payload, and callers must never render or log
// the decoded token.

export const OFFER_V1 = 1
export const OFFER_V2 = 2

/** One scoped credential inside an offer. v1's payload cannot name its
 * token's scope (the scope lives in the server's store), so the normalized
 * v1 entry carries `null` — a caller that needs the scope decides from
 * context, exactly as it did before v2 existed. */
export interface OfferToken {
  scope: string | null
  token: string
}

export interface Offer {
  v: number
  endpoint: string
  /** v1's single token. Empty for v2, which carries its tokens in the
   * list below — one per scope, no top-level credential. */
  token: string
  expires_at: string
  label: string
  /** Every token the offer carries, in payload order. v1 decodes to one
   * unscoped entry; v2 keeps its scoped entries as minted. */
  tokens: OfferToken[]
}

export class OfferError extends Error {}

/**
 * The offer decoded fine but carries no token this surface can use (e.g. a
 * terminal-only offer in the mirror's pairing field). The message is the
 * user's sentence — authored here, next to the decoder's other refusals
 * (one vocabulary owner), and screens pass it through rather than
 * re-wording it.
 */
export class OfferScopeError extends Error {}

function fromBase64Url(s: string): Uint8Array | null {
  // atob speaks std base64; map the url alphabet onto it and re-pad.
  const std = s.replace(/-/g, '+').replace(/_/g, '/')
  const padded = std + '='.repeat((4 - (std.length % 4)) % 4)
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(padded)) return null
  try {
    const bin = atob(padded)
    const out = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
    return out
  } catch {
    return null
  }
}

/**
 * Parses one pairing-offer line, v1 or v2. Error messages mirror offer.go
 * so the golden vectors assert both implementations with one
 * `error_contains`.
 */
export function decodeOffer(input: string): Offer {
  const s = input.trim()
  if (s === '') throw new OfferError('pairing offer: empty')
  // Go tolerates padded input from a copy that wrapped std encoding; the
  // url-alphabet mapping above already accepts both, minus the padding.
  const data = fromBase64Url(s.replace(/=+$/, ''))
  if (data === null) throw new OfferError('pairing offer: not base64url')
  let doc: unknown
  try {
    doc = JSON.parse(new TextDecoder().decode(data))
  } catch {
    throw new OfferError('pairing offer: malformed document')
  }
  if (typeof doc !== 'object' || doc === null || Array.isArray(doc)) {
    throw new OfferError('pairing offer: malformed document')
  }
  const o = doc as Record<string, unknown>
  const v = typeof o.v === 'number' ? o.v : 0
  if (v !== OFFER_V1 && v !== OFFER_V2) {
    throw new OfferError(`pairing offer: version ${v} is not supported (this gadak speaks v1 and v2)`)
  }
  const endpoint = typeof o.endpoint === 'string' ? o.endpoint : ''
  if (endpoint.trim() === '') throw new OfferError('pairing offer: no endpoint')
  const token = typeof o.token === 'string' ? o.token : ''
  const base = {
    v,
    endpoint,
    expires_at: typeof o.expires_at === 'string' ? o.expires_at : '',
    label: typeof o.label === 'string' ? o.label : '',
  }
  if (v === OFFER_V1) {
    if (Array.isArray(o.tokens)) throw new OfferError('pairing offer: v1 carries one token, not a list')
    if (token === '') throw new OfferError('pairing offer: no token')
    return { ...base, token, tokens: [{ scope: null, token }] }
  }
  // v2 (GDK-1498): one token per scope, no top-level token. Each
  // malformed shape is its own sentence, mirroring Go.
  if (token !== '') {
    throw new OfferError('pairing offer: v2 carries its tokens as a list — a top-level token is the v1 shape')
  }
  if (!Array.isArray(o.tokens) || o.tokens.length === 0) {
    throw new OfferError('pairing offer: v2 carries no tokens')
  }
  const seen = new Set<string>()
  const tokens: OfferToken[] = []
  for (const entry of o.tokens) {
    if (typeof entry !== 'object' || entry === null || Array.isArray(entry)) {
      throw new OfferError('pairing offer: a v2 token entry has no scope')
    }
    const e = entry as Record<string, unknown>
    const scope = typeof e.scope === 'string' ? e.scope : ''
    if (scope === '') throw new OfferError('pairing offer: a v2 token entry has no scope')
    const tok = typeof e.token === 'string' ? e.token : ''
    if (tok === '') throw new OfferError('pairing offer: a v2 token entry has no token')
    if (seen.has(scope)) {
      throw new OfferError(`pairing offer: v2 carries two tokens for scope "${scope}"`)
    }
    seen.add(scope)
    tokens.push({ scope, token: tok })
  }
  return { ...base, token: '', tokens }
}

/** The structural minimum the scope pickers need — a decoded Offer, or
 * any caller-shaped object with the same fields. */
interface TokenBearer {
  token: string
  tokens?: OfferToken[]
}

/** The token a REST client (this app) should present: the serve token
 * when the offer carries one, else the v1 token. A terminal token is
 * never the pairing credential — it opens a shell, not the mirror. */
export function serveTokenOf(offer: TokenBearer): string | null {
  for (const t of offer.tokens ?? []) {
    if (t.scope === 'serve') return t.token
  }
  return offer.token !== '' ? offer.token : null
}

/** The offer's terminal token, when it carries one — the credential the
 * terminal pane presents. Null for v1 and for v2 offers minted without
 * the terminal scope. */
export function terminalTokenOf(offer: TokenBearer): string | null {
  for (const t of offer.tokens ?? []) {
    if (t.scope === 'terminal') return t.token
  }
  return null
}
