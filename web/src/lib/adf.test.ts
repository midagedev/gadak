import { describe, expect, test } from 'vitest'
import { adfToPlainText, isSimpleAdf, renderAdf, renderCommandBody } from './adf'
import type { AdfNode } from './types'

const para = (...text: string[]): AdfNode => ({
  type: 'paragraph',
  content: text.map((t) => ({ type: 'text', text: t })),
})

const doc = (...content: AdfNode[]): AdfNode => ({
  type: 'doc',
  version: 1,
  content,
})

describe('isSimpleAdf', () => {
  test('null, undefined, and an empty doc are simple', () => {
    expect(isSimpleAdf(null)).toBe(true)
    expect(isSimpleAdf(undefined)).toBe(true)
    expect(isSimpleAdf(doc())).toBe(true)
  })

  test('paragraphs, text, and hardBreaks only are simple (demo.db shape)', () => {
    expect(
      isSimpleAdf(
        doc(
          para('The workspace switcher dropdown shows the plan tier badge.'),
          para('Plan changes should surface within a few seconds.'),
        ),
      ),
    ).toBe(true)
    expect(
      isSimpleAdf(
        doc({
          type: 'paragraph',
          content: [
            { type: 'text', text: 'line one' },
            { type: 'hardBreak' },
            { type: 'text', text: 'line two' },
          ],
        }),
      ),
    ).toBe(true)
  })

  test('marks (strong, link) make the doc not simple', () => {
    expect(
      isSimpleAdf(
        doc({
          type: 'paragraph',
          content: [{ type: 'text', text: 'bold', marks: [{ type: 'strong' }] }],
        }),
      ),
    ).toBe(false)
    expect(
      isSimpleAdf(
        doc({
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'See related',
              marks: [{ type: 'link', attrs: { href: 'https://example.test/browse/NMA-1' } }],
            },
          ],
        }),
      ),
    ).toBe(false)
  })

  test('tables, media, lists, headings, and mentions are not simple', () => {
    expect(
      isSimpleAdf(
        doc({
          type: 'table',
          content: [
            {
              type: 'tableRow',
              content: [{ type: 'tableCell', content: [para('cell')] }],
            },
          ],
        }),
      ),
    ).toBe(false)
    expect(
      isSimpleAdf(
        doc({
          type: 'mediaSingle',
          content: [{ type: 'media', attrs: { id: 'abc', type: 'file' } }],
        }),
      ),
    ).toBe(false)
    expect(
      isSimpleAdf(
        doc({
          type: 'bulletList',
          content: [{ type: 'listItem', content: [para('item')] }],
        }),
      ),
    ).toBe(false)
    expect(isSimpleAdf(doc({ type: 'heading', attrs: { level: 2 }, content: para('Steps').content }))).toBe(
      false,
    )
    expect(
      isSimpleAdf(
        doc({
          type: 'paragraph',
          content: [{ type: 'mention', attrs: { id: 'acc-1', text: '@Dana' } }],
        }),
      ),
    ).toBe(false)
  })
})

describe('adfToPlainText', () => {
  test('null and empty become empty string', () => {
    expect(adfToPlainText(null)).toBe('')
    expect(adfToPlainText(undefined)).toBe('')
    expect(adfToPlainText(doc())).toBe('')
  })

  test('joins paragraphs with a newline (demo.db NMB-1 shape)', () => {
    expect(
      adfToPlainText(
        doc(
          para('Context: Document onboarding checklist in the public reference.'),
          para('Acceptance: behaviour is covered by a test and documented in the reference.'),
        ),
      ),
    ).toBe(
      'Context: Document onboarding checklist in the public reference.\nAcceptance: behaviour is covered by a test and documented in the reference.',
    )
  })

  test('hardBreak becomes a newline inside a paragraph', () => {
    expect(
      adfToPlainText(
        doc({
          type: 'paragraph',
          content: [
            { type: 'text', text: 'it crashes' },
            { type: 'hardBreak' },
            { type: 'text', text: 'every time' },
          ],
        }),
      ),
    ).toBe('it crashes\nevery time')
  })

  test('mention and emoji keep attrs.text (same as internal/adf.PlainText)', () => {
    expect(
      adfToPlainText(
        doc({
          type: 'paragraph',
          content: [
            { type: 'mention', attrs: { id: 'acc-1', text: '@Dana' } },
            { type: 'text', text: ' it crashes' },
          ],
        }),
      ),
    ).toBe('@Dana it crashes')
    expect(
      adfToPlainText(
        doc({
          type: 'paragraph',
          content: [{ type: 'emoji', attrs: { shortName: ':wave:', text: '👋' } }],
        }),
      ),
    ).toBe('👋')
  })

  test('flattens the nested document from internal/adf.PlainText', () => {
    const nested: AdfNode = {
      type: 'doc',
      version: 1,
      content: [
        { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'Steps' }] },
        {
          type: 'bulletList',
          content: [
            {
              type: 'listItem',
              content: [
                {
                  type: 'paragraph',
                  content: [
                    { type: 'text', text: 'open ' },
                    { type: 'text', text: 'the editor', marks: [{ type: 'strong' }] },
                  ],
                },
              ],
            },
            {
              type: 'listItem',
              content: [{ type: 'paragraph', content: [{ type: 'text', text: 'press save' }] }],
            },
          ],
        },
        {
          type: 'paragraph',
          content: [
            { type: 'mention', attrs: { id: 'acc-1', text: '@Dana' } },
            { type: 'text', text: ' it crashes' },
            { type: 'hardBreak' },
            { type: 'text', text: 'every time' },
          ],
        },
        { type: 'codeBlock', content: [{ type: 'text', text: 'panic: nil map' }] },
      ],
    }
    const got = adfToPlainText(nested)
    for (const want of [
      'Steps',
      'open the editor',
      'press save',
      '@Dana it crashes',
      'every time',
      'panic: nil map',
    ]) {
      expect(got, `flattened text missing ${JSON.stringify(want)}\ngot:\n${got}`).toContain(want)
    }
    expect(got).not.toContain('{')
    expect(got.split('\n').length).toBeGreaterThanOrEqual(4)
  })
})

/*
 * GDK-1162: the ▶ markup, and the two ways it is withheld.
 *
 * The renderer decides *whether a button exists*; whether it is live is the
 * container's (AdfContent reads the session table and stamps data-shell). So
 * what belongs here is the narrowness: off unless asked, and never on a block
 * that is not one line — the serve refuses a payload with a newline in it, so
 * a ▶ there would be a button that always fails.
 */
describe('runnable code blocks', () => {
  const code = (text: string, language?: string): AdfNode => ({
    type: 'codeBlock',
    ...(language ? { attrs: { language } } : {}),
    content: [{ type: 'text', text }],
  })

  test('no ▶ unless the caller asked for one', () => {
    expect(renderAdf(doc(code('go vet ./...', 'sh')))).not.toContain('data-run-command')
  })

  test('a one-line block carries the command verbatim in the attribute', () => {
    const html = renderAdf(doc(code('go vet ./...', 'sh')), { commands: true })
    expect(html).toContain('data-run-command="go vet ./..."')
    expect(html).toContain('adf-code-head')
    expect(html).toContain('adf-code-lang">sh<')
  })

  test('a multi-line block is still a code block, with no ▶', () => {
    const html = renderAdf(doc(code('cd web\nnpm run build')), { commands: true })
    expect(html).toContain('adf-code')
    expect(html).not.toContain('data-run-command')
  })

  test('a quote in the command cannot break out of the attribute', () => {
    // The text stays in the value as entities. It is fine for the *characters*
    // onclick=alert(1) to be in the document — they are content; what must
    // never happen is the `"` closing the attribute and turning them into one.
    const html = renderAdf(doc(code(`printf 'a" onclick=alert(1) x="'`)), { commands: true })
    expect(html).toContain(
      'data-run-command="printf &#39;a&quot; onclick=alert(1) x=&quot;&#39;"',
    )
    expect(html, 'a real event-handler attribute').not.toMatch(/\sonclick\s*=\s*["']/)
  })

  test('a markdown body renders its fences as the same cards', () => {
    const html = renderCommandBody('run it:\n\n```sh\ngadak sync\n```\n', { commands: true })
    expect(html).toContain('data-run-command="gadak sync"')
    expect(html).toContain('adf-plain')
    // Prose with no fence keeps the caller's plain branch.
    expect(renderCommandBody('just prose', { commands: true })).toBe('')
  })
})
