/*
 * Queue row contracts (GDK-801): the render view-model grammar, the sort
 * golden, and the mine/all filter branching.
 *
 *  - Row grammar (ux-report Q1): the status column is the origin's own
 *    display name — `status_text` must never be the category word — and
 *    `status_category` reaches the row as the dot's ink only. Age is the
 *    web relativeTime compact grammar (same buckets, per-locale units).
 *  - Sort golden: queueRows' existing contract (lib/api.ts), pinned here
 *    so the queue chunk cannot drift it: done excluded, priority_rank
 *    ascending, unset rank last, updated_at descending tiebreak, limit.
 *  - Filter: mine keys on assignee_id (account id) — a display name as
 *    the key must match nothing — and narrows BEFORE the sort/slice, so
 *    a mine row ranked below the global top-N is not hidden. 'mine'
 *    without an account id is the full queue, never an empty screen
 *    (#5049).
 */
import { describe, expect, it, vi } from 'vitest'

const { tauriFetch } = vi.hoisted(() => ({ tauriFetch: vi.fn() }))
vi.mock('@tauri-apps/plugin-http', () => ({ fetch: tauriFetch }))

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import { queueRows, type QueueRow } from './api'
import {
  ageCompact,
  defaultMode,
  rowView,
  syncAgeLabel,
  visibleRows,
  type QueueRowFull,
} from './queue-rows'

/** Fixed clock — every bucket boundary below is measured against this. */
const NOW = Date.parse('2026-08-25T12:00:00.000Z')

type RowSpec = Partial<QueueRowFull> & { issue_key: string }

const row = (spec: RowSpec): QueueRowFull => ({
  summary: spec.issue_key,
  status: 'Open',
  status_category: 'new',
  priority: null,
  priority_rank: 3,
  assignee: null,
  assignee_id: null,
  updated_at: '2026-08-25T11:00:00.000Z',
  ...spec,
})

describe('row render contract — status is the origin name, category is ink only', () => {
  it('status_text is the raw status name, even when the category word differs', () => {
    // Discriminating fixture: a Korean-account status name. The scaffold's
    // mistake (categoryLabel text) would render '진행 중' here.
    const r = row({ issue_key: 'GDK-1', status: 'QA 검증 중', status_category: 'inprogress' })
    expect(rowView(r, NOW).status_text).toBe('QA 검증 중')
  })

  it('status_ink maps the category to the dot color, never to text', () => {
    expect(rowView(row({ issue_key: 'GDK-1', status_category: 'new' }), NOW).status_ink).toBe(
      'var(--color-status-new)',
    )
    expect(rowView(row({ issue_key: 'GDK-2', status_category: 'inprogress' }), NOW).status_ink).toBe(
      'var(--color-status-inprogress)',
    )
    expect(rowView(row({ issue_key: 'GDK-3', status_category: 'weird' }), NOW).status_ink).toBe(
      'var(--color-text-muted)',
    )
    // The view-model carries no category-word field at all: whatever the
    // category, the only strings a row can render are the origin's own.
    const view = rowView(row({ issue_key: 'GDK-4', status: 'Reopened', status_category: 'new' }), NOW)
    expect(view.status_text).toBe('Reopened')
  })

  it('priority_ink follows the priorityInk ladder by rank', () => {
    expect(rowView(row({ issue_key: 'GDK-1', priority_rank: 1 }), NOW).priority_ink).toBe(
      'var(--color-status-reopen)',
    )
    expect(rowView(row({ issue_key: 'GDK-2', priority_rank: 0 }), NOW).priority_ink).toBe(
      'var(--color-border-strong)',
    )
  })

  it('age is the web compact grammar against a pinned clock (en)', () => {
    expect(rowView(row({ issue_key: 'GDK-1', updated_at: '2026-08-25T11:59:30.000Z' }), NOW, 'en').age).toBe('just now')
    expect(rowView(row({ issue_key: 'GDK-2', updated_at: '2026-08-25T11:57:00.000Z' }), NOW, 'en').age).toBe('3m')
    expect(rowView(row({ issue_key: 'GDK-3', updated_at: '2026-08-25T10:00:00.000Z' }), NOW, 'en').age).toBe('2h')
    expect(rowView(row({ issue_key: 'GDK-4', updated_at: '2026-08-23T12:00:00.000Z' }), NOW, 'en').age).toBe('2d')
    expect(rowView(row({ issue_key: 'GDK-5', updated_at: null }), NOW, 'en').age).toBe('')
  })
})

describe('age grammar — buckets and locale units (web time.* parity)', () => {
  it('buckets: just-now under a minute, then m/h/d/w/mo/y', () => {
    expect(ageCompact('2026-08-25T11:59:31.000Z', NOW, 'en')).toBe('just now')
    expect(ageCompact('2026-08-25T11:57:00.000Z', NOW, 'en')).toBe('3m')
    expect(ageCompact('2026-08-25T10:00:00.000Z', NOW, 'en')).toBe('2h')
    expect(ageCompact('2026-08-23T12:00:00.000Z', NOW, 'en')).toBe('2d')
    expect(ageCompact('2026-08-16T12:00:00.000Z', NOW, 'en')).toBe('1w')
    expect(ageCompact('2026-07-11T12:00:00.000Z', NOW, 'en')).toBe('1mo')
    expect(ageCompact('2025-07-25T12:00:00.000Z', NOW, 'en')).toBe('1y')
  })

  it('missing or unparseable timestamps render nothing, not a lie', () => {
    expect(ageCompact(null, NOW, 'en')).toBe('')
    expect(ageCompact('', NOW, 'en')).toBe('')
    expect(ageCompact('not-a-date', NOW, 'en')).toBe('')
  })

  it('ko rows use Korean units', () => {
    expect(ageCompact('2026-08-25T11:59:31.000Z', NOW, 'ko')).toBe('방금')
    expect(ageCompact('2026-08-25T11:57:00.000Z', NOW, 'ko')).toBe('3분')
    expect(ageCompact('2026-08-23T12:00:00.000Z', NOW, 'ko')).toBe('2일')
  })

  it('syncAgeLabel composes the freshness sentence, absent when never synced', () => {
    expect(syncAgeLabel(null, NOW, 'en')).toBe('')
    expect(syncAgeLabel('2026-08-25T11:59:31.000Z', NOW, 'en')).toBe('Synced just now')
    expect(syncAgeLabel('2026-08-25T11:48:00.000Z', NOW, 'en')).toBe('Synced 12m ago')
    expect(syncAgeLabel('2026-08-25T11:48:00.000Z', NOW, 'ko')).toBe('12분 전 동기화')
    expect(syncAgeLabel('2026-08-25T11:59:31.000Z', NOW, 'ko')).toBe('방금 동기화')
  })
})

describe('sort golden — the existing queueRows contract, unchanged', () => {
  it('orders by priority_rank ascending, updated_at descending within a rank', () => {
    const rows = [
      row({ issue_key: 'R2', priority_rank: 2, updated_at: '2026-08-24T00:00:00.000Z' }),
      row({ issue_key: 'R1b', priority_rank: 1, updated_at: '2026-08-24T00:00:00.000Z' }),
      row({ issue_key: 'R1a', priority_rank: 1, updated_at: '2026-08-25T00:00:00.000Z' }),
      row({ issue_key: 'R3', priority_rank: 0, updated_at: '2026-08-25T11:00:00.000Z' }),
      row({ issue_key: 'DONE', priority_rank: 1, status_category: 'done' }),
    ]
    expect(queueRows(rows).map((r) => r.issue_key)).toEqual(['R1a', 'R1b', 'R2', 'R3'])
  })

  it('unset rank (0) sorts last, not first', () => {
    const rows = [
      row({ issue_key: 'UNSET', priority_rank: 0 }),
      row({ issue_key: 'LOW', priority_rank: 5 }),
    ]
    expect(queueRows(rows).map((r) => r.issue_key)).toEqual(['LOW', 'UNSET'])
  })

  it('a missing updated_at tiebreaks as oldest', () => {
    const rows = [
      row({ issue_key: 'NO-DATE', priority_rank: 1, updated_at: null }),
      row({ issue_key: 'OLD', priority_rank: 1, updated_at: '2026-08-01T00:00:00.000Z' }),
    ]
    expect(queueRows(rows).map((r) => r.issue_key)).toEqual(['OLD', 'NO-DATE'])
  })

  it('slices to the limit after sorting', () => {
    const rows = [1, 2, 3].map((n) => row({ issue_key: `K${n}`, priority_rank: n }))
    expect(queueRows(rows, 2).map((r) => r.issue_key)).toEqual(['K1', 'K2'])
  })

  it('done rows never appear', () => {
    const rows = [
      row({ issue_key: 'OPEN', status_category: 'inprogress' }),
      row({ issue_key: 'D1', status_category: 'done' }),
    ]
    expect(queueRows(rows).map((r) => r.issue_key)).toEqual(['OPEN'])
  })
})

describe('filter branching — mine keys on assignee_id', () => {
  const MINE = row({ issue_key: 'MINE', priority_rank: 4, assignee_id: 'acc-1', assignee: 'Hchang Kim' })
  const OTHER = row({ issue_key: 'OTHER', priority_rank: 1, assignee_id: 'acc-2', assignee: 'Hchang Kim' })
  const NOBODY = row({ issue_key: 'NOBODY', priority_rank: 2, assignee_id: null })

  it('mine keeps only rows whose assignee_id is me', () => {
    expect(visibleRows([MINE, OTHER, NOBODY], 'mine', 'acc-1').map((r) => r.issue_key)).toEqual(['MINE'])
  })

  it('a display name is not a key: filtering by it matches nothing', () => {
    // Both rows carry assignee 'Hchang Kim'; the id axis is the only match.
    expect(visibleRows([MINE, OTHER], 'mine', 'Hchang Kim')).toEqual([])
  })

  it('mine narrows BEFORE the sort/slice — a mine row below the global top-N is not hidden', () => {
    const five = [1, 2, 3, 4, 5].map((n) => row({ issue_key: `G${n}`, priority_rank: n, assignee_id: n === 5 ? 'acc-1' : 'acc-2' }))
    // All mode with limit 3: G5 (rank 5, mine) is out.
    expect(visibleRows(five, 'all', 'acc-1', 3).map((r) => r.issue_key)).toEqual(['G1', 'G2', 'G3'])
    // Mine mode: G5 is the whole queue — slicing the global top-3 first
    // would have hidden it.
    expect(visibleRows(five, 'mine', 'acc-1', 3).map((r) => r.issue_key)).toEqual(['G5'])
  })

  it('all returns the full open queue', () => {
    expect(visibleRows([MINE, OTHER], 'all', 'acc-1').map((r) => r.issue_key)).toEqual(['OTHER', 'MINE'])
  })

  it("mine without an account id is the full queue — an empty screen is never ours (#5049)", () => {
    expect(visibleRows([MINE, OTHER], 'mine', '').map((r) => r.issue_key)).toEqual(['OTHER', 'MINE'])
  })

  it('rows without assignee_id (older cache shape) are not mine', () => {
    const legacy: QueueRow = { ...NOBODY } // assignee_id stripped by an older writer
    delete (legacy as QueueRowFull).assignee_id
    expect(visibleRows([legacy], 'mine', 'acc-1')).toEqual([])
  })

  it('defaultMode follows me(): connected → mine, standalone/empty → all', () => {
    expect(defaultMode('acc-1')).toBe('mine')
    expect(defaultMode('')).toBe('all')
    expect(defaultMode(null)).toBe('all')
  })
})
