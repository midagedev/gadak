/*
 * Search.svelte module-block contracts: the input ladder and the join.
 * Pinned here because they are the screen's grammar, mirrored from the web
 * originals — looksLikeKey/looksLikeJql from web/src/lib/omnibox.ts +
 * jql.ts, titleShowsQuery from web/src/lib/search-match.ts matchEvidence.
 * The component instance is not mounted (node environment); only the pure
 * exports are exercised.
 */
import { describe, expect, it, vi } from 'vitest'

const { tauriFetch } = vi.hoisted(() => ({ tauriFetch: vi.fn() }))
vi.mock('@tauri-apps/plugin-http', () => ({ fetch: tauriFetch }))

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import type { QueueRow } from '../lib/api'
import type { QueueRowFull } from '../lib/queue-rows'
import {
  joinSearchKeys,
  localTitleMatches,
  looksLikeJql,
  looksLikeKey,
  titleShowsQuery,
  viewChipPlan,
} from './Search.svelte'

const row = (issue_key: string, summary: string): QueueRow => ({
  issue_key,
  summary,
  status: 'In Progress',
  status_category: 'inprogress',
  priority: null,
  priority_rank: 0,
  assignee: null,
  updated_at: null,
})

describe('looksLikeKey (ladder rung ①)', () => {
  it('accepts an uppercase project key with a numeric tail', () => {
    expect(looksLikeKey('GDK-801')).toBe('GDK-801')
    expect(looksLikeKey('AB-12')).toBe('AB-12')
    expect(looksLikeKey('A-1')).toBe('A-1')
  })

  it('trims surrounding whitespace', () => {
    expect(looksLikeKey('  GDK-801\n')).toBe('GDK-801')
  })

  it('rejects everything that is not the whole input being one key', () => {
    expect(looksLikeKey('gdk-801')).toBeNull() // lowercase project is not the key shape
    expect(looksLikeKey('GDK 801')).toBeNull()
    expect(looksLikeKey('GDK-')).toBeNull()
    expect(looksLikeKey('-801')).toBeNull()
    expect(looksLikeKey('GDK-801 draft')).toBeNull()
    expect(looksLikeKey('801')).toBeNull()
    expect(looksLikeKey('')).toBeNull()
    expect(looksLikeKey('   ')).toBeNull()
  })
})

describe('looksLikeJql (web jql.ts lockstep)', () => {
  it('detects a field <op> value clause', () => {
    expect(looksLikeJql('project = GDK')).toBe(true)
    expect(looksLikeJql('statusCategory = new')).toBe(true)
    expect(looksLikeJql('assignee = currentUser()')).toBe(true)
    expect(looksLikeJql('text ~ webhook')).toBe(true)
    expect(looksLikeJql('priority = High and fixVersion in (v1)')).toBe(true)
  })

  it('detects ORDER BY and a jql= URL/fragment', () => {
    expect(looksLikeJql('status = new ORDER BY updated DESC')).toBe(true)
    expect(looksLikeJql('order by created')).toBe(true)
    expect(looksLikeJql('?jql=status%3Dnew')).toBe(true)
    expect(looksLikeJql('https://jira.example.com/browse/?jql=project%3DGDK')).toBe(true)
  })

  it('does not fire on plain text or a bare issue key', () => {
    expect(looksLikeJql('webhook replay')).toBe(false)
    expect(looksLikeJql('fix the pairing flow')).toBe(false)
    expect(looksLikeJql('GDK-801')).toBe(false)
    expect(looksLikeJql('')).toBe(false)
  })
})

describe('localTitleMatches (ladder rung ②)', () => {
  const pool = [
    row('GDK-1', 'Camera look round'),
    row('GDK-2', 'Pairing flow polish'),
    row('GDK-3', 'camera drift fix'),
  ]

  it('substring-matches case-insensitively over titles', () => {
    expect(localTitleMatches('camera', pool).map((r) => r.issue_key)).toEqual(['GDK-1', 'GDK-3'])
  })

  it('trims the query', () => {
    expect(localTitleMatches('  camera  ', pool)).toHaveLength(2)
  })

  it('keeps pool order (the queue order is the local answer)', () => {
    expect(localTitleMatches('flow', pool).map((r) => r.issue_key)).toEqual(['GDK-2'])
  })

  it('returns [] for a blank query or no match', () => {
    expect(localTitleMatches('', pool)).toEqual([])
    expect(localTitleMatches('   ', pool)).toEqual([])
    expect(localTitleMatches('nonexistent', pool)).toEqual([])
  })
})

describe('joinSearchKeys (ladder rung ③)', () => {
  const pool = [row('GDK-1', 'One'), row('GDK-2', 'Two')]

  it('follows the server key order, not the pool order', () => {
    const got = joinSearchKeys(['GDK-2', 'GDK-1'], pool)
    expect(got.map((h) => h.key)).toEqual(['GDK-2', 'GDK-1'])
    expect(got.every((h) => h.row !== null)).toBe(true)
  })

  it('marks keys outside the pool as row: null (key-only rendering)', () => {
    const got = joinSearchKeys(['GDK-9', 'GDK-1'], pool)
    expect(got[0]).toEqual({ key: 'GDK-9', row: null })
    expect(got[1]?.row?.summary).toBe('One')
  })

  it('handles an empty pool', () => {
    expect(joinSearchKeys(['GDK-1'], [])).toEqual([{ key: 'GDK-1', row: null }])
  })
})

describe('titleShowsQuery (web matchEvidence mirror)', () => {
  it('is true when the whole query is visible in the title', () => {
    expect(titleShowsQuery('webhook replay notes', 'webhook replay')).toBe(true)
    expect(titleShowsQuery('Webhook Replay', 'WEBHOOK REPLAY')).toBe(true)
  })

  it('is false when the title only token-matches, not literal-matches', () => {
    // The web comment's own example: FTS hits each token separately, so a
    // row can be a real hit whose title never shows the query as written.
    expect(
      titleShowsQuery('Write a runbook for replaying failed webhook deliveries', 'webhook replay'),
    ).toBe(false)
  })

  it('is false for a blank query or empty title', () => {
    expect(titleShowsQuery('anything', '  ')).toBe(false)
    expect(titleShowsQuery('', 'query')).toBe(false)
  })
})

/* ── Saved-view chips (A-nav) ──
 * viewChipPlan decides local vs web-only. The axes and semantics mirror
 * web/src/stores/filters.svelte.ts; the "anything else set → web" rule is
 * the fail-safe half — a half-read filter must never answer locally. */
describe('viewChipPlan (saved-view chips)', () => {
  const full = (
    issue_key: string,
    summary: string,
    over: Partial<QueueRowFull> = {},
  ): QueueRowFull => ({ ...row(issue_key, summary), ...over })

  const pool: QueueRowFull[] = [
    full('GDK-1', 'New thing', { status: 'Open', status_category: 'new', assignee_id: null, assignee_email: null }),
    full('WEB-2', 'Wired up', { assignee_id: 'acc:5', assignee_email: 'dev@home.lan' }),
    full('WEB-3', 'Done elsewhere', { status_category: 'done', assignee_id: 'acc:7', assignee_email: 'sev@Home.Lan' }),
    full('GDK-4', 'Assigned here', { assignee_id: 'acc:5', assignee_email: null }),
  ]

  const keys = (plan: ReturnType<typeof viewChipPlan>): string[] | null =>
    plan.kind === 'local' ? plan.rows.map((r) => r.issue_key) : null

  it('applies status_category over the folded category axis (aliases included)', () => {
    const plan = viewChipPlan({ filters: { status_category: ['new'] } }, pool)
    expect(keys(plan)).toEqual(['GDK-1'])
    // 'todo' is the same axis value as 'new' (web effectiveCategory fold).
    expect(keys(viewChipPlan({ filters: { status_category: ['todo'] } }, pool))).toEqual(['GDK-1'])
  })

  it('subtracts via the _not twin', () => {
    const plan = viewChipPlan(
      { filters: { jira_project: ['GDK', 'WEB'], jira_project_not: ['WEB'] } },
      pool,
    )
    expect(keys(plan)).toEqual(['GDK-1', 'GDK-4'])
  })

  it('keys projects on the issue_key prefix', () => {
    expect(keys(viewChipPlan({ filters: { jira_project: ['WEB'] } }, pool))).toEqual(['WEB-2', 'WEB-3'])
    expect(keys(viewChipPlan({ filters: { status_category_not: ['done'], jira_project: ['WEB'] } }, pool))).toEqual(['WEB-2'])
  })

  it('matches people on account id or email identity — never a display name', () => {
    expect(keys(viewChipPlan({ filters: { assignee_email: ['acc:5'] } }, pool))).toEqual(['WEB-2', 'GDK-4'])
    // Case-insensitive only when both sides are emails (web sameIdentity).
    expect(keys(viewChipPlan({ filters: { assignee_email: ['sev@home.lan'] } }, pool))).toEqual(['WEB-3'])
    expect(keys(viewChipPlan({ filters: { assignee_email: ['acc:7'] } }, pool))).toEqual(['WEB-3'])
    expect(keys(viewChipPlan({ filters: { assignee_email: ['Dev'] } }, pool))).toEqual([])
    expect(keys(viewChipPlan({ filters: { assignee_email_not: ['acc:5'] } }, pool))).toEqual(['GDK-1', 'WEB-3'])
  })

  it('drops assigned rows on the unassigned flag', () => {
    expect(keys(viewChipPlan({ filters: { unassigned: true } }, pool))).toEqual(['GDK-1'])
  })

  it('matches keys case-insensitively after trimming', () => {
    expect(keys(viewChipPlan({ filters: { keys: [' gdk-4 '] } }, pool))).toEqual(['GDK-4'])
  })

  it('is web-only when any axis outside the interpreted set is set', () => {
    // The axes the phone cannot key on ids for — priority display names,
    // status display names, labels, dates, text, custom fields record.
    const uninterpretable = [
      { priority: ['High'] },
      { status: ['In Progress'] },
      { labels: ['infra'] },
      { updated_after: '2026-01-01' },
      { q: 'camera' },
      { fields: { customfield_1: ['x'] } },
    ]
    for (const filters of uninterpretable) {
      expect(viewChipPlan({ filters: { ...filters, status_category: ['new'] } }, pool)).toEqual({ kind: 'web' })
    }
  })

  it('stays local over unset extras (empty arrays, false flags, empty strings)', () => {
    const plan = viewChipPlan(
      { filters: { status_category: ['new'], labels: [], priority: [], unassigned: false, q: '' } },
      pool,
    )
    expect(keys(plan)).toEqual(['GDK-1'])
  })

  it('is web-only for a malformed or missing config', () => {
    expect(viewChipPlan(null, pool)).toEqual({ kind: 'web' })
    expect(viewChipPlan('not an object', pool)).toEqual({ kind: 'web' })
    expect(viewChipPlan({ display: { columns: [] } }, pool)).toEqual({ kind: 'web' })
    expect(viewChipPlan({ filters: null }, pool)).toEqual({ kind: 'web' })
  })

  it('answers the local pool honestly — done rows are outside it, so such views match fewer', () => {
    // A "done + WEB" view over the open pool: WEB-3 exists in the pool
    // here, so it matches; the point is the filter runs, not that every
    // saved row is present.
    const plan = viewChipPlan({ filters: { status_category: ['done'], jira_project: ['WEB'] } }, pool)
    expect(keys(plan)).toEqual(['WEB-3'])
  })
})
