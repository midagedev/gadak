/*
 * GDK-1323 recurrence gate (audit B-3, epic GDK-1310): one dialog, one word
 * for the local copy.
 *
 * Onboarding walked the user through picking projects "to mirror", reported
 * "Mirrored {n} issues", and the sidebar said "No documents mirrored" — while
 * the surrounding product had moved to calling the local copy a cache. Two
 * vocabularies for one thing inside one flow is the defect; which word wins
 * is settled elsewhere (the user's 2026-09-08 decision: cache says both why
 * it is fast and why it is safe to delete).
 *
 * Scope is the keys the audit named, not every string containing "mirror".
 * The freshness chip is explicitly out — GDK-1323 calls it "별건", a separate
 * matter — and so are `docs/MIRROR.md`, code identifiers and CLI output,
 * which keep the word by the same decision. A gate that swept all of them
 * would be a vocabulary rename wearing a gate's clothes, and renames of that
 * shape are what swallow wire contracts.
 *
 * en only for the onboarding keys: ko and ja are the lead's to write (project
 * CLAUDE.md — Korean and Japanese prose is not delegated). The GDK-1817 block
 * at the bottom is the one place this file does measure them, because that is
 * the one place the lead has since written.
 *
 * FAIL-first, on the strings before this round:
 *   write.projectNotMirrored  "…not in this mirror. Pick a mirrored project…"
 *   onboarding.projectsIntro  "Pick the projects to mirror, or pick none to mirror…"
 *   onboarding.syncDone       "Mirrored {n} issues."
 *   sidebar.docsNoneTitle     "No documents mirrored"
 *   sidebar.docsNotFetchedHint "Sync now to mirror the spaces you chose."
 */
import { describe, expect, test } from 'vitest'

import { messages } from './catalog'

/** The onboarding + first-run empty states the audit listed. */
const ONBOARDING_KEYS = [
  'write.projectNotMirrored',
  'onboarding.projectsIntro',
  'onboarding.syncDone',
  'sidebar.docsNoneTitle',
  'sidebar.docsNotFetchedHint',
] as const

const MIRROR_WORD = /\bmirror(s|ed|ing)?\b/i

describe('GDK-1323 onboarding says cache, not mirror', () => {
  test.each(ONBOARDING_KEYS)('%s (en) carries no mirror vocabulary', (key) => {
    const en = (messages as Record<string, Record<string, string>>)[key].en
    expect(
      MIRROR_WORD.test(en) ? `"${en}"` : '(none)',
      `${key}: one flow, one word for the local copy (GDK-1323)`,
    ).toBe('(none)')
  })

  // The word the flow settled on is actually there — otherwise the rule above
  // is satisfied by deleting the noun and saying nothing.
  test.each(['onboarding.projectsIntro', 'onboarding.syncDone'] as const)(
    '%s (en) names the cache',
    (key) => {
      const en = (messages as Record<string, Record<string, string>>)[key].en
      expect(en.toLowerCase()).toMatch(/cach/)
    },
  )

  // The deliberate exclusion, asserted so that "the freshness chip still says
  // Mirror" stays a decision on the record rather than an oversight someone
  // later tidies away.
  test('the freshness chip is out of scope and still says Mirror', () => {
    const en = (messages as Record<string, Record<string, string>>)['sync.freshLabel'].en
    expect(MIRROR_WORD.test(en), 'GDK-1323 marks the freshness wording a separate matter').toBe(
      true,
    )
  })
})

/*
 * GDK-1817 (audit pass 3, axis 6 A12): the chip's ko and ja, which the block
 * above deliberately left alone and which nothing measured.
 *
 * GDK-1286 moved the Sources tab beside this chip to 캐시 / キャッシュ, so the
 * settings window was showing a person two words for one thing — and the
 * English exclusion above said nothing about it, because the English word
 * stays by the same 2026-09-08 decision. A rule written for one language is
 * not a rule for the other two; this is the half that was missing.
 *
 * FAIL-first, on the strings before this round: sync.freshLabel ko '미러 신선도'
 * / ja 'ミラーの鮮度', and 미러/ミラー in freshTitle, freshLocalTitle,
 * staleTitle and neverTitle — five keys, ten values, all red.
 */
const CHIP_KEYS = [
  'sync.freshLabel',
  'sync.freshTitle',
  'sync.freshLocalTitle',
  'sync.staleTitle',
  'sync.neverTitle',
] as const

/** The word the prose editions dropped: 미러 (ko) and ミラー (ja). */
const MIRROR_LOANWORD = /미러|ミラー/

describe('GDK-1817 the freshness chip says cache in ko and ja', () => {
  test.each(CHIP_KEYS)('%s carries no mirror loanword in ko or ja', (key) => {
    const row = (messages as Record<string, Record<string, string>>)[key]
    const offenders = (['ko', 'ja'] as const)
      .filter((locale) => MIRROR_LOANWORD.test(row[locale]))
      .map((locale) => `${locale}: "${row[locale]}"`)
    expect(
      offenders,
      `${key}: the Sources tab on this same screen says 캐시 / キャッシュ (GDK-1286); ` +
        'one screen, one word for the local copy. English keeps `mirror` by the ' +
        'same 2026-09-08 decision — only ko and ja moved.',
    ).toEqual([])
  })

  // The noun is actually there, so the rule is not satisfied by deleting it.
  test.each(CHIP_KEYS)('%s names the cache in ko and ja', (key) => {
    const row = (messages as Record<string, Record<string, string>>)[key]
    expect(row.ko).toMatch(/캐시/)
    expect(row.ja).toMatch(/キャッシュ/)
  })
})
