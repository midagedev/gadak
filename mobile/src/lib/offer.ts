// Pairing-offer decoder — the TypeScript half of internal/pairing/offer.go.
// Lockstep is enforced by offer.test.ts reading the same golden vectors
// (internal/pairing/testdata/offer-vectors.json) the Go tests read.
//
// The offer carries the token, so it is a credential: errors describe the
// problem but never quote the payload, and callers must never render or log
// the decoded token.

export const OFFER_V1 = 1

export interface Offer {
  v: number
  endpoint: string
  token: string
  expires_at: string
  label: string
}

export class OfferError extends Error {}

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
 * Parses one pairing-offer line. Error messages mirror offer.go so the
 * golden vectors assert both implementations with one `error_contains`.
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
  if (v !== OFFER_V1) {
    throw new OfferError(`pairing offer: version ${v} is not supported (this gadak speaks v${OFFER_V1})`)
  }
  const endpoint = typeof o.endpoint === 'string' ? o.endpoint : ''
  if (endpoint.trim() === '') throw new OfferError('pairing offer: no endpoint')
  const token = typeof o.token === 'string' ? o.token : ''
  if (token === '') throw new OfferError('pairing offer: no token')
  return {
    v,
    endpoint,
    token,
    expires_at: typeof o.expires_at === 'string' ? o.expires_at : '',
    label: typeof o.label === 'string' ? o.label : '',
  }
}
