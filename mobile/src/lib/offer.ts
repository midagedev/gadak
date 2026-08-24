/*
 * Pairing offer decoder — the TypeScript mirror of internal/pairing/offer.go.
 * The contract lives in the Go file and in the shared golden vectors
 * (internal/pairing/testdata/offer-vectors.json); both suites read the same
 * file, so a change on either side turns both red.
 *
 * Contract (offer.go DecodeOffer):
 *  - input is trimmed; blank → "pairing offer: empty"
 *  - base64url, unpadded; trailing '=' from a wrapping copy is tolerated;
 *    characters outside the alphabet (including interior whitespace) →
 *    "pairing offer: not base64url"
 *  - the decoded bytes are a JSON object; a body that is not a JSON object
 *    with the right field types → "pairing offer: malformed document"
 *    (Go json.Unmarshal into Offer errors the same way)
 *  - v must be exactly 1 → otherwise "pairing offer: version N is not
 *    supported (this gadak speaks v1)"; absent v is 0 and refuses too
 *  - blank endpoint → "pairing offer: no endpoint"; empty token →
 *    "pairing offer: no token"
 *  - unknown JSON keys are ignored (a newer home may mint extra fields)
 *  - errors never quote the payload — the token inside must not leak
 *
 * ExpiresAt is advisory for the human (RFC3339, the home clock); expiry
 * enforcement belongs to the server gate, which judges from its store.
 */

export interface Offer {
  v: number
  endpoint: string
  token: string
  expires_at: string
  label: string
}

export class OfferError extends Error {}

const OFFER_V1 = 1

export function decodeOffer(input: string): Offer {
  const s = input.trim()
  if (s === '') throw new OfferError('pairing offer: empty')

  const bytes = decodeBase64url(s)
  if (bytes === null) throw new OfferError('pairing offer: not base64url')

  let doc: unknown
  try {
    doc = JSON.parse(new TextDecoder().decode(bytes))
  } catch {
    throw new OfferError('pairing offer: malformed document')
  }
  if (typeof doc !== 'object' || doc === null || Array.isArray(doc)) {
    throw new OfferError('pairing offer: malformed document')
  }
  const o = doc as Record<string, unknown>
  // Absent fields default to the Go zero values (missing v refuses as
  // version 0); present-but-wrong-type is a malformed document, matching
  // json.Unmarshal into the Offer struct.
  const v = 'v' in o ? o.v : 0
  const endpoint = 'endpoint' in o ? o.endpoint : ''
  const token = 'token' in o ? o.token : ''
  const expires_at = 'expires_at' in o ? o.expires_at : ''
  const label = 'label' in o ? o.label : ''
  if (
    typeof v !== 'number' || !Number.isInteger(v) ||
    typeof endpoint !== 'string' || typeof token !== 'string' ||
    typeof expires_at !== 'string' || typeof label !== 'string'
  ) {
    throw new OfferError('pairing offer: malformed document')
  }
  if (v !== OFFER_V1) {
    throw new OfferError(`pairing offer: version ${v} is not supported (this gadak speaks v${OFFER_V1})`)
  }
  if (endpoint.trim() === '') throw new OfferError('pairing offer: no endpoint')
  if (token === '') throw new OfferError('pairing offer: no token')
  return { v, endpoint, token, expires_at, label }
}

/** The offer expiry as a Date, or null when unset or unparseable. */
export function offerExpiry(o: Offer): Date | null {
  if (!o.expires_at) return null
  const d = new Date(o.expires_at)
  return Number.isNaN(d.getTime()) ? null : d
}

/** Advisory only — the server gate is the real judge (offer.go comment). */
export function isExpired(o: Offer, now: Date = new Date()): boolean {
  const d = offerExpiry(o)
  return d !== null && d.getTime() <= now.getTime()
}

/*
 * base64url decode mirroring base64.RawURLEncoding with the padded-input
 * tolerance of DecodeOffer: trailing '=' is stripped, interior '=' or any
 * non-alphabet character (including whitespace) is rejected, and a length
 * that is 1 mod 4 cannot decode. Implemented by hand rather than atob():
 * forgiving-base64 silently skips interior whitespace, which would accept
 * lines the Go decoder refuses.
 */
const B64URL_REVERSE = (() => {
  const map = new Int16Array(256).fill(-1)
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  for (let i = 0; i < alphabet.length; i++) map[alphabet.charCodeAt(i)] = i
  return map
})()

function decodeBase64url(s: string): Uint8Array | null {
  const t = s.replace(/=+$/, '')
  const len = t.length
  if (len % 4 === 1) return null
  const out = new Uint8Array(Math.floor((len * 3) / 4))
  let acc = 0
  let bits = 0
  let o = 0
  for (let i = 0; i < len; i++) {
    const c = t.charCodeAt(i)
    if (c > 255) return null
    const val = B64URL_REVERSE[c]
    if (val < 0) return null
    acc = (acc << 6) | val
    bits += 6
    if (bits >= 8) {
      bits -= 8
      out[o++] = (acc >> bits) & 0xff
    }
  }
  return out
}
