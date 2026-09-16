/*
 * GDK-1947 recurrence gate: the catalog owns the singular.
 *
 * The phone's landing page read `NMS-124 · 1 comments` because `t()` had no
 * plural rule — a count reached a plural noun and the catalog had only the
 * plural. The singular now lives beside the plural in the entry as two
 * `|`-separated forms and t() chooses from the numeric `n` param
 * (`{n} comment|{n} comments`). This file holds the string-side contract:
 * every English value that counts a noun must carry both forms, and nothing
 * else may. Checkable from the strings alone — no rendering involved.
 *
 * FAIL-first at authoring time, before the catalog was fixed (2026-09-16):
 * 41 keys flagged — 38 plural-only values (from `deploy.prMergedCount` to
 * `sync.tokenExpiring`) plus the 3 hand-made `detail.resume.*One` singulars,
 * which folded into their twins instead of gaining a form of their own.
 */
import { describe, expect, test } from 'vitest'
import { t } from './index'
import { en, ja, ko, messages, type MessageKey } from './catalog'

/**
 * Keys whose `{n}` is followed by a word that is not the counted noun. Each
 * entry states why the value is correct as written. Same contract as
 * ALLOWED_PHONE_ONLY in catalog.test.ts: an entry is a stated reason, and a
 * key that gains a dual form (or dies) is stale and must go.
 */
const NOT_A_NOUN = new Map<string, string>([
  ['list.selectedCount', '"{n} selected" — participle, no noun to inflect'],
  ['list.moreCount', '"+{n} more" — quantifier'],
  ['bulk.resultOk', '"{n} succeeded" — participle'],
  ['bulk.resultFail', '"{n} failed" — participle'],
  ['bulk.resultSkip', '"{n} skipped" — participle'],
  ['detail.epicShowAll', '"Show {n} more" — quantifier'],
  [
    'detail.priorityShare',
    '"{n} of the {total} open issues" — the noun is counted by {total}, not {n}; "1 of the 12 open issues" is correct',
  ],
  ['person.showingOf', '"Showing the {n} most recent of {total}." — adjective'],
  ['retro.aging.more', '"{n} more" — quantifier'],
  ['retro.closed.clipped', '"{n} above the top" — preposition'],
  ['feed.unreadCount', '"{n} unread" — participle'],
  ['browse.resumeHint', '"{n} open" — participle'],
  ['onboarding.selectedCount', '"{n} selected" — participle'],
  ['palette.triageSelected', '"{n} selected" — participle'],
  ['settings.runtimeApiToday', '"{n} today" — adverb'],
  ['settings.runtimeApiWeek', '"{n} in 7 days" — preposition'],
  ['settings.runtimeApiThrottled', '"{n} throttled" — participle'],
  ['onboarding.stepOf', '"Step {n} of 4" — preposition; an ordinal, never a count'],
])

/** `{n}` directly followed by whitespace + an ASCII letter word. */
const COUNTS_A_WORD = /\{n\}\s+[A-Za-z]+/

describe('GDK-1947 the catalog owns the singular', () => {
  test('an en value that counts a noun carries both forms', () => {
    const failures: string[] = []
    for (const key of Object.keys(en) as MessageKey[]) {
      if (en[key].includes('|')) continue
      if (!COUNTS_A_WORD.test(en[key])) continue
      if (NOT_A_NOUN.has(key)) continue
      failures.push(`${key}=${JSON.stringify(en[key])} — add "one|other" forms (or a NOT_A_NOUN reason)`)
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('every allowlist entry still names a key that needs it — no dead entries', () => {
    const stale: string[] = []
    for (const key of NOT_A_NOUN.keys()) {
      // The allowlist is keyed by string (a reason may outlive a rename);
      // look the value up without narrowing the catalog's own key type.
      const value = (en as Record<string, string | undefined>)[key]
      if (value === undefined) {
        stale.push(`${key}: key no longer exists`)
        continue
      }
      if (value.includes('|')) {
        stale.push(`${key}: value is dual-form now — remove the entry`)
        continue
      }
      if (!COUNTS_A_WORD.test(value)) stale.push(`${key}: {n} no longer counts a word — remove the entry`)
    }
    expect(stale, stale.join('\n')).toEqual([])
  })

  test('a dual form is exactly one bar with two non-empty forms that agree on placeholders', () => {
    const tokenRe = /\{[^{}]+\}/g
    const tokens = (s: string): string[] => [...(s.match(tokenRe) ?? [])].sort()
    const failures: string[] = []
    for (const key of Object.keys(en) as MessageKey[]) {
      const value = en[key]
      const bars = [...value.matchAll(/\|/g)]
      if (bars.length === 0) continue
      if (bars.length > 1) {
        failures.push(`${key}: ${bars.length} bars — a dual form is "one|other", exactly one bar`)
        continue
      }
      const [one, other] = value.split('|')
      if (one.trim() === '' || other.trim() === '') {
        failures.push(`${key}=${JSON.stringify(value)} — a form is empty`)
        continue
      }
      if (tokens(one).join('\0') !== tokens(other).join('\0')) {
        failures.push(
          `${key}: forms disagree on placeholders — one=${JSON.stringify(tokens(one))} other=${JSON.stringify(tokens(other))}`,
        )
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('ko and ja never carry the bar — they have no plural rule to feed it', () => {
    // A bar in a no-plural locale is silently dropped (t() always picks the
    // last segment), so it can only ever be an accident, never copy.
    const failures: string[] = []
    for (const key of Object.keys(en) as MessageKey[]) {
      if (ko[key].includes('|')) failures.push(`ko.${key}=${JSON.stringify(ko[key])}`)
      if (ja[key].includes('|')) failures.push(`ja.${key}=${JSON.stringify(ja[key])}`)
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('t() picks the form from the numeric n — one, other, and the degenerate calls', () => {
    // The landing-page defect, closed: the phone's own row reads singular.
    expect(t('list.commentCount', { n: 1 })).toBe('1 comment')
    expect(t('list.commentCount', { n: 3 })).toBe('3 comments')
    // English takes the plural for zero ("0 comments", CLDR 'other').
    expect(t('list.commentCount', { n: 0 })).toBe('0 comments')
    // Other placeholders ride both forms.
    expect(t('list.bodyMatchCount', { n: 1, q: 'latency' })).toBe('1 body match · “latency”')
    expect(t('list.bodyMatchCount', { n: 2, q: 'latency' })).toBe('2 body matches · “latency”')
    // A string n opts out of the choice (the GDK-1560 identity-number hatch)
    // — but never leaks the bar.
    expect(t('list.commentCount', { n: '1' })).toBe('1 comments')
    expect(t('list.commentCount')).toBe('{n} comments')
  })

  test('the folded pairs render what their deleted keys used to', () => {
    // Six *One keys this round deleted (their ko/ja were what their twins
    // render at n=1, byte for byte); the dual form must still say the en.
    expect(t('detail.resume.statusChanges', { n: 1 })).toBe('1 status change')
    expect(t('detail.resume.newComments', { n: 1 })).toBe('1 new comment')
    expect(t('detail.resume.otherChanges', { n: 1 })).toBe('1 other change')
    expect(t('list.sessionChanged', { n: 1 })).toBe('1 issue changed')
    expect(t('settings.scopePageCount', { n: 1 })).toBe('1 page')
    expect(t('sync.tokenExpiring', { n: 1 })).toBe('Token expires in 1 day')
    // viewKeyOne/viewFilterOne survive: their ja is a word order the plural
    // key cannot produce (キー 1件, not 1件のキー), so their call site keeps
    // picking them at n=1. The plural keys still own their en singular.
    expect(t('palette.viewKeys', { n: 1 })).toBe('1 key')
    expect(t('palette.viewFilters', { n: 1 })).toBe('1 filter')
  })

  test('messages that never counted a noun are byte-identical to before', () => {
    // The mechanism must not touch values without a bar. Spot rows across
    // the families: compact units, participles, no-noun counts, and a plain
    // sentence — the substitution path before the plural branch.
    expect(t('time.minute', { n: 5 })).toBe('5m')
    expect(t('list.selectedCount', { n: 1 })).toBe('1 selected')
    expect(t('detail.wipCount', { n: 4 })).toBe('in progress: 4')
    expect(t('list.moreCount', { n: 9 })).toBe('+9 more')
  })
})

// The messages object participates so a future locale joining types.ts's
// LOCALES has to answer this file too: any table derived from messages must
// carry the same bar policy as en/ko/ja do today.
void messages
