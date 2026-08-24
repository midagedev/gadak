/*
 * Golden-vector tests for the pairing offer decoder. The vectors are the
 * exact same file the Go suite consumes — internal/pairing/testdata/
 * offer-vectors.json — so a contract change on either side turns both red.
 * Vector semantics are owned by internal/pairing/offer.go (and its
 * offer_test.go); this file only mirrors.
 */
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { OfferError, decodeOffer, isExpired, offerExpiry, type Offer } from './offer'

interface VectorCase {
  name: string
  offer: string
  want?: Offer
  error_contains?: string
  note?: string
}

const vectorsPath = fileURLToPath(
  new URL('../../../internal/pairing/testdata/offer-vectors.json', import.meta.url),
)
const vectors = JSON.parse(readFileSync(vectorsPath, 'utf8')) as {
  valid: VectorCase[]
  invalid: VectorCase[]
}

describe('offer vectors — valid', () => {
  for (const c of vectors.valid) {
    it(c.name, () => {
      const got = decodeOffer(c.offer)
      // Field-wise compare: expect(...).toEqual would also pass on a want
      // object carrying stray undefined keys; the contract is exactly five
      // fields (internal/pairing/offer.go Offer).
      expect({ ...got }).toEqual(c.want)
    })
  }
})

describe('offer vectors — invalid', () => {
  for (const c of vectors.invalid) {
    it(c.name, () => {
      try {
        decodeOffer(c.offer)
        expect.unreachable(`invalid case ${c.name} decoded without error`)
      } catch (err) {
        if (!(err instanceof OfferError)) throw err
        expect(err.message).toContain(c.error_contains)
      }
    })
  }
})

// Mirrors TestOfferVectorsHostileHugeLabel on the Go side: an enormous but
// well-formed payload decodes on both sides. The paste surface bounds input
// length in the UI, not in the decoder.
it('decodes a hostile 100KiB label without throwing', () => {
  const huge = decodeOffer(
    encodeBase64url(
      JSON.stringify({
        v: 1,
        endpoint: 'https://home.example.ts.net',
        token: 't',
        expires_at: '',
        label: 'x'.repeat(100 * 1024),
      }),
    ),
  )
  expect(huge.label.length).toBe(100 * 1024)
})

// Mirrors TestDecodeOfferErrorsNeverEchoPayload: the offer carries the token,
// so an error that quoted the input would leak it into logs.
it('decode errors never echo the payload', () => {
  const secret = 'SUPERSECRET-TOKEN-VALUE'
  const cases = [
    encodeBase64url(JSON.stringify({ v: 7, endpoint: 'https://x', token: secret })),
    encodeBase64url(JSON.stringify({ v: 1, token: secret })),
    secret,
  ]
  for (const c of cases) {
    try {
      decodeOffer(c)
      expect.unreachable('unexpectedly decoded')
    } catch (err) {
      if (!(err instanceof OfferError)) throw err
      expect(err.message).not.toContain(secret)
    }
  }
})

describe('expiry (advisory — the gate judges from the store)', () => {
  it('parses RFC3339 expiry and flags the past', () => {
    const past = decodeOffer(
      encodeBase64url(
        JSON.stringify({
          v: 1,
          endpoint: 'https://x',
          token: 't',
          expires_at: '2020-01-01T00:00:00Z',
          label: '',
        }),
      ),
    )
    expect(isExpired(past, new Date('2026-08-24T00:00:00Z'))).toBe(true)
    const future = { ...past, expires_at: '2027-06-30T09:00:00Z' }
    expect(isExpired(future, new Date('2026-08-24T00:00:00Z'))).toBe(false)
  })
  it('treats an unset or unparseable expiry as unknown, not expired', () => {
    expect(offerExpiry({ v: 1, endpoint: 'https://x', token: 't', expires_at: '', label: '' })).toBeNull()
    expect(
      offerExpiry({ v: 1, endpoint: 'https://x', token: 't', expires_at: 'not a date', label: '' }),
    ).toBeNull()
  })
})

/** Test-side encoder (base64url, unpadded) — mirrors EncodeOffer for ad-hoc cases. */
function encodeBase64url(s: string): string {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
