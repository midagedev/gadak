/*
 * Pair-screen flow units (GDK-799 QR consumption + GDK-800 save→proof):
 * the scan-line extractor, the decode→preview→expired save gate, and the
 * connect-failure → message mapping. Component wiring is svelte-check's
 * and the build's; these pin the branch axes the spec names. The golden
 * decoder vectors stay in offer.test.ts, untouched.
 */
import { describe, expect, it } from 'vitest'
import {
  connectFailure,
  connectFailureMessage,
  decodeOffer,
  isExpired,
  offerFromLine,
  OfferError,
  type ConnectFailureKind,
} from './offer'
import { t } from './i18n'

/** Test-side encoder — the same mirror offer.test.ts uses for ad-hoc cases. */
function encodeBase64url(s: string): string {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function offerLine(
  o: {
    endpoint?: string
    token?: string
    expires_at?: string
    label?: string
  } = {},
): string {
  return encodeBase64url(
    JSON.stringify({
      v: 1,
      endpoint: o.endpoint ?? 'https://home.example.ts.net',
      token: o.token ?? 'test-token',
      expires_at: o.expires_at ?? '',
      label: o.label ?? 'desk',
    }),
  )
}

describe('offerFromLine — one reader for scan and paste', () => {
  it('passes a bare offer line through for decodeOffer to judge', () => {
    const bare = offerLine({ token: 'bare-tok' })
    expect(offerFromLine(bare)).toBe(bare)
    expect(offerFromLine(`  ${bare}\n`)).toBe(bare)
  })

  it('extracts the code from gadak://pair?code=', () => {
    const bare = offerLine({ token: 'qr-tok' })
    expect(offerFromLine(`gadak://pair?code=${bare}`)).toBe(bare)
    // Host folds case-insensitively, like internal/deeplink Parse.
    expect(offerFromLine(`GADAK://Pair?code=${bare}`)).toBe(bare)
    // searchParams percent-decodes: a QR writer that escaped the payload
    // still hands decodeOffer the original bytes (base64url needs no
    // escaping, so the escape is built by hand to make the case real).
    const esc = `%${bare.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0')}${bare.slice(1)}`
    expect(esc).not.toBe(bare)
    expect(offerFromLine(`gadak://pair?code=${esc}`)).toBe(bare)
  })

  it('decodes a scanned link end to end', () => {
    const decoded = decodeOffer(offerFromLine(`gadak://pair?code=${offerLine({ token: 'e2e' })}`))
    expect(decoded.endpoint).toBe('https://home.example.ts.net')
    expect(decoded.token).toBe('e2e')
  })

  it('refuses a URL that is not a gadak:// pair link', () => {
    const cases = [
      `https://pair?code=${offerLine()}`,
      `orca://pair?code=${offerLine()}`,
      'gadak://view?pj=GDK&sc=inprogress',
      'gadak://issue/GDK-119',
    ]
    for (const c of cases) {
      expect(() => offerFromLine(c)).toThrow(OfferError)
    }
  })

  it('refuses a pair link with a missing or empty code', () => {
    expect(() => offerFromLine('gadak://pair')).toThrow(OfferError)
    expect(() => offerFromLine('gadak://pair?code=')).toThrow(OfferError)
    expect(() => offerFromLine('gadak://pair?other=x')).toThrow(OfferError)
    expect(() => offerFromLine('gadak://pair?code=%20')).toThrow(OfferError)
  })

  it('never echoes the payload in link errors', () => {
    const secret = 'SUPERSECRET-TOKEN-VALUE'
    const link = `gadak://pair?code=${offerLine({ token: secret })}`
    for (const c of [`https://x?code=${secret}`, 'gadak://pair?code=']) {
      try {
        offerFromLine(c)
        expect.unreachable('unexpectedly accepted')
      } catch (err) {
        if (!(err instanceof OfferError)) throw err
        expect(err.message).not.toContain(secret)
        expect(err.message).not.toContain(link)
      }
    }
  })
})

describe('decode → preview → save gate', () => {
  it('a live offer previews and is savable', () => {
    const decoded = decodeOffer(
      offerLine({ label: 'studio', expires_at: '2030-01-01T00:00:00Z' }),
    )
    expect(decoded.endpoint).toBe('https://home.example.ts.net')
    expect(decoded.label).toBe('studio')
    expect(isExpired(decoded, new Date('2026-08-25T00:00:00Z'))).toBe(false)
    // The save gate is the same expression the button disables on.
    expect(decoded !== null && !isExpired(decoded)).toBe(true)
  })

  it('an expired offer is blocked from saving', () => {
    const decoded = decodeOffer(offerLine({ expires_at: '2020-01-01T00:00:00Z' }))
    expect(isExpired(decoded, new Date('2026-08-25T00:00:00Z'))).toBe(true)
    expect(decoded !== null && !isExpired(decoded)).toBe(false)
  })

  it('an unset expiry is unknown, not expired — still savable', () => {
    const decoded = decodeOffer(offerLine({ expires_at: '' }))
    expect(isExpired(decoded, new Date('2026-08-25T00:00:00Z'))).toBe(false)
  })
})

describe('connect-failure → message mapping', () => {
  const cases: Array<{ err: unknown; kind: ConnectFailureKind }> = [
    { err: Object.assign(new Error('The operation was aborted'), { name: 'TimeoutError' }), kind: 'timeout' },
    { err: Object.assign(new Error('The operation was aborted'), { name: 'AbortError' }), kind: 'timeout' },
    { err: { message: 'network' }, kind: 'network' },
    { err: { message: 'http 403 forbidden_host', status: 403, code: 'forbidden_host' }, kind: 'network' },
    { err: { message: 'http 401 pairing_rejected', status: 401, code: 'pairing_rejected' }, kind: 'rejected' },
    { err: { message: 'http 500 internal_error', status: 500, code: 'internal_error' }, kind: 'http' },
    { err: { message: 'shape' }, kind: 'http' },
    { err: new Error('anything else'), kind: 'http' },
  ]

  it('classifies each verdict axis', () => {
    for (const c of cases) {
      expect(connectFailure(c.err).kind).toBe(c.kind)
    }
  })

  it('carries the HTTP status through for the reason line', () => {
    expect(connectFailure({ message: 'http 418', status: 418, code: 'teapot' })).toEqual({
      kind: 'http',
      status: 418,
    })
    expect(connectFailure({ message: 'network' }).status).toBeUndefined()
  })

  it('each verdict renders a distinct sentence, in both locales', () => {
    const keys = cases.map((c) => connectFailureMessage(connectFailure(c.err)))
    const kinds = Array.from(new Set(cases.map((c) => c.kind)))
    expect(kinds).toEqual(['timeout', 'network', 'rejected', 'http'])
    for (const kind of kinds) {
      const key = keys[cases.findIndex((c) => c.kind === kind)]
      const en = t(key, undefined, 'en')
      const ko = t(key, undefined, 'ko')
      expect(en, `${key} missing in en`).toBeTruthy()
      expect(ko, `${key} missing in ko`).toBeTruthy()
      expect(en).not.toContain('{')
      expect(ko).not.toContain('{')
    }
    // The four sentences are pairwise distinct — the spec separates them.
    const enMessages = kinds.map((k) => t(connectFailureMessage({ kind: k, status: undefined }), undefined, 'en'))
    expect(new Set(enMessages).size).toBe(kinds.length)
  })

  it('the timeout sentence teaches the 25s/network/Tailscale cause', () => {
    const en = t('pair.connect.timeout', undefined, 'en')
    const ko = t('pair.connect.timeout', undefined, 'ko')
    expect(en).toContain('25 seconds')
    expect(en.toLowerCase()).toContain('tailscale')
    expect(ko).toContain('25초')
    expect(ko).toContain('Tailscale')
    expect(en).not.toBe(t('pair.connect.network', undefined, 'en'))
  })
})
