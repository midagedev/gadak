// GDK-877: the share payload builder is the one door to a share path, and
// a pairing offer (DESIGN.md §5) must never get through it. The offer
// strings come from the same golden vectors offer.test.ts reads, so "an
// offer" here means exactly what the decoder means.

import { describe, it, expect, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  buildSharePayload,
  clipboardText,
  looksLikeOffer,
  shareIssue,
  ShareRefused,
  type SharePayload,
} from './share'

interface Vectors {
  valid: { name: string; offer: string }[]
}
const vectors: Vectors = JSON.parse(
  readFileSync(
    fileURLToPath(new URL('../../../internal/pairing/testdata/offer-vectors.json', import.meta.url)),
    'utf8',
  ),
)
const offers = vectors.valid.map((v) => v.offer)

describe('buildSharePayload', () => {
  it('shares key + summary, and the origin page when the row has an absolute one', () => {
    const p = buildSharePayload({
      issue_key: 'NMA-12',
      summary: 'Digest duplicates after retry',
      url: 'https://example.atlassian.net/browse/NMA-12',
    })
    expect(p).toEqual({
      title: 'NMA-12',
      text: 'NMA-12 Digest duplicates after retry',
      url: 'https://example.atlassian.net/browse/NMA-12',
    })
    expect(clipboardText(p)).toBe(
      'NMA-12 Digest duplicates after retry\nhttps://example.atlassian.net/browse/NMA-12',
    )
  })

  it('omits the url for the built-in tracker (relative /browse/KEY) and for rows without one', () => {
    expect(buildSharePayload({ issue_key: 'GDK-1', summary: 'x', url: '/browse/GDK-1' }).url).toBeUndefined()
    expect(buildSharePayload({ issue_key: 'GDK-1', summary: 'x', url: null }).url).toBeUndefined()
    expect(buildSharePayload({ issue_key: 'GDK-1', summary: 'x' })).toEqual({ title: 'GDK-1', text: 'GDK-1 x' })
    expect(clipboardText(buildSharePayload({ issue_key: 'GDK-1', summary: '' }))).toBe('GDK-1')
  })

  it('refuses a subject that is not an issue key', () => {
    expect(() => buildSharePayload({ issue_key: 'not a key', summary: 'x' })).toThrow(ShareRefused)
    expect(() => buildSharePayload({ issue_key: '', summary: 'x' })).toThrow(ShareRefused)
  })

  it('refuses a subject carrying offer fields, whatever else it has', () => {
    for (const extra of [
      { token: 'abc' },
      { tokens: [{ scope: 'serve', token: 'abc' }] },
      { endpoint: 'https://home.example.ts.net' },
      { expires_at: '2026-01-01T00:00:00Z' },
    ]) {
      const subject = { issue_key: 'GDK-1', summary: 'x', ...extra }
      expect(() => buildSharePayload(subject as never), JSON.stringify(Object.keys(extra))).toThrow(ShareRefused)
    }
  })

  it('refuses every golden offer, whole or embedded, in any string field', () => {
    expect(offers.length).toBeGreaterThan(0)
    for (const offer of offers) {
      expect(looksLikeOffer(offer)).toBe(true)
      expect(() => buildSharePayload({ issue_key: 'GDK-1', summary: offer })).toThrow(ShareRefused)
      expect(() => buildSharePayload({ issue_key: 'GDK-1', summary: `pair with ${offer} today` })).toThrow(
        ShareRefused,
      )
      expect(() => buildSharePayload({ issue_key: 'GDK-1', summary: 'x', url: offer })).toThrow(ShareRefused)
      expect(() => buildSharePayload({ issue_key: 'GDK-1', summary: 'x', url: `gadak://pair?o=${offer}` })).toThrow(
        ShareRefused,
      )
    }
  })

  it('refuses a non-http url scheme even when it is not an offer', () => {
    expect(() => buildSharePayload({ issue_key: 'GDK-1', summary: 'x', url: 'gadak://issue/GDK-1' })).toThrow(
      ShareRefused,
    )
  })

  it('the refusal never quotes the input', () => {
    try {
      buildSharePayload({ issue_key: 'GDK-1', summary: offers[0] })
      expect.unreachable()
    } catch (err) {
      expect((err as Error).message).not.toContain(offers[0].slice(0, 12))
    }
  })

  it('ordinary long words are not mistaken for offers', () => {
    expect(looksLikeOffer('x'.repeat(80))).toBe(false)
    expect(looksLikeOffer('https://example.atlassian.net/browse/NMA-12?focusedCommentId=1234567890')).toBe(false)
  })
})

describe('shareIssue', () => {
  const payload: SharePayload = { title: 'GDK-1', text: 'GDK-1 x', url: 'https://j.example.com/browse/GDK-1' }

  function deps(share?: (d: SharePayload) => Promise<void>, clip = vi.fn(async () => {})) {
    return { share, writeClipboard: clip, onCopied: vi.fn(), onCopyFailed: vi.fn() }
  }

  it('takes the share sheet when the platform has one', async () => {
    const share = vi.fn(async () => {})
    const d = deps(share)
    await expect(shareIssue(payload, d)).resolves.toBe('shared')
    expect(share).toHaveBeenCalledWith(payload)
    expect(d.writeClipboard).not.toHaveBeenCalled()
    expect(d.onCopied).not.toHaveBeenCalled()
  })

  it('a dismissed sheet announces nothing', async () => {
    const abort = Object.assign(new Error('dismissed'), { name: 'AbortError' })
    const d = deps(vi.fn(async () => Promise.reject(abort)))
    await expect(shareIssue(payload, d)).resolves.toBe('cancelled')
    expect(d.writeClipboard).not.toHaveBeenCalled()
    expect(d.onCopied).not.toHaveBeenCalled()
    expect(d.onCopyFailed).not.toHaveBeenCalled()
  })

  it('a sheet that failed falls through to the clipboard', async () => {
    const d = deps(vi.fn(async () => Promise.reject(new TypeError('no permission'))))
    await expect(shareIssue(payload, d)).resolves.toBe('copied')
    expect(d.writeClipboard).toHaveBeenCalledWith(clipboardText(payload))
    expect(d.onCopied).toHaveBeenCalledOnce()
  })

  it('no share API → clipboard, with the copied toast', async () => {
    const d = deps(undefined)
    await expect(shareIssue(payload, d)).resolves.toBe('copied')
    expect(d.writeClipboard).toHaveBeenCalledWith('GDK-1 x\nhttps://j.example.com/browse/GDK-1')
    expect(d.onCopied).toHaveBeenCalledOnce()
  })

  it('a refused clipboard gets its own sentence', async () => {
    const d = deps(undefined, vi.fn(async () => Promise.reject(new Error('denied'))))
    await expect(shareIssue(payload, d)).resolves.toBe('copyFailed')
    expect(d.onCopied).not.toHaveBeenCalled()
    expect(d.onCopyFailed).toHaveBeenCalledOnce()
  })
})
