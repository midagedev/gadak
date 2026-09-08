/**
 * The tagline per locale, and the one sentence under it.
 *
 * Plain JavaScript, not TypeScript, for one reason: `tools/brand/render.mjs`
 * bakes these strings into the pixels of `docs/media/og.<lang>.png` and runs
 * as plain Node. It can import this file; it cannot import `i18n.ts`. So the
 * copy that appears in the share card lives here and `i18n.ts` reads it back,
 * rather than the two carrying near-identical sentences that drift apart —
 * which is exactly how `og.png` came to ship an English tagline to every
 * Japanese and Korean reader for as long as it did (GDK-1501).
 *
 * `proof` is the row of three claims along the bottom of the card. It exists
 * only for `en` on purpose: nobody has authored the Korean or Japanese
 * wording, and the renderer omits the row rather than setting an English
 * strip under a Japanese headline. Add a `proof` array to a locale and the
 * row appears there on the next `make brand` — no renderer change.
 *
 * @typedef {{ heading: string, body: string, proof?: string[] }} TaglineCopy
 * @type {Record<'en' | 'ko' | 'ja', TaglineCopy>}
 */
export const TAGLINE = {
  en: {
    heading: 'Same Jira. No waiting.',
    body: 'Your team’s Jira — and its Confluence wiki — mirrored into one local SQLite file.',
    proof: ['17 ms reads', 'GROUP BY in one query', 'agents speak SQL to it'],
  },
  ko: {
    heading: '같은 Jira, 기다림 없이.',
    body: '팀의 Jira와 Confluence 위키를 통째로 캐시합니다.',
  },
  ja: {
    heading: '同じJira。待ち時間なし。',
    body: 'チームのJiraとConfluenceのWikiを、まるごとキャッシュ。',
  },
}
