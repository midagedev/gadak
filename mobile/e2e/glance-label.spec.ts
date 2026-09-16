// The activity row's short label does not give way to its long one (GDK-1963).
//
// GlanceStrip renders `[key] [kind] · [detail] [time]`. The kind comes from a
// closed catalogue of seven strings (feed.kindCreated … feed.kindField); the
// detail is whatever a person wrote. Both were `flex: 0 1 auto`, so an
// overflowing row shrank them in proportion and a long comment body clipped
// the label beside it: measured on the 2026-09-16 phone clip, the top row
// read `New …` while its siblings read `New attachment` and `Status change`
// in full. Losing the label is losing what the row is about, which is the
// one thing the row cannot be read without.
//
// The gate is a measurement, not a screenshot: render a row whose detail is
// far too long for the phone's width, then ask the label whether it is
// ellipsized — scrollWidth past clientWidth is the browser's own answer, and
// it needs no font metrics or expected pixel count of ours.
import { expect, test } from './helpers'

/** The phone's width; a row at 402 css px is the whole of the problem. */
const FEED_ROUTE = '/'

/** A detail long enough to overflow any phone row, in either script. */
const LONG_DETAIL =
  '저도 재현됩니다. 업그레이드 직후 화면 첨부합니다. 같은 워크스페이스에서 두 번 더 확인했고 캐시를 지운 뒤에도 같습니다.'

test('a long activity detail does not clip the label beside it', async ({ page }) => {
  // The strip's own shape (domain.ts FeedResponse / FeedItem): `unread_counts`
  // gates whether it renders at all, and `payload.excerpt` is what
  // feedDetail() puts beside the label for a comment. A stub that spells
  // either of those differently renders no strip, and the test would skip
  // rather than measure.
  await page.route('**/issues/feed/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        unread_counts: { all: 2, assignee: 2, reporter: 0, mention: 0, watched: 0 },
        items: [
          {
            event_id: 'e1',
            issue_key: 'NMB-110',
            event_type: 'comment_added',
            actor_name: 'Dana Whitfield',
            reasons: ['assignee'],
            read_at: null,
            occurred_at: new Date().toISOString(),
            payload: { excerpt: LONG_DETAIL },
          },
          {
            event_id: 'e2',
            issue_key: 'NMB-110',
            event_type: 'attachment_added',
            actor_name: 'Dana Whitfield',
            reasons: ['assignee'],
            read_at: null,
            occurred_at: new Date().toISOString(),
            payload: { filename: 'after-upgrade.png' },
          },
        ],
      }),
    })
  })
  await page.goto(FEED_ROUTE, { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()

  // The strip renders only while something is unread, and the stub above is
  // what makes that true. Waiting for it (rather than skipping when it is
  // absent) is deliberate: a spec that quietly passes because the element it
  // measures never appeared is a spec that measures nothing.
  const strip = page.locator('[data-testid="glance-strip"]')
  await strip.waitFor()

  // Every kind label in the strip, with the browser's own ellipsis answer.
  const labels = await page.locator('[data-testid="glance-strip"] .what').evaluateAll((els) =>
    els.map((el) => ({
      text: (el.textContent ?? '').trim(),
      clipped: el.scrollWidth > el.clientWidth + 1,
      client: Math.round(el.clientWidth),
      scroll: Math.round(el.scrollWidth),
    })),
  )
  console.log(`[glance] labels ${JSON.stringify(labels)}`)
  expect(labels.length, 'no activity rows rendered — the route stub did not reach the strip').toBeGreaterThan(0)
  for (const l of labels) {
    expect(
      l.clipped,
      `the activity kind "${l.text}" is ellipsized (${l.scroll}px of text in ${l.client}px) — ` +
        `the row's variable-length detail must give way first, not its label`,
    ).toBe(false)
  }
})
