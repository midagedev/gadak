/**
 * Web UI demo recording for docs/media/web-demo.{gif,mp4}.
 *
 * Gated by GADAK_MEDIA=1 so the main suite skips it. This is a hero asset: a
 * reader gives it two seconds, so it shows one thing first — the list collapsing
 * under the keystroke — then an issue (labels, priority, title, reopen),
 * documents, and epics. The in-app Jira pane is a desktop WKWebView; this
 * recording is the browser tab against demo.db, not a reconstruction of that.
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

const isMedia = !!process.env.GADAK_MEDIA
const ISSUE = 'NMA-123'

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
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('search, issue, documents, epics', async ({ page, request }) => {
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
    const history: DemoPage[] = []
    for (const p of latin.slice(1)) {
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
        'gadak:recent',
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
    await beat(page, 900)

    // ── The pitch: type, and hundreds of issues become a handful ──────────
    const input = searchInput(page)
    await input.click()
    await beat(page, 250)
    await input.pressSequentially('pagination', { delay: 140 })
    await expect(page.locator('mark').first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 1500)
    await input.fill('')
    await beat(page, 400)

    // ── An issue — title, priority, labels, reopen (the 0.12 verbs) ───────
    await input.pressSequentially(ISSUE, { delay: 70 })
    const row = page.locator(
      `[data-testid="issue-list-scroller"] [data-issue-key="${ISSUE}"]`,
    )
    await expect(row).toBeVisible()
    await beat(page, 400)
    await row.locator('span.flex-1').first().click()
    const issuePanel = page.getByTestId('issue-detail-panel')
    await expect(issuePanel).toBeVisible()
    await expect(issuePanel.getByTestId('title-editor')).toBeVisible()
    await expect(issuePanel.getByTestId('priority-picker')).toBeVisible()
    await expect(issuePanel.getByTestId('label-editor')).toBeVisible()
    await expect(issuePanel.getByText(/reopened/i).first()).toBeVisible()
    await beat(page, 2000)
    await issuePanel.getByTestId('issue-detail-close').click()
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-detail-open', 'false')
    await input.fill('')
    await beat(page, 350)

    // ── Documents: the mirror is not only Jira ────────────────────────────
    const docsSection = page.getByTestId('docs-section')
    await docsSection.scrollIntoViewIfNeeded()
    await beat(page, 300)
    await docsSection.getByTestId('docs-documents').click()
    const docsView = page.getByTestId('docs-view')
    await expect(docsView).toBeVisible()
    await expect(docsView.getByTestId('doc-row').first()).toBeVisible()
    await beat(page, 1400)

    await docsView.getByTestId('doc-row').first().click()
    const docPanel = page.getByTestId('doc-panel')
    await expect(docPanel).toBeVisible()
    await expect(docPanel.getByTestId('doc-title')).toBeVisible()
    await beat(page, 1800)
    await page.keyboard.press('x')
    await expect(docPanel).toHaveCount(0)
    await beat(page, 300)

    // ── Epics: re-section the same list by the hierarchy Jira hides ───────
    const epics = page.getByRole('button', { name: /Epics/ })
    await epics.scrollIntoViewIfNeeded()
    await epics.click()
    await expect(page.getByTestId('group-header').first()).toBeVisible()
    await beat(page, 1800)

    await expect(listCount(page)).toBeVisible()
    await beat(page, 1400)
  })
})
