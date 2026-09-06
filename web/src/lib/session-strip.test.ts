import { describe, expect, test } from 'vitest'
import { en, ja, ko } from './i18n/catalog'
import { SESSION_GAP_MS, changedSince, relatchBoundary, stripLabel, viewKeys } from './session-strip'
import { KEYS_CAP } from './view-config'
import type { IssueLite } from './types'

/*
 * Session strip unit tests (spec r2-session, Part C). Clause table — the
 * clauses this file owns (C1's server half is internal/store/local_session_test.go
 * + internal/server/session_strip_test.go; C4/C6's rendered half is
 * e2e/session-strip.spec.ts):
 *
 *   C1 an unparseable boundary is no boundary — changedSince block ①
 *   C2 zero changes → null → the strip renders nothing, no empty state —
 *      changedSince block ②
 *   C3 subject is the issues — stripLabel block ③
 *        ① no you/your/당신/님 in any locale's three keys
 *   C5 the keys the click hands to the view are capped at KEYS_CAP —
 *      viewKeys block ① … ③ (changedSince's own list stays uncapped: the
 *      label counts every change, the view caps what it can show)
 *   C6 the count is the pool at bootstrap, order preserved — changedSince
 *      block ③ (pool order in, pool order out — the strip is a snapshot,
 *      not a re-sort)
 *   C7 parsed-date comparison, strict — changedSince block ④ ⑤
 *        ① updated_at == since is NOT after (seen before the boundary)
 *        ② +09:00 and Z spellings compare by time, not by string
 *   C8 en/ko/ja parity — stripLabel block ② + the token-multiset check ④
 *   mine via person-match (id first, then email) — changedSince block ⑥
 *
 * FAIL-first (each block names the mutation it kills):
 *   - changedSince with >= instead of > fails ④①; comparing ISO strings
 *     instead of parsed dates fails ④② ('2026-09-01T01:00:00.000+09:00'
 *     sorts after '2026-09-01T00:00:00.000Z' while being 8h earlier);
 *   - viewKeys without the cap fails ① (501 → 501 instead of 500);
 *   - stripLabel pushing the mine part without the me guard fails ②;
 *   - a missing ko/ja key or placeholder fails ④ via the token multiset.
 */

const SINCE = '2026-09-01T00:00:00.000Z'

function issue(key: string, over: Partial<IssueLite> = {}): IssueLite {
  return {
    issue_key: key,
    updated_at: null,
    assignee_id: null,
    assignee_email: null,
    ...over,
  } as IssueLite
}

describe('changedSince', () => {
  test('keys are the issues updated strictly after the boundary, in pool order; null when none', () => {
    const pool = [
      issue('STD-1', { updated_at: '2026-09-02T00:00:00.000Z' }),
      issue('STD-2', { updated_at: '2026-08-30T00:00:00.000Z' }), // before
      issue('STD-3', { updated_at: '2026-09-06T10:00:00.000Z' }),
      issue('STD-4', { updated_at: null }), // unknowable, not changed
    ]
    const got = changedSince(pool, SINCE, null)
    expect(got?.keys).toEqual(['STD-1', 'STD-3']) // C6 ③ pool order
    expect(got?.mine).toBe(0)
    expect(changedSince([issue('STD-2', { updated_at: '2026-08-31T23:59:59.999Z' })], SINCE, null)).toBeNull() // C2 ②
    expect(changedSince([], SINCE, null)).toBeNull() // empty pool
  })

  test('an unparseable boundary is no boundary', () => {
    expect(changedSince([issue('STD-1', { updated_at: '2026-09-02T00:00:00.000Z' })], '', null)).toBeNull() // C1 ①
    expect(changedSince([issue('STD-1', { updated_at: '2026-09-02T00:00:00.000Z' })], 'not a date', null)).toBeNull()
  })

  test('a change at exactly the boundary was seen before it', () => {
    expect(changedSince([issue('STD-1', { updated_at: SINCE })], SINCE, null)).toBeNull() // C7 ④①
    const after = changedSince([issue('STD-1', { updated_at: '2026-09-01T00:00:00.001Z' })], SINCE, null)
    expect(after?.keys).toEqual(['STD-1'])
  })

  test('offset spellings compare by parsed time, not string order', () => {
    // Same instant as SINCE written with a +09:00 offset: the string sorts
    // after SINCE, the instant does not → not counted.
    expect(changedSince([issue('STD-1', { updated_at: '2026-09-01T09:00:00.000+09:00' })], SINCE, null)).toBeNull()
    // 2026-09-01T01:00 (+09:00) = 2026-08-31T16:00Z — before the boundary.
    expect(changedSince([issue('STD-1', { updated_at: '2026-09-01T01:00:00.000+09:00' })], SINCE, null)).toBeNull()
    // 2026-09-01T10:00 (+09:00) = 2026-09-01T01:00Z — genuinely after.
    const later = changedSince([issue('STD-1', { updated_at: '2026-09-01T10:00:00.000+09:00' })], SINCE, null)
    expect(later?.keys).toEqual(['STD-1'])
  })

  test('mine counts through person-match: id first, then email; null me counts none', () => {
    const pool = [
      issue('STD-1', { updated_at: '2026-09-02T00:00:00.000Z', assignee_id: 'acc-1', assignee_email: null }),
      issue('STD-2', {
        updated_at: '2026-09-02T00:00:00.000Z',
        assignee_id: 'other',
        assignee_email: 'Dana@Example.com',
      }),
      issue('STD-3', { updated_at: '2026-09-02T00:00:00.000Z', assignee_id: null, assignee_email: 'x@example.com' }),
      issue('STD-4', { updated_at: '2026-08-01T00:00:00.000Z', assignee_id: 'acc-1' }), // not changed
    ]
    // By id.
    expect(changedSince(pool, SINCE, { accountId: 'acc-1', email: null })?.mine).toBe(1)
    // By email, case-insensitive, id mismatching.
    expect(changedSince(pool, SINCE, { accountId: 'nope', email: 'dana@example.com' })?.mine).toBe(1)
    // Both axes hit different rows.
    expect(changedSince(pool, SINCE, { accountId: 'acc-1', email: 'dana@example.com' })?.mine).toBe(2)
    // No identity → no mine part.
    expect(changedSince(pool, SINCE, null)?.mine).toBe(0)
  })
})

describe('viewKeys', () => {
  const many = Array.from({ length: KEYS_CAP + 1 }, (_, i) => `STD-${i + 1}`)

  test('caps the list the view receives at KEYS_CAP and says so', () => {
    const got = viewKeys(many)
    expect(got.keys).toHaveLength(KEYS_CAP) // C5 ①
    expect(got.keys[0]).toBe('STD-1') // first-wins order survives the cap
    expect(got.capped).toBe(true)
  })

  test('under the cap: unchanged and not flagged', () => {
    const few = ['STD-1', 'STD-2']
    const got = viewKeys(few)
    expect(got.keys).toEqual(few)
    expect(got.capped).toBe(false)
  })

  test('changedSince stays uncapped — the label counts every change', () => {
    const pool = many.map((k) => issue(k, { updated_at: '2026-09-02T00:00:00.000Z' }))
    expect(changedSince(pool, SINCE, null)?.keys).toHaveLength(KEYS_CAP + 1) // C5 ③
  })
})

describe('stripLabel', () => {
  /** The runtime's t with the en table and {param} interpolation it uses. */
  const tEn = (key: keyof typeof en, params?: Record<string, string | number>): string =>
    (en[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? ''))

  test('order is since · changed · mine; mine only when identified and > 0', () => {
    const d = { keys: ['STD-1', 'STD-2'], mine: 1 }
    expect(stripLabel(d, '2h ago', tEn, null)).toBe('Since last session 2h ago · 2 issues changed')
    expect(stripLabel(d, '2h ago', tEn, { accountId: 'a', email: null })).toBe(
      'Since last session 2h ago · 2 issues changed · 1 of them assigned here',
    )
    // Identified but none of the changes are mine → the mine part stays out.
    expect(stripLabel({ keys: ['STD-1'], mine: 0 }, '2h ago', tEn, { accountId: 'a', email: null })).toBe(
      'Since last session 2h ago · 1 issue changed',
    )
  })

  test('singular has its own key', () => {
    expect(stripLabel({ keys: ['STD-1'], mine: 0 }, '1d ago', tEn, null)).toBe(
      'Since last session 1d ago · 1 issue changed',
    )
  })

  test('the subject stays the issues — no second person in any locale', () => {
    const keys = ['list.sessionSince', 'list.sessionChanged', 'list.sessionChangedOne', 'list.sessionMine'] as const
    for (const key of keys) {
      expect(en[key]).not.toMatch(/\byou\b|\byour\b/i)
      expect(ko[key]).not.toContain('당신')
      expect(ko[key]).not.toContain('님')
    }
  })

  test('en/ko/ja carry the same placeholder multiset', () => {
    const tokenRe = /\{[^{}]+\}/g
    const tokens = (s: string): string[] => [...(s.match(tokenRe) ?? [])].sort()
    const keys = ['list.sessionSince', 'list.sessionChanged', 'list.sessionChangedOne', 'list.sessionMine'] as const
    for (const key of keys) {
      expect(tokens(ko[key]).join('\0'), `ko.${key}`).toBe(tokens(en[key]).join('\0'))
      expect(tokens(ja[key]).join('\0'), `ja.${key}`).toBe(tokens(en[key]).join('\0'))
    }
  })
})

describe('relatchBoundary — a hidden tab comes back to a new session (research F #24)', () => {
  /*
   * FAIL-first: the pre-change component had no re-latch at all — a tab
   * hidden overnight never spoke again while `gadak retro` counted two
   * sessions. The pure rule: hidden longer than the session gap → the
   * boundary is the moment it went hidden; shorter → null; never hidden →
   * null. The gap mirrors internal/retro SessionGap (30m).
   */
  const hidden = Date.parse('2026-09-07T00:00:00.000Z')

  test('the gap is the retro session gap, 30 minutes', () => {
    expect(SESSION_GAP_MS).toBe(30 * 60 * 1000)
  })

  test('longer than the gap → the hidden moment, as ISO', () => {
    expect(relatchBoundary(hidden, hidden + SESSION_GAP_MS + 1)).toBe('2026-09-07T00:00:00.000Z')
  })

  test('at or under the gap → null (same session, nothing to re-say)', () => {
    expect(relatchBoundary(hidden, hidden + SESSION_GAP_MS)).toBeNull()
    expect(relatchBoundary(hidden, hidden + 60_000)).toBeNull()
  })

  test('never hidden, or an unusable stamp → null', () => {
    expect(relatchBoundary(null, hidden)).toBeNull()
    expect(relatchBoundary(Number.NaN, hidden)).toBeNull()
  })
})
