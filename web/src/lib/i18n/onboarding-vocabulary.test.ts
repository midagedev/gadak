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
 * en only: ko and ja are the lead's to write (project CLAUDE.md — Korean and
 * Japanese prose is not delegated). They still carry 미러/ミラー and are
 * listed in this round's report.
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
    const en = (messages as Record<string, Record<string, string>>)['freshness.label'].en
    expect(MIRROR_WORD.test(en), 'GDK-1323 marks the freshness wording a separate matter').toBe(
      true,
    )
  })
})
