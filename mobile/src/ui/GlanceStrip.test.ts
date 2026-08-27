import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  feedAfterRead,
  feedKindLabel,
  glanceRows,
  type FeedItem,
  type FeedResponse,
} from '../lib/domain'

/*
 * GDK-871 — the glance strip, as contracts. This directory's established
 * component-test style is source contracts over the .svelte files with real
 * calls into the pure functions (Issues.test.ts / KeyBar.test.ts); there is
 * no DOM mount harness under mobile/src. The pure half is the load-bearing
 * half: row selection and the post-receipt state must live outside the
 * component so "the strip shrank to N keys" is a function call, not a
 * screenshot.
 */

const here = dirname(fileURLToPath(import.meta.url))
const strip = readFileSync(join(here, 'GlanceStrip.svelte'), 'utf8')
const store = readFileSync(join(here, '..', 'lib', 'store.svelte.ts'), 'utf8')
const issues = readFileSync(join(here, '..', 'screens', 'Issues.svelte'), 'utf8')

function item(over: Partial<FeedItem> & { event_id: string; issue_key: string }): FeedItem {
  return {
    event_type: 'comment_added',
    occurred_at: '2026-08-20T00:00:00.000Z',
    actor_name: 'Jane',
    reasons: ['assignee'],
    read_at: null,
    ...over,
  }
}

function fixture(): FeedResponse {
  return {
    items: [
      item({ event_id: 'cl:4', issue_key: 'STD-4', occurred_at: '2026-08-26T09:00:00.000Z' }),
      item({ event_id: 'cl:1', issue_key: 'STD-1', occurred_at: '2026-08-27T08:00:00.000Z' }),
      item({ event_id: 'cl:5', issue_key: 'STD-5', read_at: '2026-08-27T07:00:00.000Z' }),
      item({ event_id: 'cl:2', issue_key: 'STD-2', occurred_at: '2026-08-27T09:00:00.000Z' }),
      item({ event_id: 'cl:3', issue_key: 'STD-3', occurred_at: '2026-08-25T09:00:00.000Z' }),
    ],
    unread_counts: { all: 4, assignee: 4, reporter: 0, mention: 0 },
  }
}

describe('GDK-871 glanceRows — selection is a pure function', () => {
  it('shows unread only, newest first, capped at three', () => {
    const rows = glanceRows(fixture().items)
    expect(rows.map((r) => r.issue_key)).toEqual(['STD-2', 'STD-1', 'STD-4'])
  })

  it('sorts null stamps last and breaks ties on event_id', () => {
    const rows = glanceRows(
      [
        item({ event_id: 'cl:b', issue_key: 'STD-2', occurred_at: null }),
        item({ event_id: 'cl:9', issue_key: 'STD-3', occurred_at: '2026-08-26T09:00:00.000Z' }),
        item({ event_id: 'cl:a', issue_key: 'STD-1', occurred_at: null }),
        item({ event_id: 'cl:8', issue_key: 'STD-4', occurred_at: '2026-08-25T09:00:00.000Z' }),
      ],
      10,
    )
    expect(rows.map((r) => r.issue_key)).toEqual(['STD-3', 'STD-4', 'STD-1', 'STD-2'])
  })

  it('yields nothing for an all-read feed — the strip-absence basis', () => {
    const all = {
      ...fixture(),
      unread_counts: { all: 0, assignee: 0, reporter: 0, mention: 0 },
    }
    for (const i of all.items) i.read_at = '2026-08-27T10:00:00.000Z'
    expect(glanceRows(all.items)).toEqual([])
  })
})

describe('GDK-871 feedAfterRead — local state follows the receipt', () => {
  it("drops the marked issue's rows and mirrors the server's per-reason decrement", () => {
    const feed: FeedResponse = {
      items: [
        item({ event_id: 'cl:1', issue_key: 'STD-1', reasons: ['assignee'] }),
        item({ event_id: 'cl:2', issue_key: 'STD-1', reasons: ['assignee', 'mention'] }),
        item({ event_id: 'cl:3', issue_key: 'STD-2', reasons: ['reporter'] }),
      ],
      unread_counts: { all: 5, assignee: 3, reporter: 4, mention: 2 },
    }
    const after = feedAfterRead(feed, ['STD-1'])
    expect(after.items.map((i) => i.issue_key)).toEqual(['STD-2'])
    // removed: two unread rows — assignee×2, mention×1, reporter×0
    expect(after.unread_counts).toEqual({ all: 3, assignee: 1, reporter: 4, mention: 1 })
  })

  it('prefers the counts the receipt reply carries', () => {
    const after = feedAfterRead(fixture(), ['STD-1'], { all: 99, assignee: 0, reporter: 0, mention: 0 })
    expect(after.unread_counts).toEqual({ all: 99, assignee: 0, reporter: 0, mention: 0 })
    expect(after.items.map((i) => i.issue_key)).not.toContain('STD-1')
  })

  it("mark-all empties the window and zeroes the counts even when the window was truncated", () => {
    const after = feedAfterRead(fixture(), null)
    expect(after.items).toEqual([])
    expect(after.unread_counts).toEqual({ all: 0, assignee: 0, reporter: 0, mention: 0 })
  })

  it('drops the marked key from the painted list; the next unread rises into view', () => {
    const before = glanceRows(fixture().items).map((r) => r.issue_key)
    const after = glanceRows(feedAfterRead(fixture(), ['STD-2']).items).map((r) => r.issue_key)
    expect(before).toEqual(['STD-2', 'STD-1', 'STD-4'])
    expect(after).toEqual(['STD-1', 'STD-4', 'STD-3'])
  })
})

describe('GDK-871 the strip is wired, not self-erasing', () => {
  it('renders only while something is unread — no empty box', () => {
    expect(strip).toContain("const unread = $derived(app.feed?.unread_counts.all ?? 0)")
    expect(strip).toContain('const shown = $derived(unread > 0)')
    expect(strip).toContain('{#if shown}')
  })

  it('never marks rows read just for being seen', () => {
    expect(strip).not.toContain('$effect')
    expect(strip).not.toContain('visibilitychange')
    // The only read receipts are inside the two tap handlers.
    expect(strip).toContain('onclick={markAll}')
    expect(strip).toContain('onclick={() => open(item)}')
    expect(strip.match(/markGlanceIssueRead\(/g)?.length).toBe(1)
  })

  it('a row tap rides the existing navigation route and marks by issue key', () => {
    expect(strip).toContain('openIssue(item.issue_key)')
    expect(strip).toContain('markGlanceIssueRead(item.issue_key)')
  })

  it('surfaces a refused receipt instead of swallowing it', () => {
    expect(strip.match(/error = errorMessage\(err\)/g)?.length).toBe(2)
    expect(strip).toContain('{#if error}')
  })

  it('takes its words from the desktop catalog', () => {
    expect(strip).toContain("t('feed.unreadCount'")
    expect(strip).toContain("t('feed.markAllRead'")
    expect(feedKindLabel('comment_added')).toBe('New comment')
    expect(feedKindLabel('fields_changed')).toBe('Field change')
    // An unknown type shows itself rather than a blank row.
    expect(feedKindLabel('future_event')).toBe('future_event')
  })

  it('sits on the Issues plate above every branch, scope-independent', () => {
    expect(issues).toContain('<GlanceStrip />')
    // Above the first plate branch, not inside one of them.
    expect(issues.indexOf('<GlanceStrip />')).toBeLessThan(issues.indexOf("bootKind === 'skeleton'"))
    expect(issues.indexOf('<GlanceStrip />')).toBeLessThan(issues.indexOf('{:else if isDocs'))
  })
})

describe('GDK-871 the store owns the polling and the receipts', () => {
  it('rides the existing sync cycle with the phone window, and no timer of its own', () => {
    const syncBody = store.slice(
      store.indexOf('export async function sync'),
      store.indexOf('/* ── glance strip read receipts'),
    )
    expect(syncBody).toContain("'issues/feed/?focus=all&limit=20'")
    // Two interval calls existed before this round (syncTimer, startClock's
    // clock); a third would be a feed-specific poller, which the spec forbids.
    expect(store.match(/setInterval\(/g)?.length).toBe(2)
  })

  it('moves local state only after the receipt answers — the restore path', () => {
    for (const name of ['markGlanceIssueRead', 'markGlanceAllRead']) {
      const start = store.indexOf(`export async function ${name}`)
      const body = store.slice(start, store.indexOf('\n}', start))
      const awaited = body.indexOf('await request')
      const mutated = body.indexOf('app.feed = feedAfterRead')
      expect(awaited).toBeGreaterThanOrEqual(0)
      expect(mutated).toBeGreaterThan(awaited)
    }
  })

  it('marks by issue key and by all — the two bodies the server contract defines', () => {
    expect(store).toContain('body: { issue_keys: [key] }')
    expect(store).toContain('body: { all: true }')
  })

  it('forgets the feed on unpair, like every other mirrored plate', () => {
    const unpairBody = store.slice(store.indexOf('export async function unpair'))
    expect(unpairBody).toContain('app.feed = null')
  })
})
