import { describe, expect, test } from 'vitest'
import { renderAdf, renderCommandBody } from './adf'
import type { AdfNode } from './types'

const doc = (...content: AdfNode[]): AdfNode => ({
  type: 'doc',
  version: 1,
  content,
})

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

describe('code blocks round-trip (GDK-1178)', () => {
  const doc: AdfNode = {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'before' }] },
      {
        type: 'codeBlock',
        attrs: { language: 'sh' },
        content: [{ type: 'text', text: 'gadak sql "x"' }],
      },
    ],
  }

  // GDK-1385: the editor no longer flattens ADF itself — the server sends
  // description_md and format_loss — so the simple-doc gate and the textarea
  // seed left this module. What stays is that the fence renders as a card.
  test('renders the fenced command as a card', () => {
    expect(renderAdf(doc, { commands: true })).toContain('gadak sql')
  })
})
