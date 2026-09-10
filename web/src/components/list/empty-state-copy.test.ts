/*
 * GDK-1091 A-6: an empty state's hint says what to do next — it does not say
 * the title again.
 *
 * The 0-hit search screen read
 *
 *   No issues match
 *   No issues match this search.
 *
 * Two lines that are one sentence. The hint is the only line with room for a
 * next move, and it was spending itself on a restatement, so the screen told a
 * reader who could already see the empty list nothing they did not know.
 *
 * The gate is over the pairs the app actually draws, not over a hand list:
 * every `<EmptyState>` in the source with both a `title={t('…')}` and a
 * `hint={t('…')}` is looked up in the catalog and compared. A new empty state
 * is covered the moment it is written.
 *
 * The rule (UX_PRINCIPLES §"Two totals, one sentence"): the hint must not
 * restate the title — no containment either way, and no near-total word
 * overlap. It may repeat a word; it may not repeat the sentence.
 *
 * FAIL-first output on the strings before this round is in the round report:
 *   list.noMatchTitle "No issues match"
 *   list.noMatchQueryHint "No issues match this search."
 */
import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

import { messages } from '../../lib/i18n/catalog'

/** The screens that draw settled empty states from the catalog. */
const SOURCES = ['../list/ListView.svelte', '../docs/DocsView.svelte'] as const

/** `<EmptyState … />` — non-greedy to the closing `/>`, which every call in
 *  these files uses (a childless component). */
const EMPTY_STATE = /<EmptyState\b[\s\S]*?\/>/g

/** Every `t('key')` inside a `title=`/`hint=` attribute of one block. A hint
 *  built from two keys (`${t(a)} ${t(b)}`) contributes both. */
function keysOf(block: string, attr: 'title' | 'hint'): string[] {
  const m = new RegExp(`${attr}=\\{([\\s\\S]*?)\\}\\n`).exec(block)
  if (!m) return []
  return [...m[1].matchAll(/t\('([^']+)'/g)].map((x) => x[1])
}

function en(key: string): string {
  const row = (messages as Record<string, Record<string, string>>)[key]
  if (!row) throw new Error(`no catalog entry for ${key}`)
  return row.en
}

/** Lowercase, drop punctuation and typographic quotes, collapse spaces. */
function normalize(s: string): string {
  return s
    .toLowerCase()
    .replace(/[.,;:!?“”"‘’'—–-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

/** Share of the shorter line's words that the longer line also has. */
function overlap(a: string, b: string): number {
  const wa = new Set(normalize(a).split(' '))
  const wb = new Set(normalize(b).split(' '))
  const [small, big] = wa.size <= wb.size ? [wa, wb] : [wb, wa]
  let hit = 0
  for (const w of small) if (big.has(w)) hit += 1
  return small.size === 0 ? 0 : hit / small.size
}

/** The pairs, discovered from the source. */
const PAIRS: { where: string; title: string; hint: string }[] = []
for (const rel of SOURCES) {
  const src = readFileSync(new URL(rel, import.meta.url), 'utf8')
  for (const block of src.match(EMPTY_STATE) ?? []) {
    const titles = keysOf(block, 'title')
    const hints = keysOf(block, 'hint')
    if (titles.length !== 1 || hints.length === 0) continue
    PAIRS.push({
      where: `${rel.split('/').pop()} ${titles[0]}`,
      title: en(titles[0]),
      hint: hints.map(en).join(' '),
    })
  }
}

describe('GDK-1091 A-6: an empty-state hint is not the title again', () => {
  test('the scan found the pairs it is meant to gate', () => {
    // A regex that silently matched nothing would make every assertion below
    // vacuous — the failure mode this gate is most exposed to.
    expect(PAIRS.length, `pairs found: ${JSON.stringify(PAIRS, null, 2)}`).toBeGreaterThanOrEqual(4)
  })

  test.each(PAIRS.map((p) => [p.where, p] as const))('%s', (_where, pair) => {
    const t = normalize(pair.title)
    const h = normalize(pair.hint)
    expect(
      h.includes(t) || t.includes(h) ? `"${pair.title}" ⊃⊂ "${pair.hint}"` : '(distinct)',
      `the hint restates the title — it is the only line with room for a next move`,
    ).toBe('(distinct)')
    expect(
      overlap(pair.title, pair.hint),
      `the hint is a near-copy of the title: "${pair.title}" / "${pair.hint}"`,
    ).toBeLessThan(0.8)
  })
})
