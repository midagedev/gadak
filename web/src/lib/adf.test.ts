import { describe, expect, test, vi } from 'vitest'
import { renderAdf, renderCommandBody } from './adf'
import { initLocale } from './i18n'
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

describe('an unresolved media node with no alt (GDK-1505)', () => {
  // Measured on the demo fixture, NMB-110's comment: a media node the mirror
  // could not resolve and that carries no `alt` rendered as the chip
  // "Attachment: Attachment" — the placeholder was being fed in as the name.
  // The prefix earns its place only when a real name follows it.
  const media = (attrs?: Record<string, unknown>): AdfNode =>
    ({ type: 'media', ...(attrs ? { attrs } : {}) }) as AdfNode
  const mediaInline = (attrs?: Record<string, unknown>): AdfNode =>
    ({ type: 'mediaInline', ...(attrs ? { attrs } : {}) }) as AdfNode
  const browse = { issueKey: 'STD-7', browseUrl: (k: string) => `https://team.example.net/b/${k}` }
  const anchor = (body: string) =>
    `<a class="adf-media" href="https://team.example.net/b/STD-7"` +
    ` target="_blank" rel="noopener noreferrer">${body}</a>`

  test('the chip is the bare word, in both node kinds and both link variants', () => {
    expect(renderAdf(doc(media()))).toBe('<span class="adf-media">Attachment</span>')
    expect(renderAdf(doc(media()), browse)).toBe(anchor('Attachment'))
    expect(renderAdf(doc(mediaInline()))).toBe('<span class="adf-media">Attachment</span>')
    // mediaInline sits inside a paragraph's text run and has never linked out;
    // a browseUrl does not change that, only the label it carries.
    expect(renderAdf(doc(mediaInline()), browse)).toBe('<span class="adf-media">Attachment</span>')
  })

  test('an alt that is empty or all whitespace is no name either', () => {
    expect(renderAdf(doc(media({ alt: '' })))).toBe('<span class="adf-media">Attachment</span>')
    expect(renderAdf(doc(media({ alt: '   ' })))).toBe('<span class="adf-media">Attachment</span>')
    expect(renderAdf(doc(mediaInline({ alt: '' })))).toBe(
      '<span class="adf-media">Attachment</span>',
    )
  })

  test('a real name still gets the prefixed label', () => {
    expect(renderAdf(doc(media({ alt: 'gone.png' })))).toBe(
      '<span class="adf-media">Attachment: gone.png</span>',
    )
    expect(renderAdf(doc(media({ alt: 'gone.png' })), browse)).toBe(anchor('Attachment: gone.png'))
    expect(renderAdf(doc(mediaInline({ alt: 'gone.png' })))).toBe(
      '<span class="adf-media">Attachment: gone.png</span>',
    )
  })

  test('the bare word is each locale’s own, not an English one', () => {
    // English hides this defect: the bare word and the file-name placeholder
    // are spelled the same there. ko and ja are where the two arms disagreed.
    // Storage stub follows i18n/locale-detect.test.ts.
    const mem = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        mem.set(k, v)
      },
      removeItem: (k: string) => {
        mem.delete(k)
      },
      clear: () => mem.clear(),
      key: (i: number) => [...mem.keys()][i] ?? null,
      get length() {
        return mem.size
      },
    })
    vi.stubGlobal('navigator', { language: 'en-US' })
    try {
      for (const [loc, word] of [
        ['ko', '첨부 파일'],
        ['ja', '添付ファイル'],
      ] as const) {
        mem.set('gadak_locale', loc)
        initLocale()
        expect(renderAdf(doc(media())), loc).toBe(`<span class="adf-media">${word}</span>`)
        expect(renderAdf(doc(mediaInline())), loc).toBe(`<span class="adf-media">${word}</span>`)
      }
    } finally {
      mem.set('gadak_locale', 'en')
      initLocale()
      vi.unstubAllGlobals()
    }
  })
})

describe('an alt-carrying media node resolves through the filename (GDK-1517)', () => {
  // Live Jira Cloud carries attrs.alt = the attachment's filename on every
  // media node (24/24 across five issues, measured 2026-09-10), and the
  // mirror's attachments have no media id — alt is the only join
  // findAttachment has. The doc below is NMB-110's comment (examples/demo.db,
  // comment jira:10612, Dana Whitfield) extracted verbatim; the attachment
  // list mirrors the server's shape (media_id arrives as '').
  const danaComment: AdfNode = {
    type: 'doc',
    version: 1,
    content: [
      {
        type: 'paragraph',
        content: [
          {
            type: 'text',
            text: 'Reproduced on staging. The tier is cached without the workspace id — screenshot and sketch attached.',
          },
        ],
      },
      {
        type: 'mediaSingle',
        attrs: { layout: 'center' },
        content: [
          {
            type: 'media',
            attrs: { type: 'file', id: 'e1c32652-396f-4be3-bf65-e1094e9a7bc0', alt: 'nimbus-error.png', collection: '' },
          },
        ],
      },
      {
        type: 'mediaSingle',
        attrs: { layout: 'center' },
        content: [
          {
            type: 'media',
            attrs: { type: 'file', id: 'a144bb4a-4740-47c9-ba76-7ea1b5b1942a', alt: 'cache-key-sketch.png', collection: '' },
          },
        ],
      },
    ],
  }
  const image = (id: string, filename: string): DetailAttachment => ({
    id,
    filename,
    mime_type: 'image/png',
    size: 10,
    media_id: '',
    media_collection: '',
    is_image: true,
    is_video: false,
    cache_status: 'ready',
    created_at: null,
    content_url: `/api/v1/issues/STD-1/attachments/${id}/content/`,
  })
  const attachments = [
    image('10000', 'nimbus-error.png'),
    image('10001', 'latency-before-after.png'),
    image('10002', 'cache-key-sketch.png'),
  ]

  test('each media node of the comment renders its image, not a chip', () => {
    const html = renderAdf(danaComment, { attachments, apiBase: '/api/v1/issues/' })
    expect(html.match(/<img /g)?.length ?? 0).toBe(2)
    expect(html).toContain('alt="nimbus-error.png"')
    expect(html).toContain('alt="cache-key-sketch.png"')
    // The unresolved chip/anchor is class="adf-media"; the resolved button is
    // adf-media-image. The closing quote keeps this from matching the button.
    expect(html).not.toContain('class="adf-media"')
  })
})
