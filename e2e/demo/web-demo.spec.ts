/**
 * Web UI demo recording for docs/media/web-demo.{gif,mp4}.
 *
 * Gated by SCRY_MEDIA=1 so the main suite skips it. This is a hero asset: a
 * reader gives it two seconds, so it shows one thing first — the list collapsing
 * under the keystroke — and only then documents, epics, and the palette.
 *
 * Three rules learned the hard way:
 *  - Never `waitForLoadState('networkidle')`. The client polls for a delta every
 *    15s, so it returns 15s later and the first half of the GIF is a still frame.
 *  - No DOM caption overlays. App code stays clean; pacing carries the story.
 *  - Beats cost bytes. The GIF budget is 8 MB at 1024 px wide, so every added
 *    beat has to pay for itself — drop one before dropping resolution again.
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

test.describe('web UI demo', () => {
  test.skip(!isMedia, 'SCRY_MEDIA=1 only — media pipeline recording')

  test('search, documents, epics, palette', async ({ page }) => {
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
    await beat(page, 400)

    // ── Same box, wider net: issues *and* mirrored wiki pages ─────────────
    // Enter runs the server-side search, which answers with a Documents group.
    // This is the beat that says "the mirror is not only Jira".
    await input.pressSequentially('billing', { delay: 140 })
    await input.press('Enter')
    const docRows = page.getByTestId('search-doc-row')
    await expect(docRows.first()).toBeVisible({ timeout: 15_000 })
    await beat(page, 1800)

    // Open one — the panel proves the body is mirrored, the breadcrumb proves
    // scry kept the page's place in the space tree.
    await docRows.first().click()
    const docPanel = page.getByTestId('doc-panel')
    await expect(docPanel).toBeVisible()
    await expect(docPanel.getByTestId('doc-breadcrumb')).toBeVisible()
    await beat(page, 1900)
    await page.keyboard.press('Escape')
    await input.fill('')
    await beat(page, 500)

    // ── Sidebar DOCS tree: a space, then one level under it ───────────────
    const docsSection = page.getByTestId('docs-section')
    await docsSection.scrollIntoViewIfNeeded()
    await beat(page, 300)
    await docsSection.getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
    const nodes = docsSection.getByTestId('doc-tree-node')
    await expect(nodes.first()).toBeVisible()
    // The sidebar is taller than 640 px once a space opens, so every reveal has
    // to be scrolled to or the beat happens below the fold (it did, once).
    await nodes.last().scrollIntoViewIfNeeded()
    await beat(page, 1200)
    const featureSpecs = nodes.filter({ hasText: 'Feature Specs' })
    await featureSpecs.getByTestId('doc-tree-toggle').click()
    const childDoc = nodes.filter({ hasText: 'Billing Settings Spec' }).first()
    await expect(childDoc).toBeVisible()
    await childDoc.scrollIntoViewIfNeeded()
    await beat(page, 1700)
    // Collapse again so the sidebar is back to its resting shape for the loop.
    await docsSection.getByTestId('docs-space').filter({ hasText: 'PROD' }).click()
    await beat(page, 400)

    // ── Epics: re-section the same list by the hierarchy Jira hides ───────
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await beat(page, 450)
    await page.getByRole('button', { name: 'Epic', exact: true }).click()
    const headers = page.getByTestId('group-header')
    await expect(headers.first()).toBeVisible()
    await beat(page, 1500)
    // Down a couple of sections: one epic header is a feature, several in a row
    // are the hierarchy. Jump rather than smooth-scroll — a 1 s animation of a
    // full-width list is the single most expensive thing this GIF can contain.
    await page.getByTestId('issue-list-scroller').evaluate((el) => {
      el.scrollTop = 900
    })
    await beat(page, 1800)

    // ── ⌘K palette → an epic, which answers with its own rollup ───────────
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 450)
    await page.keyboard.type('NMB-195', { delay: 120 })
    await expect(palette.getByRole('option').first()).toContainText('NMB-195')
    await beat(page, 700)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // "4 of 12 done" plus the child list — the aggregate no JQL query returns.
    await expect(panel.getByTestId('epic-progress')).toBeVisible()
    await beat(page, 2100)

    // ── Rest on the grouped list for a clean loop ─────────────────────────
    await page.keyboard.press('Escape')
    await expect(listCount(page)).toBeVisible()
    await beat(page, 1400)
  })
})
