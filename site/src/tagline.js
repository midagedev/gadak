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
 * Each heading names the job, not a feeling: the 2026-09-08 review round
 * (GDK-1632) found "Same Jira. No waiting." told a reader arriving from a
 * search result or a reshared link nothing about what the tool is, and
 * promised no waiting for operations that still wait on sync. The three
 * headings differ on purpose — the editions are parallel, not translated —
 * and each says what its reader came looking for. The brand line (README
 * line 12, fact ledger §1) sits under the wordmark; it does not have to
 * explain the product.
 *
 * `proof` is the row of three claims along the bottom of the card. It exists
 * only for `en` on purpose: nobody has authored the Korean or Japanese
 * wording, and the renderer omits the row rather than setting an English
 * strip under a Japanese headline. Add a `proof` array to a locale and the
 * row appears there on the next `make brand` — no renderer change. Every
 * claim in it is a ledger fact (docs/project/FACT_LEDGER.md §6, §8).
 *
 * @typedef {{ heading: string, body: string, proof?: string[] }} TaglineCopy
 * @type {Record<'en' | 'ko' | 'ja', TaglineCopy>}
 */
export const TAGLINE = {
  en: {
    heading: 'Query your Jira backlog with SQL.',
    body: 'Selected Jira Cloud projects and Confluence spaces, mirrored into SQLite on your machine.',
    proof: ['GROUP BY in 22 ms', 'search with no network', 'an MCP server for your agent'],
  },
  ko: {
    heading: '묵은 Jira 이슈를 Claude Code로 찾아봅니다.',
    body: '필요한 Jira 프로젝트와 Confluence 스페이스를 골라 캐시합니다.',
  },
  ja: {
    heading: 'Jira の課題を SQL で集計する。',
    body: '指定した範囲の Jira と Confluence をキャッシュし、まとめて検索できます。',
  },
}
