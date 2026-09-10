import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * Overlay the served config document for this page — the same mock pattern
 * built-in.spec.ts owns (serveWorkspaceKind). Kept local because
 * helpers.ts is not this round's to edit.
 */
async function serveConfigOverride(page: Page, extra: Record<string, unknown>): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = (await res.json()) as Record<string, unknown>
    await route.fulfill({ response: res, json: { ...doc, ...extra } })
  })
}

/** Search the key and open its panel. Exact row via data-issue-key (moved
 *  from ux-f12.spec.ts): hasText('NMA-1') also matches NMA-10/-100, and
 *  which comes first depends on the boot view's ordering (GDK-100). */
async function openIssueByKey(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  await page
    .locator(`[data-testid="issue-list-scroller"] [data-issue-key="${key}"]`)
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('detail', () => {
  test('row click opens detail panel with summary/history/comments sections', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Open an issue known to exist with changelog entries (NMB-110 has history).
    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // Section titles from en.ts detail.*
    await expect(panel.getByRole('heading', { name: 'Details' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Description' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'History' })).toBeVisible()
    // Issue key visible in the sticky header
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    // Write gate: the configured credential alone must unlock the write UI
    // (me/ → email → identified). Regression guard for the boot-time identity probe.
    await expect(panel.locator('textarea[placeholder*="Add a comment"]')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the list beside an open panel gives its width up from the chips, not the title', async ({
    page,
  }) => {
    /*
     * With a panel open, the rows behind it used to hand over the whole column:
     * the title fell to "Su…" and then to a single character, while the label
     * chips beside it kept every pixel they had asked for — the widest thing on
     * the row surrendering to the narrowest, and the document rows in the same
     * product ordering it the other way round (vision verdict 2026-08-07).
     *
     * Both halves are read, because a floor on the title alone would also be
     * satisfied by a row that simply overflows the column it sits in.
     */
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)

    const scroller = page.getByTestId('issue-list-scroller')
    // The row carrying the widest strip of chips: it is the one whose title
    // reaches its floor first, so it is where the two sides actually compete.
    // A row with one short label never runs out of room and would pass this
    // whether or not the rule exists.
    const widest = await scroller.evaluate((el) => {
      let best = { key: '', width: 0 }
      for (const row of el.querySelectorAll<HTMLElement>('[data-issue-key]')) {
        const strip = [...row.querySelectorAll<HTMLElement>('span')].find((s) =>
          s.querySelector('[title^="Labels:"]'),
        )
        const width = strip?.getBoundingClientRect().width ?? 0
        if (width > best.width) best = { key: row.dataset.issueKey ?? '', width }
      }
      return best
    })
    expect(widest.width, 'the fixture must show label chips somewhere').toBeGreaterThan(0)
    const key = widest.key
    const chips = scroller.locator(`[data-issue-key="${key}"] button[title^="Labels:"]`).first()
    const strip = scroller.locator(`[data-issue-key="${key}"]`).locator('span').filter({
      has: page.locator('button[title^="Labels:"]'),
    })
    const chipped = scroller.locator(`[data-issue-key="${key}"]`)

    // Open some other row, so the row being measured is not also the selected
    // one — selection paints a row, and a painted row is a second variable.
    await scroller.locator(`[data-issue-key]:not([data-issue-key="${key}"])`).first().click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(chipped).toBeVisible()

    const rows = scroller.locator('[data-issue-key]')
    const n = Math.min(8, await rows.count())
    expect(n).toBeGreaterThan(0)
    for (let i = 0; i < n; i++) {
      const title = await rows.nth(i).locator('span.flex-1').first().boundingBox()
      expect(title?.width ?? 0, `row ${i} title width`).toBeGreaterThanOrEqual(96)
    }

    // And the space came from the chips: that same row's strip is narrower than
    // it was with the whole column to itself.
    const narrow = await strip.boundingBox()
    expect(narrow?.width ?? 0).toBeLessThan(widest.width)

    /*
     * It came from dropping chips, not from grinding them down. The strip used
     * to hand its width over by truncation, which at this width left "cust…"
     * "d…" "r…" — labels narrower than the words in them, which read as a
     * rendering fault. Every chip still on screen has to be wide enough to be
     * a word; the bound sits under the chip's floor so this fails on fraying
     * rather than on a rounding difference.
     *
     * GDK-1050 (2026-08-27): that floor stepped 48 → 32px (2rem) so a
     * two-digit +N counter always fits beside the chip at the 64px slot step
     * (64 − 4 gap − 25 measured "+99" = 35px of chip room; a 40px chip
     * pushes the counter back out of the slot — the clipped-count defect
     * this closed). 31.5 = the 2rem floor with subpixel tolerance; it
     * still fails on fraying, not on rounding. FAIL-first: the unfixed
     * floor made this read 35 < 40 against the post-fix counter reserve.
     */
    const visibleChips = () =>
      scroller.evaluate((el) =>
        // Buttons only: the +N counters carry the same title (they name the
        // labels they stand for), and a count is not a chip.
        [...el.querySelectorAll<HTMLElement>('button[title^="Labels:"]')]
          .filter((chip) => chip.offsetParent !== null)
          .map((chip) => ({
            text: chip.textContent?.trim() ?? '',
            width: chip.getBoundingClientRect().width,
          })),
      )

    for (const chip of await visibleChips()) {
      expect(chip.width, `label chip "${chip.text}" width`).toBeGreaterThanOrEqual(31.5)
    }
    // One chip always stays — a detail-open list that folds to "+3" reads as
    // having no labels. Extra labels may still collapse to +N beside it.
    expect((await chips.innerText()).trim().length).toBeGreaterThan(1)
    expect((await chips.innerText()).trim()).not.toMatch(/^\+\d+$/)

    // The fold reverses. Widen until the list is back above the last step and
    // the chips return — otherwise "no frayed chips" would also be satisfied by
    // a row that had quietly stopped drawing labels at every width.
    await page.setViewportSize({ width: 1920, height: 900 })
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect
      .poll(async () => (await visibleChips()).length, {
        message: 'chips must come back once the list is wide again',
      })
      .toBeGreaterThan(0)
    for (const chip of await visibleChips()) {
      // Same floor as above — GDK-1050 stepped it 48 → 32px (2rem), and a
      // short label now renders at its own width instead of the old 48px
      // floor, so 40 would trip on label length, not on fraying.
      expect(chip.width, `wide-list label chip "${chip.text}" width`).toBeGreaterThanOrEqual(31.5)
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail header offers a label editor when a credential is stored', async ({ page }) => {
    /*
     * The fixture credential is fake, so a real PUT would fail at Jira. This
     * stops where triage.spec.ts stops: the add surface has to appear. The
     * write itself is TestLabelsSetAndClear.
     */
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const editor = panel.getByTestId('label-editor')
    await expect(editor).toBeVisible()

    await editor.getByTestId('label-editor-add').click()
    const field = editor.getByTestId('label-editor-input')
    await expect(field).toBeVisible()
    await field.fill('tech-debt')
    await expect(field).toHaveValue('tech-debt')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail header offers a priority picker when a credential is stored', async ({ page }) => {
    /*
     * GET priorities/ would hit Jira; the fixture token is fake. The catalog
     * is mocked so the menu can render. The write itself is TestPrioritySetAndClear.
     */
    const errors = attachConsoleErrors(page)
    await page.route('**/priorities/', (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      return route.fulfill({
        json: {
          priorities: [
            { id: '1', name: 'Highest' },
            { id: '2', name: 'High' },
            { id: '3', name: 'Medium' },
            { id: '4', name: 'Low' },
            { id: '5', name: 'Lowest' },
          ],
        },
      })
    })
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const chip = panel.getByTestId('priority-picker')
    await expect(chip).toBeVisible()
    await chip.click()
    const menu = page.getByRole('listbox', { name: 'Priority' })
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option', { name: 'None' })).toBeVisible()
    await expect(menu.getByRole('option', { name: 'High', exact: true })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail title becomes a field when clicked', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const title = panel.getByTestId('title-editor')
    await expect(title).toBeVisible()
    await title.click()
    const field = panel.getByTestId('title-editor-input')
    await expect(field).toBeVisible()
    await expect(field).toBeFocused()
    await field.press('Escape')
    await expect(panel.getByTestId('title-editor')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1290: the paste's first line is the origin's own page for the key —
   * the Jira /browse/ URL a teammate can open — followed by the app links, so
   * a paste into chat still opens gadak. The fixture serves site
   * https://nimbus.example.com (e2e/serve.sh), so this is the connected-Jira
   * shape; copy is asserted against the real clipboard, not the toast alone
   * (GDK-178: a toast that lies is worse than a button that fails aloud).
   */
  test('copy-link writes the origin URL first, then the gadak:// and http forms', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const copy = panel.getByTestId('issue-copy-link')
    await expect(copy).toBeVisible()
    await copy.click()

    const origin = new URL(page.url()).origin
    const want = `https://nimbus.example.com/browse/NMB-110\ngadak://view?issue=NMB-110\n${origin}/#/?issue=NMB-110`
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(want)

    // The toast names what line one is: the tracker whose page was copied.
    await expect(page.getByTestId('toast')).toContainText('Jira link copied')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1290, the other side: a workspace with no origin page — the built-in
   * tracker's serve sends jiraBaseUrl "" (originbind seeds cfg.Site = ""),
   * originType "gadak", workspaceKind "standalone" — copies exactly what it
   * copied before: the deep link first, then the serve http line. No first
   * line is invented for it, and the toast stays the plain "Copied".
   */
  test('copy-link without an origin page keeps the gadak:// and http forms', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await serveConfigOverride(page, {
      jiraBaseUrl: '',
      originType: 'gadak',
      workspaceKind: 'standalone',
    })
    // The built-in tracker stores a relative /browse/KEY as the row's url
    // (measured on a paired mirror); the Jira fixture's rows carry the
    // absolute nimbus URL, so the bootstrap is rewritten to the real shape —
    // otherwise the row-first resolver (GDK-1149) would honestly find a page.
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const res = await route.fetch()
      const doc = (await res.json()) as { issues: Array<Record<string, unknown>> }
      for (const it of doc.issues) it.url = `/browse/${String(it.issue_key)}`
      await route.fulfill({ response: res, json: doc })
    })
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    const copy = panel.getByTestId('issue-copy-link')
    await expect(copy).toBeVisible()
    await copy.click()

    const origin = new URL(page.url()).origin
    const want = `gadak://view?issue=NMB-110\n${origin}/#/?issue=NMB-110`
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(want)

    await expect(page.getByTestId('toast')).toContainText('Copied')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * Moved from ux-f7.spec.ts / ux-f12.spec.ts (v0.21 audit ladder round): the
   * audit-placement ux-fNN files were dissolved by surface; both of these are
   * detail-panel contracts. The /tmp/fNN-shots captures were deleted with the
   * move.
   */
  test('GDK-462: Esc in the comment composer blurs; the next Esc closes', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const panel = await openIssueByKey(page, 'NMB-110')

    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toBeVisible()
    await composer.fill('f7-esc-draft-must-survive')
    await expect(composer).toBeFocused()

    await page.keyboard.press('Escape')
    await expect(panel).toBeVisible()
    await expect(composer).not.toBeFocused()
    await expect(composer).toHaveValue('f7-esc-draft-must-survive')

    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()

    const reopened = await openIssueByKey(page, 'NMB-110')
    await expect(reopened.getByTestId('comment-composer')).toHaveValue('f7-esc-draft-must-survive')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-475: zero comments are counted once; shortcut lives on one kbd', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)
    // Flat view: the epic-grouped boot default (GDK-100) can leave the
    // searched row outside the virtual scroller, so the click never lands.
    await page.getByRole('button', { name: /All open/ }).click()
    const panel = await openIssueByKey(page, 'NMA-1')

    const comments = panel.getByRole('heading', { name: 'Comments' })
    await expect(comments).toBeVisible()
    await expect(comments).toHaveText(/Comments\s*0/)
    await expect(panel.getByText('No comments', { exact: true })).toHaveCount(0)

    const composer = panel.getByTestId('comment-composer')
    await expect(composer).toHaveAttribute('placeholder', en['write.commentPlaceholder'])

    const shortcut = panel.getByTestId('comment-shortcut')
    await expect(shortcut).toHaveCount(1)
    // GDK-354 / F-1: kbd is the platform modifier + the ↵ glyph the cheat
    // sheet prints (GDK-621) — not a catalog string hard-coding ⌘Enter on
    // every OS. Same platform test as modifierSymbol() in
    // web/src/lib/unified-search.ts. GDK-826: the catalog equality (every
    // locale '{mod} ↵', never a literal ⌘) is owned by
    // surface-consistency.test.ts, not re-asserted here. GDK-1783: the same
    // goes for the placeholder-copy regex that used to sit here — a pure
    // catalog assertion with no browser in sight, now also in
    // surface-consistency.test.ts.
    const mod = await page.evaluate(() =>
      /Mac|iP(hone|ad)/.test(navigator.platform) ? '⌘' : 'Ctrl',
    )
    const label = `${mod} ↵`
    await expect(shortcut).toHaveText(label)
    await expect(panel.getByText(label)).toHaveCount(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1753: the reopen verdict has one owner (the server's is_reopen, from
   * internal/store's ReopenTransition — done→non-done and in-progress→new).
   * NMB-3 on the demo mirror carries In Progress → Backlog, the row the old
   * done-only web rule left grey while SQL counted reopen_count=1 and the
   * feed showed a red event. The row must paint: red dot plus the badge.
   */
  test('GDK-1753: an in-progress → new move paints the history point as a reopen (NMB-3)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const panel = await openIssueByKey(page, 'NMB-3')

    const history = panel.getByRole('heading', { name: 'History' }).locator('..')
    // The phrase, not the two words: NMB-3 also carries the forward move
    // ("Backlog → In Progress") and filtering on both words matches it too.
    const row = history
      .locator('li')
      .filter({ hasText: /In Progress → Backlog/ })
      .first()
    await expect(row).toBeVisible()
    await expect(row).toContainText('Backlog')
    // The badge is the visible claim; the dot class is the paint the issue
    // was filed on ("타임라인 점은 무채색").
    await expect(row.getByText(en['feed.kindReopen'])).toBeVisible()
    await expect(row.locator('span.bg-status-reopen')).toHaveCount(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-1149: a Linear workspace has no site — its serve sends jiraBaseUrl ""
 * — but every mirrored row carries the page Linear itself minted
 * (items.url, "https://linear.app/<slug>/issue/KEY/<title>"). The origin
 * affordances that were Jira-only (key anchor, copy-link's first line, the
 * `o` command, the palette entry) must resolve through that stored URL, and
 * the copy names Linear, not Jira. The fixture is Jira-shaped, so the
 * bootstrap is rewritten in flight to give NMB-110 a Linear-shaped url.
 */
test.describe('detail — Linear origin (GDK-1149)', () => {
  const LINEAR_URL = 'https://linear.app/nimbus/issue/NMB-110/example-title-slug'

  async function serveLinearShape(page: Page): Promise<void> {
    await serveConfigOverride(page, {
      jiraBaseUrl: '',
      originType: 'linear',
      workspaceKind: 'connected',
    })
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const res = await route.fetch()
      const doc = (await res.json()) as { issues: Array<Record<string, unknown>> }
      for (const it of doc.issues) {
        if (it.issue_key === 'NMB-110') it.url = LINEAR_URL
      }
      await route.fulfill({ response: res, json: doc })
    })
  }

  test('key anchor, copy-link and the o command resolve the Linear page', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await serveLinearShape(page)
    await page.addInitScript(() => {
      const opened: string[] = []
      ;(window as unknown as { __gadakOpened: string[] }).__gadakOpened = opened
      window.open = (url?: string | URL) => {
        opened.push(String(url ?? ''))
        return null
      }
    })
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    // Key anchor: href is Linear's page; its title names Linear.
    const anchor = panel.getByRole('link', { name: 'NMB-110', exact: true }).first()
    await expect(anchor).toHaveAttribute('href', LINEAR_URL)
    await expect(anchor).toHaveAttribute('title', 'Open in Linear')

    // Copy-link: Linear page first, then the app links; toast names Linear.
    await panel.getByTestId('issue-copy-link').click()
    const origin = new URL(page.url()).origin
    const want = `${LINEAR_URL}\ngadak://view?issue=NMB-110\n${origin}/#/?issue=NMB-110`
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(want)
    await expect(page.getByTestId('toast')).toContainText('Linear link copied')

    // `o` with the detail open goes to the same page (serve: window.open).
    await page.keyboard.press('o')
    const opened = await page.evaluate(
      () => (window as unknown as { __gadakOpened: string[] }).__gadakOpened,
    )
    expect(opened).toEqual([LINEAR_URL])

    // Palette entry carries the tracker name.
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('Open in', { delay: 15 })
    await expect(palette.getByRole('option').filter({ hasText: 'Open in Linear' })).toHaveCount(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
