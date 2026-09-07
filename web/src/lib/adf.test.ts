import { describe, expect, test } from 'vitest'
import { renderAdf, renderCommandBody } from './adf'
import type { AdfNode, DetailAttachment } from './types'

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

describe('runtime config comes in as options (GDK-1497)', () => {
  // The renderer no longer reads the web's config store; the caller says
  // where attachment URLs and browse links come from. These tests pin that
  // contract at its three decision points: safeMediaUrl's whitelist base,
  // the media-fallback link, and absence meaning "no site configured".
  const att = (over: Partial<DetailAttachment>): DetailAttachment => ({
    id: '10',
    filename: 'clip.mp4',
    mime_type: 'video/mp4',
    size: 10,
    media_id: 'm-2',
    media_collection: 'c',
    is_image: false,
    is_video: true,
    cache_status: 'ready',
    created_at: null,
    content_url: '/api/v1/issues/STD-1/attachments/10/content/',
    ...over,
  })

  test('an attachment video renders only when apiBase whitelists its URL', () => {
    const media = { type: 'media', attrs: { id: 'm-2' } } as AdfNode
    expect(renderAdf(doc(media), { attachments: [att({})], apiBase: '/api/v1/issues/' })).toBe(
      '<figure class="adf-media-video"><video src="/api/v1/issues/STD-1/attachments/10/content/"' +
        ' controls preload="metadata" playsinline aria-label="clip.mp4"></video>' +
        '<figcaption>clip.mp4</figcaption></figure>',
    )
    // No apiBase → nothing is whitelisted, so no src exists to emit.
    expect(renderAdf(doc(media), { attachments: [att({})] })).toBe('')
    // A content_url outside the whitelist base is not a src either.
    expect(
      renderAdf(doc(media), {
        attachments: [att({ content_url: 'https://evil.example/x' })],
        apiBase: '/api/v1/issues/',
      }),
    ).toBe('')
  })

  test('a non-media attachment renders as the file chip it always was', () => {
    const media = { type: 'media', attrs: { id: 'm-3' } } as AdfNode
    const file = att({
      id: '11',
      media_id: 'm-3',
      filename: 'notes.txt',
      mime_type: 'text/plain',
      is_image: false,
      is_video: false,
      content_url: '/api/v1/issues/STD-1/attachments/11/content/',
    })
    expect(renderAdf(doc(media), { attachments: [file], apiBase: '/api/v1/issues/' })).toBe(
      '<a class="adf-media" href="/api/v1/issues/STD-1/attachments/11/content/"' +
        ' target="_blank" rel="noopener noreferrer">📎 notes.txt</a>',
    )
  })

  test('an unresolved media with no browseUrl stays plain text (the phone)', () => {
    // browseUrl absent → null, exactly what jiraBrowseUrl returns with no
    // site configured. No anchor, so there is nothing to navigate.
    const html = renderAdf(doc({ type: 'media', attrs: { alt: 'gone.png' } }), {
      issueKey: 'STD-7',
    })
    expect(html).toContain('<span class="adf-media">')
    expect(html).toContain('gone.png')
    expect(html).not.toContain('<a')
  })

  test('an unresolved media links out only through the browseUrl callback', () => {
    const html = renderAdf(doc({ type: 'media', attrs: { alt: 'gone.png' } }), {
      issueKey: 'STD-7',
      browseUrl: (k) => `https://team.example.net/browse/${k}`,
    })
    expect(html).toContain('href="https://team.example.net/browse/STD-7"')
    // The callback returning null is the same as no site configured.
    expect(
      renderAdf(doc({ type: 'media', attrs: { alt: 'gone.png' } }), {
        issueKey: 'STD-7',
        browseUrl: () => null,
      }),
    ).not.toContain('<a')
  })
})
