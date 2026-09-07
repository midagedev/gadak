import { describe, expect, test } from 'vitest'
import { classifyAdfHref, classifyAdfTarget } from './adf-links'

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
  function node(of: { attId?: string; href?: string | null }): StubEl {
    return {
      dataset: of.attId !== undefined ? { attachmentId: of.attId } : {},
      getAttribute: (name: string) => (name === 'href' ? of.href ?? null : null),
      closest: (selector: string) => {
        if (selector === '[data-attachment-id]') {
          return of.attId !== undefined ? node({ attId: of.attId }) : null
        }
        if (selector === 'a[href]') {
          return of.href !== undefined ? node({ href: of.href }) : null
        }
        return null
      },
    }
  }

  test('an attachment image button wins over any href it might carry', () => {
    expect(classifyAdfTarget(asEl(node({ attId: '9' })))).toEqual({ kind: 'attachment', id: '9' })
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
})
