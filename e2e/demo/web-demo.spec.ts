/**
 * Web UI demo recording for docs/media/web-demo.{gif,mp4}.
 *
 * Gated by SCRY_MEDIA=1 so the main suite skips it. This is a hero asset: a
 * reader gives it two seconds, so it shows one thing first — the list collapsing
 * under the keystroke — and only then documents, spaces, and epics.
 *
 * Four rules learned the hard way:
 *  - Never `waitForLoadState('networkidle')`. The client polls for a delta every
 *    15s, so it returns 15s later and the first half of the GIF is a still frame.
 *  - No DOM caption overlays. App code stays clean; pacing carries the story.
 *  - Beats cost bytes. The GIF budget is 8 MB at 1024 px wide, so every added
 *    beat has to pay for itself — drop one before dropping resolution again.
 *  - Walk the UI the app actually has. The sidebar document tree is gone (r9);
 *    documents are reached through the tabbed Documents view, and the tree is a
 *    toggle inside one space.
 */
import { test, expect, type Page } from '@playwright/test'
import { gotoApp, searchInput } from '../helpers'

const isMedia = !!process.env.SCRY_MEDIA

/** Pause between beats so a human can read the UI. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/** List toolbar count ("N issues") — not the sidebar pool total. */
function listCount(page: Page) {
  return page.locator('span').filter({ hasText: /^\d+ issues?$/ })
}

/** Hangul in a title — the snapshot still carries a few Korean pages, and an
 *  English take should not open one. Screening happens here rather than in the
 *  fixture: the mirror is the source of truth, this only picks what to click. */
function isLatin(title: string): boolean {
  return !/[ᄀ-ᇿ㄰-㆏가-힯]/.test(title)
}

interface DemoPage {
  key: string
  title: string
  space_key: string
  updated_at: string | null
}

test.describe('web UI demo', () => {
  test.skip(!isMedia, 'SCRY_MEDIA=1 only — media pipeline recording')

  test('search, documents, spaces, epics', async ({ page, request }) => {
    // ── Viewing history, so Documents opens on something ──────────────────
    // Viewed is the default tab and it is this browser's own return path, so a
    // first-ever profile lands on the empty state — an honest screen, and a
    // weak opening frame. Eight prior visits are seeded from the mirror's own
    // page index (demo.db keys, nothing invented) to record a returning user;
    // fewer left most of the column empty on screen. Timestamps are recent, so
    // none of them carries an unread mark.
    const index = (await (await request.get('/api/v1/issues/pages/')).json()) as {
      pages: DemoPage[]
    }
    const latin = [...index.pages]
      .sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))
      .filter((p) => isLatin(p.title))
    // The newest page is what the Updated tab will open later — keep it out of
    // the history so that click lands on a row the viewer has not seen yet.
    const history: DemoPage[] = []
    for (const p of latin.slice(1)) {
      // One per space first: the row's "in SPACE" suffix says nothing when
      // every line repeats the same space.
      if (history.some((h) => h.space_key === p.space_key)) continue
      history.push(p)
    }
    for (const p of latin.slice(1)) {
      if (history.length >= 8) break
      if (!history.some((h) => h.key === p.key)) history.push(p)
    }
    await page.addInitScript((keys: string[]) => {
      const base = Date.now()
      localStorage.setItem(
        'scry:recent',
        JSON.stringify(
          keys.map((key, i) => ({
            key,
            viewed_at: new Date(base - (i + 1) * 37 * 60_000).toISOString(),
            kind: 'doc',
          })),
        ),
      )
    }, history.map((p) => p.key))

    // ── Boot ──────────────────────────────────────────────────────────────
    await gotoApp(page)
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    // Short settle for the trailing boot requests. Deliberately not networkidle.
    await beat(page, 900)

    // ── The pitch: type, and hundreds of issues become a handful ──────────
    const input = searchInput(page)
    await input.click()
    await beat(page, 250)
    // Slow enough to read each narrowing step; the count and the <mark>
    // highlights are the whole point of this beat.
    await input.pressSequentially('pagination', { delay: 140 })
    await expect(page.locator('mark').first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 1500)
    await input.fill('')
    await beat(page, 500)

    // ── Documents: the mirror is not only Jira ────────────────────────────
    // One sidebar entry, and the main column becomes the document view. Viewed
    // is where you were; each row is one sentence — author, when, which space.
    const docsSection = page.getByTestId('docs-section')
    await docsSection.scrollIntoViewIfNeeded()
    await beat(page, 300)
    await docsSection.getByTestId('docs-documents').click()
    const docsView = page.getByTestId('docs-view')
    await expect(docsView).toBeVisible()
    await expect(docsView.getByTestId('doc-row').first()).toBeVisible()
    await beat(page, 1500)

    // Updated is everyone else's activity over the same index — the question a
    // wiki answers badly and a local mirror answers instantly.
    await docsView.getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    await expect(docsView.getByTestId('doc-row').first()).toContainText(latin[0].title)
    await beat(page, 1600)

    // A row opens the page beside the list, which keeps its place.
    await docsView.getByTestId('doc-row').first().click()
    const docPanel = page.getByTestId('doc-panel')
    await expect(docPanel).toBeVisible()
    await expect(docPanel.getByTestId('doc-title')).toBeVisible()
    await beat(page, 1900)
    // x is the document panel's close key; the space beat wants the full column.
    await page.keyboard.press('x')
    await expect(docPanel).toHaveCount(0)
    await beat(page, 300)

    // ── One space: flat by default, hierarchy on demand ───────────────────
    await docsSection.getByTestId('docs-spaces').click()
    const spaceRow = docsSection.getByTestId('docs-space').filter({ hasText: 'ENG' })
    await expect(spaceRow).toBeVisible()
    await beat(page, 500)
    await spaceRow.click()
    const spaceView = page.getByTestId('space-docs-view')
    await expect(spaceView.getByTestId('doc-row').first()).toBeVisible()
    await beat(page, 1300)

    // The toggle that gives the hierarchy back, on the same screen rather than
    // in a nav that grows with the wiki.
    await spaceView.getByTestId('space-tree-toggle').click()
    await expect(spaceView.getByTestId('doc-tree-node').first()).toBeVisible()
    await beat(page, 1100)
    // The toggle opens the roots only, so the frame is a list of section names
    // until one of them is opened — a beat that shows a hierarchy has to show
    // two levels of it.
    const nodes = spaceView.getByTestId('doc-tree-node')
    await nodes.filter({ hasText: 'Runbooks' }).getByTestId('doc-tree-toggle').click()
    await expect(nodes.filter({ hasText: 'Runbook — Rate Limit Storm' })).toBeVisible()
    await beat(page, 1600)
    // Collapse the disclosure so the sidebar is back to its resting shape.
    await docsSection.getByTestId('docs-spaces').click()
    await beat(page, 350)

    // ── Epics: re-section the same list by the hierarchy Jira hides ───────
    // One built-in view, not a menu dive: headers carry the epic key and summary.
    const epics = page.getByRole('button', { name: /Epics/ })
    await epics.scrollIntoViewIfNeeded()
    await epics.click()
    const headers = page.getByTestId('group-header')
    await expect(headers.first()).toBeVisible()
    await beat(page, 1600)
    // Down a couple of sections: one epic header is a feature, several in a row
    // are the hierarchy. Jump rather than smooth-scroll — a 1 s animation of a
    // full-width list is the single most expensive thing this GIF can contain.
    await page.getByTestId('issue-list-scroller').evaluate((el) => {
      el.scrollTop = 900
    })
    await beat(page, 1800)

    // ── Rest on the grouped list for a clean loop ─────────────────────────
    await page.getByTestId('issue-list-scroller').evaluate((el) => {
      el.scrollTop = 0
    })
    await expect(listCount(page)).toBeVisible()
    await beat(page, 1400)
  })
})
