// Lockstep with internal/pairing: both decoders are asserted against the
// same golden vectors file. If this test cannot find or parse the vectors,
// that is a failure — silently passing on a moved file would unhook the
// lockstep.

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { decodeOffer, OfferError, serveTokenOf, terminalTokenOf } from './offer'

interface Vectors {
  valid: {
    name: string
    offer: string
    want: {
      v: number
      endpoint: string
      token?: string
      expires_at: string
      label: string
      tokens?: { scope: string; token: string }[]
    }
  }[]
  invalid: { name: string; offer: string; error_contains: string }[]
}

const vectorsPath = fileURLToPath(
  new URL('../../../internal/pairing/testdata/offer-vectors.json', import.meta.url),
)
const vectors: Vectors = JSON.parse(readFileSync(vectorsPath, 'utf8'))

describe('decodeOffer (golden vectors)', () => {
  it('loads a non-empty vector file', () => {
    expect(vectors.valid.length).toBeGreaterThan(0)
    expect(vectors.invalid.length).toBeGreaterThan(0)
  })

  for (const v of vectors.valid) {
    it(`valid: ${v.name}`, () => {
      const got = decodeOffer(v.offer)
      expect(got.v).toBe(v.want.v)
      expect(got.endpoint).toBe(v.want.endpoint)
      expect(got.token).toBe(v.want.token ?? '')
      expect(got.expires_at).toBe(v.want.expires_at)
      expect(got.label).toBe(v.want.label)
      // The normalized list: a v1 vector wants a single unscoped entry
      // derived from its token; a v2 vector wants its own entries.
      const wantTokens = Array.isArray(v.want.tokens)
        ? v.want.tokens
        : [{ scope: null, token: v.want.token }]
      expect(got.tokens).toEqual(wantTokens)
    })
  }

  for (const v of vectors.invalid) {
    it(`invalid: ${v.name}`, () => {
      expect(() => decodeOffer(v.offer)).toThrowError(OfferError)
      try {
        decodeOffer(v.offer)
      } catch (err) {
        expect((err as Error).message).toContain(v.error_contains)
      }
    })
  }

  it('tolerates a std-encoded copy with padding (Go parity)', () => {
    const doc = JSON.stringify({ v: 1, endpoint: 'http://127.0.0.1:7899', token: 'std-padding-token' })
    const padded = Buffer.from(doc).toString('base64') // std alphabet, with '='
    const got = decodeOffer(padded)
    expect(got.token).toBe('std-padding-token')
  })

  it('never quotes the payload in an error', () => {
    // v3 is refused by version, so the hostile payload rides a line the
    // decoder must name the problem of without echoing (v2 is a known
    // version since GDK-1498 — the probe climbs like the Go one).
    const secret = Buffer.from(JSON.stringify({ v: 3, token: 'sekret-value' })).toString('base64url')
    try {
      decodeOffer(secret)
    } catch (err) {
      expect((err as Error).message).not.toContain('sekret-value')
      expect((err as Error).message).not.toContain(secret)
    }
  })

  it('normalizes both versions into the token list', () => {
    const v1 = Buffer.from(
      JSON.stringify({ v: 1, endpoint: 'http://127.0.0.1:7899', token: 'v1-only-token' }),
    ).toString('base64url')
    expect(decodeOffer(v1).tokens).toEqual([{ scope: null, token: 'v1-only-token' }])
    const v2 = Buffer.from(
      JSON.stringify({
        v: 2,
        endpoint: 'http://127.0.0.1:7899',
        tokens: [
          { scope: 'terminal', token: 'v2-term' },
          { scope: 'serve', token: 'v2-serve' },
        ],
      }),
    ).toString('base64url')
    const got = decodeOffer(v2)
    expect(got.token).toBe('')
    expect(got.tokens).toEqual([
      { scope: 'terminal', token: 'v2-term' },
      { scope: 'serve', token: 'v2-serve' },
    ]) // payload order preserved — consumers pick by name
  })

  it('serveTokenOf picks the serve entry, falling back to v1; terminalTokenOf only terminal', () => {
    const encode = (doc: unknown) => Buffer.from(JSON.stringify(doc)).toString('base64url')
    const v1 = decodeOffer(encode({ v: 1, endpoint: 'http://127.0.0.1:7899', token: 'v1-tok' }))
    expect(serveTokenOf(v1)).toBe('v1-tok')
    expect(terminalTokenOf(v1)).toBeNull()
    const both = decodeOffer(
      encode({
        v: 2,
        endpoint: 'http://127.0.0.1:7899',
        tokens: [
          { scope: 'serve', token: 'v2-serve' },
          { scope: 'terminal', token: 'v2-term' },
        ],
      }),
    )
    expect(serveTokenOf(both)).toBe('v2-serve')
    expect(terminalTokenOf(both)).toBe('v2-term')
    const termOnly = decodeOffer(
      encode({ v: 2, endpoint: 'http://127.0.0.1:7899', tokens: [{ scope: 'terminal', token: 'v2-term' }] }),
    )
    expect(serveTokenOf(termOnly)).toBeNull()
  })
})
