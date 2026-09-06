import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, forceLocale, DEMO_ISSUE_COUNT_EN_RE } from './helpers'
import { en } from '../web/src/lib/i18n/en'
import type { FeedItem } from '../web/src/lib/types'

// Where feed-days.png lands: repo scratch by default (gitignored,
// CI-safe), overridable so this round can route the capture to its own
// session scratchpad for the lead's vision review — the same pattern
// session-strip.spec.ts uses.
const SHOT_DIR =
  process.env.FEED_SHOT_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '../scratch')
const SHOT = join(SHOT_DIR, 'feed-days.png')

/*
 * The feed reads as days (2026-09-07): sticky day sections above the
 * GDK-1058 collapse, time-of-day on rows, a who-did-what second line.
 *
 * Clause table — what this file owns:
 *
 *   D1 headers appear one per day, in feed order, with the keys
 *      'today', 'yesterday', local YYYY-MM-DD for older days, 'unknown'
 *      for a row with no timestamp — the unknown row stays where it
 *      fell, it is not hoisted to the end.
 *   D2 labels: Today / Yesterday from the history catalog; a weekday
 *      name 2..6 days back; a date older, with the year only when the
 *      year differs; 'Older' for unknown. Weekday and date expectations
 *      are computed here with the same Intl options the component uses —
 *      never hardcoded, so the file is correct on any run date.
 *   D3 the today header shows the section's item total and its unread
 *      count (2 and 1 for the authored pair that collapses into one row).
 *   D4 the header is sticky (rows scroll under it, not through it).
 *   D5 a row's trailing time is time-of-day — matches a clock shape and
 *      contains no "ago" (relativeTime is gone from the rows).
 *   D6 the comment row's second line is who-did-what: it starts with the
 *      actor's own span and no longer carries the `Dana: ` excerpt
 *      prefix (the actor used to appear only inside that prefix).
 *
 * FAIL-first: against the pre-round tree every case fails at its first
 * assertion — the feed rendered no [data-testid="feed-day"] at all.
 *
 * The feed payload is authored by intercepting GET feed/ and rewriting
 * the items (passthrough-then-rewrite, the session-strip pattern): the
 * demo fixture's own feed rows are tied to fixture timestamps, while
 * these clauses need one item per *day kind*, on today's calendar.
 */

const FEED_ROUTE = '**/api/v1/issues/feed/**'

let nextId = 1

function row(over: Partial<FeedItem> & { issue_key: string }): FeedItem {
  return {
    id: nextId++,
    event_id: `evt-${nextId}`,
    summary: 'summary',
    current_status: 'In Progress',
    event_type: 'comment_added',
    occurred_at: new Date().toISOString(),
    actor_name: 'Dana',
    payload: {},
    reasons: ['assignee'],
    read_at: null,
    ...over,
  }
}

/** Intercept GET feed/ and answer with authored rows — passthrough first
 *  so headers/status come from the real server, then the body is ours. */
async function mockFeedDays(page: Page, items: FeedItem[]): Promise<void> {
  await page.route(FEED_ROUTE, async (route: Route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      json: {
        items,
        unread_counts: { all: 1, assignee: 1, reporter: 0, mention: 0 },
      },
    })
  })
}

test.describe('feed day sections', () => {
  test('the feed reads as days — sticky sections, time-of-day rows, who-did-what', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })

    // Fixtures on today's calendar, so the file is correct on any run
    // date. `today` items clamp to just after local midnight so a run
    // started seconds after midnight does not push them into yesterday.
    const now = new Date()
    const localDay = (offsetDays: number, h: number, m: number) =>
      new Date(now.getFullYear(), now.getMonth(), now.getDate() - offsetDays, h, m)
    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 1)
    const todayAt = (secondsAgo: number) =>
      new Date(Math.max(Date.now() - secondsAgo * 1000, startOfToday.getTime()))

    const threeDaysAgo = localDay(3, 10, 30)
    const nineDaysAgo = localDay(9, 15, 45)
    const lastYear = new Date(now.getFullYear() - 1, 11, 15, 16, 20)

    const items: FeedItem[] = [
      // Today: one issue, two events — they collapse (GDK-1058) into one
      // row under one header; one unread, one read. Different actors, so
      // the collapsed row names no one.
      row({
        issue_key: 'STD-100',
        summary: 'Same-day pair collapses under the day header',
        event_type: 'status_changed',
        occurred_at: todayAt(60).toISOString(),
        actor_name: 'Dana',
        payload: { from: 'To Do', to: 'In Progress' },
        read_at: null,
      }),
      row({
        issue_key: 'STD-100',
        summary: 'Same-day pair collapses under the day header',
        event_type: 'comment_added',
        occurred_at: todayAt(120).toISOString(),
        actor_name: 'Alex',
        payload: { excerpt: 'Following up after the status move' },
        read_at: todayAt(30).toISOString(),
      }),
      // Yesterday: the single comment row D5/D6 read.
      row({
        issue_key: 'STD-201',
        summary: 'Yesterday comment names who did it',
        event_type: 'comment_added',
        occurred_at: localDay(1, 14, 5).toISOString(),
        actor_name: 'Dana',
        payload: { excerpt: 'Reproduced on the staging profile' },
        reasons: ['watched'],
        read_at: todayAt(600).toISOString(),
      }),
      // Three days ago: inside the 2..6 weekday window.
      row({
        issue_key: 'STD-302',
        summary: 'Weekday window row',
        event_type: 'fields_changed',
        occurred_at: threeDaysAgo.toISOString(),
        actor_name: 'Sam',
        payload: { fields: ['labels'] },
        read_at: todayAt(700).toISOString(),
      }),
      // Nine days ago: past the window, this year (unless the run date is
      // early January — the expectation below follows the same rule the
      // component applies, so both sides agree either way).
      row({
        issue_key: 'STD-403',
        summary: 'Date row inside this year',
        event_type: 'status_changed',
        occurred_at: nineDaysAgo.toISOString(),
        actor_name: 'Robin',
        payload: { from: 'In Progress', to: 'Done' },
        read_at: todayAt(800).toISOString(),
      }),
      // Last year: a date with the year.
      row({
        issue_key: 'STD-504',
        summary: 'Date row from last year',
        event_type: 'created',
        occurred_at: lastYear.toISOString(),
        actor_name: 'Robin',
        payload: {},
        read_at: todayAt(900).toISOString(),
      }),
      // No timestamp: an unknown section where it fell — last, because
      // the feed order puts it there, not because it was hoisted.
      row({
        issue_key: 'STD-605',
        summary: 'Row with no timestamp',
        event_type: 'attachment_added',
        occurred_at: null,
        actor_name: 'Alex',
        payload: { filename: 'profile.log' },
        read_at: todayAt(1000).toISOString(),
      }),
    ]

    await mockFeedDays(page, items)

    // Cold-hash boot into the feed screen, the way a shared link arrives
    // (same entry as url-state.spec.ts's gotoParams).
    await forceLocale(page, 'en')
    await page.goto('/#/?feed=all')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    const backToList = page.getByRole('button', { name: en['feed.backToList'] })
    await expect(backToList).toBeVisible()

    // D1 — one header per day, in feed order, unknown where it fell.
    const dayKey = (d: Date) =>
      `${d.getFullYear()}-${`${d.getMonth() + 1}`.padStart(2, '0')}-${`${d.getDate()}`.padStart(2, '0')}`
    const headers = page.getByTestId('feed-day')
    await expect(headers).toHaveCount(6)
    expect(await headers.evaluateAll((els) => els.map((el) => el.getAttribute('data-day')))).toEqual([
      'today',
      'yesterday',
      dayKey(threeDaysAgo),
      dayKey(nineDaysAgo),
      dayKey(lastYear),
      'unknown',
    ])

    // D2 — labels. Weekday and date expectations use the same Intl
    // options as the component, computed on the same local dates.
    const weekdayLabel = (d: Date) =>
      new Intl.DateTimeFormat('en-US', { weekday: 'long' }).format(d)
    const dateLabel = (d: Date) =>
      new Intl.DateTimeFormat('en-US', {
        month: 'short',
        day: 'numeric',
        ...(d.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}),
      }).format(d)
    await expect(headers.nth(0)).toContainText('Today')
    await expect(headers.nth(1)).toContainText('Yesterday')
    await expect(headers.nth(2)).toContainText(weekdayLabel(threeDaysAgo))
    await expect(headers.nth(3)).toContainText(dateLabel(nineDaysAgo))
    await expect(headers.nth(4)).toContainText(dateLabel(lastYear))
    await expect(headers.nth(5)).toContainText('Older')

    // D3 — the today header: 2 events in the section, 1 unread. The
    // count spans are the header's tabular-nums spans (label and rule
    // carry no digits).
    await expect(headers.nth(0).locator('span.tabular-nums')).toHaveText(['2', '1'])

    // D4 — the header is sticky.
    await expect(headers.first()).toHaveCSS('position', 'sticky')

    // D5 — the yesterday row's trailing time is a clock time, not "ago".
    const yesterdayRow = page.locator('[data-day="yesterday"] + button')
    await expect(yesterdayRow).toBeVisible()
    const timeText = await yesterdayRow.locator('span[title]').textContent()
    expect(timeText, `row time was ${JSON.stringify(timeText)}`).toMatch(/^\d{1,2}:\d{2}(\s?[AP]M)?$/)
    expect(timeText).not.toContain('ago')

    // D6 — the comment row's second line: who did what. It starts with
    // the actor's own span (the glyph is an icon, not text) and the
    // removed `Dana: ` excerpt prefix is really gone.
    const secondLine = await yesterdayRow.locator('.mt-1').textContent()
    expect(secondLine?.trim().startsWith('Dana')).toBe(true)
    expect(secondLine).toContain(en['feed.kindComment'])
    expect(secondLine).not.toContain('Dana: ')

    // The capture for the separate vision round — written, never judged here.
    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: SHOT, animations: 'disabled' })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
