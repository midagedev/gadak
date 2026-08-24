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

import type { MessageKey } from './i18n'

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

/*
 * ── Scan/paste line → offer string (GDK-799 QR consumption) ──
 *
 * The desktop mints the offer as one bare base64url line (`gadak pairing
 * mint`); a QR wraps the same line as `gadak://pair?code=<offer>` — the
 * deeplink grammar's action-as-host shape (internal/deeplink), host folded
 * case-insensitively like Parse does. Nothing here invents a format: a
 * line carrying no scheme is a bare offer passed through verbatim for
 * decodeOffer to judge (the base64url alphabet cannot contain "://"), and
 * a line that claims a scheme must be exactly the pair link with a
 * non-empty code. Errors quote no payload, same rule as the decoder.
 */
const SCHEME_RE = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//

/** The offer string carried by one scanned or pasted line. */
export function offerFromLine(line: string): string {
  const s = line.trim()
  if (!SCHEME_RE.test(s)) return s
  let u: URL
  try {
    u = new URL(s)
  } catch {
    throw new OfferError('pairing link: malformed URL')
  }
  if (u.protocol.toLowerCase() !== 'gadak:') throw new OfferError('pairing link: not a gadak:// link')
  if (u.host.toLowerCase() !== 'pair') throw new OfferError('pairing link: not a pair link')
  const code = u.searchParams.get('code')
  if (code === null || code.trim() === '') throw new OfferError('pairing link: no code')
  return code
}

/*
 * ── Connect-proof verdicts (GDK-800 save→bootstrap) ──
 *
 * Pure mapping for the pair screen's one-bootstrap proof. It lives beside
 * the codec — not inside Pair.svelte — because vitest runs node-only
 * (vite.config test.environment) and this mapping is the branch axis the
 * spec tests. Structural on ApiError ({status, code, message}); offer.ts
 * imports nothing at runtime so codec consumers do not gain the transport
 * graph.
 */
export type ConnectFailureKind = 'timeout' | 'network' | 'rejected' | 'http'

export interface ConnectFailure {
  kind: ConnectFailureKind
  /** HTTP status when a response arrived — 'http' is the catch-all bucket. */
  status: number | undefined
}

/**
 * Classifies a failed proof bootstrap. Timeout is the 25s door closing
 * (AbortSignal.timeout rejects as TimeoutError, a manual abort as
 * AbortError); pairing_rejected teaches re-pair; forbidden_host and a
 * thrown fetch teach network (api.ts code doc: forbidden_host = "wrong
 * network"); everything else — any HTTP status, a bad body shape — is the
 * generic bucket.
 */
export function connectFailure(err: unknown): ConnectFailure {
  const name = (err as { name?: unknown } | null | undefined)?.name
  if (name === 'AbortError' || name === 'TimeoutError') {
    return { kind: 'timeout', status: undefined }
  }
  const e = err as { message?: unknown; code?: unknown; status?: unknown }
  const message = typeof e?.message === 'string' ? e.message : ''
  const code = typeof e?.code === 'string' ? e.code : ''
  const status = typeof e?.status === 'number' ? e.status : undefined
  if (code === 'pairing_rejected') return { kind: 'rejected', status }
  if (code === 'forbidden_host' || message === 'network') return { kind: 'network', status }
  return { kind: 'http', status }
}

/** The message key a verdict renders as (no params — the reason line carries detail). */
export function connectFailureMessage(f: ConnectFailure): MessageKey {
  if (f.kind === 'timeout') return 'pair.connect.timeout'
  if (f.kind === 'network') return 'pair.connect.network'
  if (f.kind === 'rejected') return 'pair.connect.rejected'
  return 'pair.connect.http'
}
