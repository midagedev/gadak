// Lockstep with internal/pairing: both decoders are asserted against the
// same golden vectors file. If this test cannot find or parse the vectors,
// that is a failure — silently passing on a moved file would unhook the
// lockstep.

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { decodeOffer, OfferError } from './offer'

interface Vectors {
  valid: { name: string; offer: string; want: Record<string, string | number> }[]
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
      expect(got.token).toBe(v.want.token)
      expect(got.expires_at).toBe(v.want.expires_at)
      expect(got.label).toBe(v.want.label)
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
    const secret = Buffer.from(JSON.stringify({ v: 2, token: 'sekret-value' })).toString('base64url')
    try {
      decodeOffer(secret)
    } catch (err) {
      expect((err as Error).message).not.toContain('sekret-value')
      expect((err as Error).message).not.toContain(secret)
    }
  })
})
