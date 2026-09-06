import { describe, expect, test } from 'vitest'
import { en } from './i18n/catalog'
import {
  pickSince,
  resumeDelta,
  resumeLabel,
  SELF_VISIT_MS,
  type ResumeDelta,
} from './resume-card'
import type { DetailResponse } from './types'

/*
 * Resume card unit tests (spec Part C). Clause table — the clauses this file
 * owns (C1's server half is internal/server/detail_visits_test.go; C4/C6's
 * rendered half is e2e/resume-card.spec.ts):
 *
 *   C1 parse of the two visit fields — pickSince block
 *        ① fresh self-visit falls back to previous_visit_at
 *        ② missing last → null; fresh-self with no previous → null
 *   C2 no boundary, no delta — pickSince null / resumeDelta null
 *        ① never-opened issue → null → the component renders no card
 *        ② all-zero delta → null → no card, no empty state
 *   C3 subject is the issue — resumeLabel block
 *        ① en labels contain no "you"/"your"
 *        ② the since-part states the issue's last open, not a person
 *   C5 basis on hover (the label carries the boundary's relative age) —
 *        ① since-part renders the {ago} param
 *        ② label order: since · status · comments · assignee · other
 *   C7 parsed-date comparison, strict — resumeDelta block
 *        ① at == since is NOT counted (seen on that visit)
 *        ② +09:00 and Z spellings of the same instant compare by time, not
 *           by string
 *
 * FAIL-first (each block names the mutation it kills):
 *   - pickSince without the self-visit window (always returning
 *     lastVisitedAt) fails "fresh self-visit" ①;
 *   - resumeDelta with >= instead of > fails the boundary ①;
 *   - resumeDelta comparing ISO strings fails the timezone ②
 *     ('2026-08-10T01:00:00.000+09:00' < '2026-08-10T00:00:00.000Z' as
 *     strings while being 6h later in time).
 */

const NOW = Date.parse('2026-09-06T12:00:00.000Z')

function detailWith(entries: Array<{ at: string | null; field: string }>, comments: Array<string | null>): Pick<DetailResponse, 'history' | 'comments'> {
  return {
    history: entries.map((e) => ({ at: e.at, field: e.field, from: null, to: null, by: null })),
    comments: comments.map((created_at) => ({
      comment_id: 'c',
      author: null,
      author_email: null,
      body: '',
      raw_body: null,
      created_at,
    })),
  }
}

describe('pickSince', () => {
  test('a fresh last visit is this open — the boundary is the previous one', () => {
    const last = '2026-09-06T11:59:30.000Z' // 30s before NOW
    const prev = '2026-08-01T09:00:00.000Z'
    expect(pickSince(last, prev, NOW)).toBe(prev) // ①
    // Clock skew in the other direction is still this open.
    expect(pickSince('2026-09-06T12:01:00.000Z', prev, NOW)).toBe(prev)
  })

  test('an old last visit is already the boundary; missing fields are null', () => {
    const last = '2026-08-20T08:00:00.000Z' // weeks before NOW
    expect(pickSince(last, '2026-07-01T00:00:00.000Z', NOW)).toBe(last) // ② GET-first race order
    expect(pickSince(null, '2026-08-01T00:00:00.000Z', NOW)).toBeNull()
    // First ever open: the only visit is this one — nothing to diff against.
    expect(pickSince('2026-09-06T11:59:59.000Z', null, NOW)).toBeNull()
    // Just outside the window it is a previous visit again.
    expect(pickSince(new Date(NOW - SELF_VISIT_MS - 1).toISOString(), '2026-07-01T00:00:00.000Z', NOW)).toBe(
      new Date(NOW - SELF_VISIT_MS - 1).toISOString(),
    )
  })
})

describe('resumeDelta', () => {
  const since = '2026-08-01T00:00:00.000Z'

  test('counts each field class; null when nothing changed', () => {
    const d = detailWith(
      [
        { at: '2026-08-02T00:00:00.000Z', field: 'status' },
        { at: '2026-08-03T00:00:00.000Z', field: 'status' },
        { at: '2026-08-04T00:00:00.000Z', field: 'assignee' },
        { at: '2026-08-05T00:00:00.000Z', field: 'priority' },
        { at: '2026-08-06T00:00:00.000Z', field: 'attachment' },
      ],
      ['2026-08-07T00:00:00.000Z', '2026-08-08T00:00:00.000Z'],
    )
    expect(resumeDelta(d, since)).toEqual({
      statusChanges: 2,
      comments: 2,
      assigneeChanged: true,
      other: 2,
    })
    expect(resumeDelta(detailWith([], []), since)).toBeNull() // C2 ② no card
    expect(resumeDelta(detailWith([], []), null)).toBeNull() // no boundary
    // Entries before the boundary or without a time do not count.
    expect(
      resumeDelta(detailWith([{ at: '2026-07-31T23:59:59.999Z', field: 'status' }, { at: null, field: 'status' }], [null, '2026-07-01T00:00:00.000Z']), since),
    ).toBeNull()
  })

  test('a change at exactly the boundary was seen on that visit', () => {
    const d = detailWith([{ at: since, field: 'status' }], [since])
    expect(resumeDelta(d, since)).toBeNull() // ① strict >
    const after = detailWith([{ at: '2026-08-01T00:00:00.001Z', field: 'status' }], [])
    expect(resumeDelta(after, since)).toEqual({ statusChanges: 1, comments: 0, assigneeChanged: false, other: 0 })
  })

  test('offset spellings compare by parsed time, not string order', () => {
    // Same instant as since, written with a +09:00 offset: as strings
    // '2026-08-01T09...' > '2026-08-01T00...', as time it is equal → not counted.
    const sameInstant = '2026-08-01T09:00:00.000+09:00'
    expect(resumeDelta(detailWith([{ at: sameInstant, field: 'status' }], []), since)).toBeNull()
    // 2026-08-01T01:00 (+09:00) = 2026-07-31T16:00Z — before the boundary,
    // even though the string sorts after it.
    const earlierInTime = '2026-08-01T01:00:00.000+09:00'
    expect(resumeDelta(detailWith([{ at: earlierInTime, field: 'status' }], []) , since)).toBeNull()
    // 2026-08-01T10:00 (+09:00) = 2026-08-01T01:00Z — genuinely after.
    const laterInTime = '2026-08-01T10:00:00.000+09:00'
    expect(resumeDelta(detailWith([{ at: laterInTime, field: 'status' }], []), since)?.statusChanges).toBe(1)
  })
})

describe('resumeLabel', () => {
  const delta = (over: Partial<ResumeDelta>): ResumeDelta => ({
    statusChanges: 0,
    comments: 0,
    assigneeChanged: false,
    other: 0,
    ...over,
  })

  /** The runtime's t with the en table and {param} interpolation it uses. */
  const tEn = (key: keyof typeof en, params?: Record<string, string | number>): string =>
    (en[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? ''))

  test('zero parts are omitted; order is since · status · comments · assignee · other', () => {
    const all = resumeLabel(delta({ statusChanges: 2, comments: 1, assigneeChanged: true, other: 3 }), '3d ago', tEn)
    expect(all).toBe('Since last opened 3d ago · 2 status changes · 1 new comment · assignee changed · 3 other changes')
    expect(all.indexOf('3d ago')).toBeLessThan(all.indexOf('status change')) // C5 ② order
    expect(resumeLabel(delta({ comments: 1 }), '2h ago', tEn)).toBe('Since last opened 2h ago · 1 new comment')
    expect(resumeLabel(delta({}), '5m ago', tEn)).toBe('Since last opened 5m ago')
  })

  test('singular has its own key; the subject stays the issue', () => {
    expect(resumeLabel(delta({ statusChanges: 1 }), '1d ago', tEn)).toBe('Since last opened 1d ago · 1 status change')
    const line = resumeLabel(delta({ other: 1 }), '1d ago', tEn)
    expect(line.endsWith('1 other change')).toBe(true)
    // C3: no second person anywhere in the rendered parts.
    const whole = resumeLabel(delta({ statusChanges: 1, comments: 1, assigneeChanged: true, other: 1 }), '1d ago', tEn)
    expect(whole).not.toMatch(/\byou\b|\byour\b/i)
    expect(en['detail.resume.sinceOpened']).toBe('Since last opened {ago}')
  })
})
