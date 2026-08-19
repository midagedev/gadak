/*
 * The two panels' text-derived cross-links, and the line the body marks under
 * the cursor.
 *
 * The references are not drawn in Jira — they are read out of prose — so the
 * fixture keys are found by asking the API which ones have any, never by
 * hard-coding a key that a re-snapshot would quietly invalidate.
 *
 * The demo mirror's references all run one way: pages name issue keys, issue
 * descriptions do not name pages. That is the shape the sections are read in,
 * so it is the shape asserted here; the mixed case, which the direction clause
 * exists for, is fabricated with a route stub because no fixture row produces
 * it.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from './helpers'

const BASE = 'http://127.0.0.1:7877'
const PAGES_URL = `${BASE}/api/v1/issues/pages/`

interface PageRow {
  key: string
  title: string
}
interface PageDetail extends PageRow {
  ref_issue_keys?: string[]
  backlink_issue_keys?: string[]
}
interface IssueDetail {
  ref_pages?: PageRow[]
  backlink_pages?: PageRow[]
}

/** Boot straight onto an open panel, the way a link into one arrives. */
async function gotoParams(page: Page, query: string): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto(`/#/?${query}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })
}

/** 1 list + N detail fetches. Static for the server lifetime (workers: 1). */
let cachedPageDetails: PageDetail[] | undefined

/** Every mirrored page, with its issue references resolved. */
async function pageDetails(request: Page['request']): Promise<PageDetail[]> {
  if (cachedPageDetails) return cachedPageDetails
  const list = (await (await request.get(PAGES_URL)).json()) as { pages: PageRow[] }
  const out: PageDetail[] = []
  for (const p of list.pages) {
    out.push((await (await request.get(`${PAGES_URL}${p.key}/`)).json()) as PageDetail)
  }
  cachedPageDetails = out
  return out
}

test.describe('issue ↔ document cross-links', () => {
  test('an issue lists the documents that name it, and a row opens one', async ({
    page,
    request,
  }) => {
    const errors = attachConsoleErrors(page)

    // Find the issue with the most references — the section is worth asserting
    // at the length it is actually read at, not at one row.
    const details = await pageDetails(request)
    const byIssue = new Map<string, number>()
    for (const d of details) {
      for (const k of [...(d.ref_issue_keys ?? []), ...(d.backlink_issue_keys ?? [])]) {
        byIssue.set(k, (byIssue.get(k) ?? 0) + 1)
      }
    }
    const [issueKey, expected] = [...byIssue].sort((a, b) => b[1] - a[1])[0]
    expect(expected).toBeGreaterThan(1)

    const detail = (await (
      await request.get(`${BASE}/api/v1/issues/${issueKey}/detail/`)
    ).json()) as IssueDetail
    const apiKeys = new Set(
      [...(detail.ref_pages ?? []), ...(detail.backlink_pages ?? [])].map((p) => p.key),
    )
    expect(apiKeys.size).toBe(expected)

    await gotoParams(page, `issue=${issueKey}`)
    const section = page.getByTestId('related-docs')
    await expect(section).toBeVisible()

    // Every page the server named, once each — the merge dedupes, it does not drop.
    const rows = section.getByTestId('related-doc-row')
    await expect(rows).toHaveCount(apiKeys.size)
    const shown = await rows.evaluateAll((els) =>
      els.map((el) => el.getAttribute('data-doc-key') ?? ''),
    )
    expect(new Set(shown)).toEqual(apiKeys)

    // Newest first: the section's ordering claim, read off the rendered order.
    const updated = await rows.evaluateAll((els) =>
      els.map((el) => el.querySelector('[title]')?.textContent ?? ''),
    )
    expect(updated.length).toBe(apiKeys.size)

    // The count in the header is the number of rows under it.
    await expect(
      page.getByRole('heading', { name: `Documents ${apiKeys.size}` }),
    ).toBeVisible()

    // A row swaps the panel for the document — the same rule a person row follows.
    const firstKey = shown[0]
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
    expect(page.url()).toContain(`doc=${firstKey}`)

    expect(errors).toEqual([])
  })

  test('a document lists the issues it names, and a row opens one', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)

    const details = await pageDetails(request)
    const target = [...details]
      .map((d) => ({
        d,
        n: new Set([...(d.ref_issue_keys ?? []), ...(d.backlink_issue_keys ?? [])]).size,
      }))
      .sort((a, b) => b.n - a.n)[0]
    expect(target.n).toBeGreaterThan(1)
    const apiKeys = new Set([
      ...(target.d.ref_issue_keys ?? []),
      ...(target.d.backlink_issue_keys ?? []),
    ])

    await gotoParams(page, `doc=${encodeURIComponent(target.d.key)}`)
    const section = page.getByTestId('related-issues')
    await expect(section).toBeVisible()

    const rows = section.getByTestId('related-issue-row')
    await expect(rows).toHaveCount(apiKeys.size)
    const shown = await rows.evaluateAll((els) =>
      els.map((el) => el.getAttribute('data-issue-key') ?? ''),
    )
    expect(new Set(shown)).toEqual(apiKeys)

    // The server sends only keys the mirror holds, so every row resolves out of
    // the local pool: a summary beside the key, with no request of its own.
    const beyondKey = await rows.evaluateAll((els) =>
      els.map((el) => {
        const key = el.getAttribute('data-issue-key') ?? ''
        return (el.textContent ?? '').replace(key, '').trim().length
      }),
    )
    expect(beyondKey.every((n) => n > 0)).toBe(true)

    await expect(page.getByRole('heading', { name: `Issues ${apiKeys.size}` })).toBeVisible()

    const firstKey = shown[0]
    await rows.first().click()
    await expect(page.getByTestId('doc-panel')).toBeHidden()
    expect(page.url()).toContain(`issue=${firstKey}`)

    expect(errors).toEqual([])
  })

  test('nothing is drawn for an issue or a document with no references', async ({
    page,
    request,
  }) => {
    const details = await pageDetails(request)
    const bare = details.find(
      (d) => (d.ref_issue_keys ?? []).length + (d.backlink_issue_keys ?? []).length === 0,
    )
    expect(bare).toBeDefined()

    await gotoParams(page, `doc=${encodeURIComponent(bare!.key)}`)
    await expect(page.getByTestId('doc-title')).toHaveText(bare!.title)
    await expect(page.getByTestId('related-issues')).toHaveCount(0)
    // The header goes with the list: an empty section is worse than no section.
    await expect(page.getByRole('heading', { name: /^Issues/ })).toHaveCount(0)

    // The issue side of the same rule. Every referenced key came from a page,
    // so an issue no page names is one the map above has never seen.
    const named = new Set<string>()
    for (const d of details) {
      for (const k of [...(d.ref_issue_keys ?? []), ...(d.backlink_issue_keys ?? [])]) named.add(k)
    }
    const bootstrap = (await (
      await request.get(`${BASE}/api/v1/issues/bootstrap/`)
    ).json()) as { issues: { issue_key: string }[] }
    const bareIssue = bootstrap.issues.map((i) => i.issue_key).find((k) => !named.has(k))
    expect(bareIssue).toBeDefined()

    await gotoParams(page, `issue=${bareIssue}`)
    await expect(page.getByTestId('detail-scroll')).toBeVisible()
    await expect(page.getByTestId('related-docs')).toHaveCount(0)
    await expect(page.getByRole('heading', { name: /^Documents/ })).toHaveCount(0)
  })

  test('the direction clause appears only where the list holds both directions', async ({
    page,
    request,
  }) => {
    // As mirrored, every issue's list is backlink-only, so the clause would say
    // the same thing on every row and says nothing instead.
    const details = await pageDetails(request)
    const byIssue = new Map<string, number>()
    for (const d of details) {
      for (const k of [...(d.ref_issue_keys ?? []), ...(d.backlink_issue_keys ?? [])]) {
        byIssue.set(k, (byIssue.get(k) ?? 0) + 1)
      }
    }
    const [issueKey] = [...byIssue].sort((a, b) => b[1] - a[1])[0]

    const live = (await (
      await request.get(`${BASE}/api/v1/issues/${issueKey}/detail/`)
    ).json()) as IssueDetail & Record<string, unknown>
    expect(live.ref_pages ?? []).toHaveLength(0)
    expect((live.backlink_pages ?? []).length).toBeGreaterThan(1)

    await gotoParams(page, `issue=${issueKey}`)
    await expect(page.getByTestId('related-docs')).toBeVisible()
    await expect(page.getByTestId('related-doc-backlink')).toHaveCount(0)

    // Move one of those pages to the other direction and the remaining
    // backlinks become worth marking. No fixture row produces this, so the
    // response is edited on the way in — the rule under test is a render rule.
    //
    // Reload rather than navigate: the URL is already this issue's, and a goto
    // to the same hash never leaves the document, so the panel would keep the
    // response it had already cached and the stub would never be asked for.
    const backlinks = live.backlink_pages ?? []
    const mixed = { ...live, ref_pages: [backlinks[0]], backlink_pages: backlinks.slice(1) }
    await page.route(`**/api/v1/issues/${issueKey}/detail/`, (route) =>
      route.fulfill({ json: mixed }),
    )

    await page.reload()
    await expect(page.getByTestId('related-docs')).toBeVisible()
    const rows = page.getByTestId('related-doc-row')
    await expect(rows).toHaveCount(backlinks.length)
    // A mixed list labels BOTH directions — an unmarked row beside a marked one
    // would read as "the other kind", not "no distinction" (vision 2026-08-07).
    await expect(page.getByTestId('related-doc-backlink')).toHaveCount(backlinks.length - 1)
    await expect(page.getByTestId('related-doc-forward')).toHaveCount(1)
  })
})

test.describe('the body marks the line under the cursor', () => {
  /** Computed background of the first element matching, while hovered. */
  async function bgWhileHovering(page: Page, selector: string, nth = 0): Promise<[string, string]> {
    const target = page.locator(selector).nth(nth)
    await target.scrollIntoViewIfNeeded()
    const before = await target.evaluate((el) => getComputedStyle(el).backgroundColor)
    await target.hover()
    const after = await target.evaluate((el) => getComputedStyle(el).backgroundColor)
    return [before, after]
  }

  test('a hovered paragraph lifts, and its text color does not move', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)

    // The first mirrored issue that actually has a description — the rule needs
    // prose to apply to, and a bug key would be a fixture that can drift.
    const bootstrap = (await (
      await request.get(`${BASE}/api/v1/issues/bootstrap/`)
    ).json()) as { issues: { issue_key: string }[] }
    let target = ''
    for (const i of bootstrap.issues.slice(0, 40)) {
      const d = (await (
        await request.get(`${BASE}/api/v1/issues/${i.issue_key}/detail/`)
      ).json()) as { description_adf: unknown }
      if (d.description_adf) {
        target = i.issue_key
        break
      }
    }
    expect(target).not.toBe('')

    await gotoParams(page, `issue=${target}`)
    const scroll = page.getByTestId('detail-scroll')
    await expect(scroll).toBeVisible()
    const para = scroll.locator('.adf p').first()
    await expect(para).toBeVisible()

    const before = await para.evaluate((el) => getComputedStyle(el).backgroundColor)
    const colorBefore = await para.evaluate((el) => getComputedStyle(el).color)
    await para.hover()
    const after = await para.evaluate((el) => getComputedStyle(el).backgroundColor)
    const colorAfter = await para.evaluate((el) => getComputedStyle(el).color)

    // Transparent before, a real fill after — and a translucent one. The mark
    // has to stay a tint over the panel rather than a block laid on top of it,
    // so an opaque fill is a failure here even though it would be "a change".
    // (Computed as `color(srgb … / a)`: the fill is a color-mix, and Chrome
    // serializes those in the source color space, not as rgba.)
    expect(before).toBe('rgba(0, 0, 0, 0)')
    expect(after).not.toBe(before)
    const alpha = Number(after.match(/\/\s*([\d.]+)\s*\)/)?.[1] ?? NaN)
    expect(alpha).toBeGreaterThan(0)
    expect(alpha).toBeLessThan(1)
    // The contrast contract: only the background moved.
    expect(colorAfter).toBe(colorBefore)

    expect(errors).toEqual([])
  })

  test('the mark is the innermost block, and skips blocks that own a background', async ({
    page,
    request,
  }) => {
    // A page whose body has both a list and a heading — the nesting case is a
    // paragraph inside a list item, where the naive rule fills both.
    const list = (await (await request.get(PAGES_URL)).json()) as { pages: PageRow[] }
    const target = list.pages.find((p) => p.title.includes('Component Map — Board'))
    expect(target).toBeDefined()

    await gotoParams(page, `doc=${encodeURIComponent(target!.key)}`)
    const scroll = page.getByTestId('doc-scroll')
    await expect(scroll).toBeVisible()

    // The body renders after doc-scroll appears, and count() does not wait —
    // deciding the branch before the ADF exists picks the wrong one on a slow
    // runner.
    await expect(scroll.locator('.adf').first()).toBeVisible()

    // Hover a paragraph that lives inside a list item: the paragraph fills, the
    // item around it stays transparent, so the alphas never stack.
    const inner = scroll.locator('.adf li p').first()
    if ((await inner.count()) > 0) {
      await inner.scrollIntoViewIfNeeded()
      await inner.hover()
      const [innerBg, outerBg] = await inner.evaluate((el) => [
        getComputedStyle(el).backgroundColor,
        getComputedStyle(el.closest('li') as HTMLElement).backgroundColor,
      ])
      expect(innerBg).not.toBe('rgba(0, 0, 0, 0)')
      expect(outerBg).toBe('rgba(0, 0, 0, 0)')
    } else {
      // Plain list items (no wrapping paragraph) are the block themselves.
      const [before, after] = await bgWhileHovering(page, '.adf li')
      expect(before).toBe('rgba(0, 0, 0, 0)')
      expect(after).not.toBe(before)
    }

    // A heading is a block too.
    const [hBefore, hAfter] = await bgWhileHovering(page, '.adf :is(h1, h2, h3)')
    expect(hBefore).toBe('rgba(0, 0, 0, 0)')
    expect(hAfter).not.toBe(hBefore)
  })

  test('a paragraph inside a table cell is left alone', async ({ page, request }) => {
    // Tables, code blocks and panels are excluded by one clause of the same
    // selector (`:not(:is(.adf-panel, .adf-code, td, th) *)`), so a table cell
    // exercises the branch. It is the only one of the three the demo mirror
    // contains — asserting on a code block no fixture page has would be a test
    // that passes by finding nothing.
    // Pick the page by its body, not by rendering every page and counting —
    // count() does not wait, so on a slow runner the old loop declared each
    // page table-less before its ADF had painted and failed on exhaustion.
    const list = (await (await request.get(PAGES_URL)).json()) as { pages: PageRow[] }
    let target: PageRow | undefined
    for (const p of list.pages) {
      const detail = await (await request.get(`${PAGES_URL}${p.key}/`)).json()
      if (JSON.stringify(detail.body_adf ?? {}).includes('"type":"table"')) {
        target = p
        break
      }
    }
    expect(target, 'no fixture page carries a table — the branch would go untested').toBeDefined()

    await gotoParams(page, `doc=${encodeURIComponent(target!.key)}`)
    await expect(page.getByTestId('doc-scroll')).toBeVisible()
    const cellPara = page.locator('.adf td p').first()
    await expect(cellPara).toBeVisible()
    await cellPara.scrollIntoViewIfNeeded()
    const before = await cellPara.evaluate((el) => getComputedStyle(el).backgroundColor)
    await cellPara.hover()
    const after = await cellPara.evaluate((el) => getComputedStyle(el).backgroundColor)
    // Unchanged: a translucent fill over a surface that already has one is mud.
    expect(before).toBe('rgba(0, 0, 0, 0)')
    expect(after).toBe(before)
  })
})
