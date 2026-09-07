import { describe, expect, test } from 'vitest'
import { classifyAdfHref, classifyAdfTarget, formatAttachmentSize } from './adf-links'

describe('classifyAdfHref — what a rendered anchor means (GDK-1497)', () => {
  test('a /browse/<KEY> URL is an issue link, with or without query', () => {
    expect(classifyAdfHref('https://team.example.net/browse/STD-42')).toEqual({
      kind: 'issue',
      key: 'STD-42',
    })
    expect(classifyAdfHref('https://team.example.net/browse/NMB-7?src=confmacro')).toEqual({
      kind: 'issue',
      key: 'NMB-7',
    })
  })

  test('any other href copies — it never becomes an in-app navigation', () => {
    // The phone has no opener: an external URL, an attachment chip's
    // same-origin path, an inline card — none of these may classify as an
    // issue, because the issue branch opens a detail screen.
    const notIssues = [
      'https://example.net/readme',
      'https://team.example.net/browse/', // /browse/ with no key
      'https://team.example.net/browse/not-a-key',
      'https://team.example.net/browse/STD-', // dash, no number
      'https://team.example.net/wiki/STD-42', // right key, wrong path
      '/api/v1/issues/STD-1/attachments/9/content/', // attachment chip href
      'javascript:alert(1)',
      '',
      null,
    ]
    for (const href of notIssues) {
      const action = classifyAdfHref(href)
      expect(action?.kind, href ?? 'null').not.toBe('issue')
    }
    expect(classifyAdfHref('https://example.net/readme')).toEqual({
      kind: 'copy',
      href: 'https://example.net/readme',
    })
  })
})

describe('classifyAdfTarget — delegated tap classification', () => {
  // The suite runs in the node environment (vite.config.ts), so there is no
  // DOM to parse. classifyAdfTarget only walks `closest` and reads
  // dataset/getAttribute, which these stubs implement — the assertions are
  // about the walk order, not about HTML parsing.
  interface StubEl {
    closest: (selector: string) => StubEl | null
    dataset: Record<string, string | undefined>
    getAttribute: (name: string) => string | null
  }
  const asEl = (stub: StubEl): HTMLElement => stub as unknown as HTMLElement

  /** An element whose closest() stops at the given chain node. */
  function node(of: { attId?: string; attKind?: string; href?: string | null }): StubEl {
    return {
      dataset: {
        ...(of.attId !== undefined ? { attachmentId: of.attId } : {}),
        ...(of.attKind !== undefined ? { attachmentKind: of.attKind } : {}),
      },
      getAttribute: (name: string) => (name === 'href' ? of.href ?? null : null),
      closest: (selector: string) => {
        if (selector === '[data-attachment-id]') {
          return of.attId !== undefined ? node({ ...of, href: of.href }) : null
        }
        if (selector === 'a[href]') {
          return of.href !== undefined && of.href !== null ? node({ href: of.href }) : null
        }
        return null
      },
    }
  }

  test('an attachment image button wins over any href it might carry', () => {
    // The renderer's own button carries data-attachment-id and no kind: the
    // absent kind means image, so A1's markup keeps working unchanged.
    expect(classifyAdfTarget(asEl(node({ attId: '9' })))).toEqual({ kind: 'image', id: '9' })
  })

  test('a tap on an anchor classifies by its href', () => {
    expect(
      classifyAdfTarget(asEl(node({ href: 'https://team.example.net/browse/STD-42' }))),
    ).toEqual({ kind: 'issue', key: 'STD-42' })
  })

  test('plain text and bare elements are not interactive', () => {
    expect(classifyAdfTarget(asEl(node({})))).toBeNull()
    expect(classifyAdfTarget(null)).toBeNull()
  })

  /*
   * GDK-1503. The renderer emits three attachment shapes and the phone must
   * answer each differently — a video's bytes are fetched on tap, a file chip
   * is copied as an absolute URL, an image opens the viewer. The renderer
   * marks only the image button, so the prime pass stamps data-attachment-kind
   * on the other two; the classifier is where that stamp becomes a decision.
   */
  test('a primed video poster asks to play, not to enlarge', () => {
    expect(classifyAdfTarget(asEl(node({ attId: '12', attKind: 'video' })))).toEqual({
      kind: 'video',
      id: '12',
    })
  })

  test('a primed file chip carries both its id and its href', () => {
    // The href is what the renderer wrote (a relative content path); the id
    // is how the caller finds the attachment row to build an absolute URL.
    expect(
      classifyAdfTarget(
        asEl(
          node({
            attId: '13',
            attKind: 'file',
            href: '/api/v1/issues/STD-1/attachments/13/content/',
          }),
        ),
      ),
    ).toEqual({ kind: 'file', id: '13', href: '/api/v1/issues/STD-1/attachments/13/content/' })
  })

  test('an unknown kind stamp falls back to image, never to navigation', () => {
    expect(classifyAdfTarget(asEl(node({ attId: '14', attKind: 'audio' })))).toEqual({
      kind: 'image',
      id: '14',
    })
  })
})

describe('formatAttachmentSize — the chip and poster hint (GDK-1503)', () => {
  // The same ladder the desktop gallery prints (AttachmentGallery.svelte), so
  // one attachment reads the same on both surfaces.
  test('bytes step through B / KB / MB / GB', () => {
    expect(formatAttachmentSize(512)).toBe('512 B')
    expect(formatAttachmentSize(2048)).toBe('2.0 KB')
    expect(formatAttachmentSize(20 * 1024)).toBe('20 KB')
    expect(formatAttachmentSize(3.5 * 1024 * 1024)).toBe('3.5 MB')
    expect(formatAttachmentSize(4 * 1024 ** 3)).toBe('4.0 GB')
  })

  test('a size the server did not send prints nothing', () => {
    // Older servers and unknown rows send 0; a hint of "0 B" would be a
    // claim about the file. Empty means the poster shows the name alone.
    expect(formatAttachmentSize(0)).toBe('')
    expect(formatAttachmentSize(-1)).toBe('')
    expect(formatAttachmentSize(Number.NaN)).toBe('')
  })
})
