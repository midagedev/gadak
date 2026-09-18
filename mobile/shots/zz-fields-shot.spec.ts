// Extra capture for the GDK-1870 review round: the Fields section on Detail.
// The walk next door opens whatever row happens to be first, which on this
// fixture carries no labels and no parent — so the section the round adds has
// no picture. NMB-105 is the row that does: labels, a component, a fix version
// and a parent (NMB-194) that is also its epic, which is the case where the
// epic row is deliberately absent.
//
// Same fixture and same 402×874 viewport as the walk, its own output dir so
// the walk's rmSync cannot take it.
import { test, expect, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = join(here, '..', '..', 'scratch', 'mobile-shots', 'fields')

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

/** Opens NMB-105 from Search, the one road that does not depend on a scope. */
async function openFieldRichIssue(page: Page): Promise<void> {
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().waitFor()
  await page.locator('.pane:not(.off) input').first().fill('NMB-105')
  const hit = page.locator('.pane:not(.off) button.row', { hasText: 'NMB-105' }).first()
  await hit.waitFor()
  await hit.click()
  await page.locator('[data-testid="detail-fields"]').waitFor()
}

test('the Fields section on Detail, light and dark', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await settle(page)

  await openFieldRichIssue(page)
  const fields = page.locator('[data-testid="detail-fields"]')

  // The capture is worth nothing if it photographs the wrong rows, so the
  // shot asserts what it is pointing at: the four the mirror holds for this
  // issue, and the epic absent because it is the parent (NMB-194).
  const labels = await fields.locator('.f-label').allInnerTexts()
  expect(labels).toEqual(['Parent', 'Labels', 'Components', 'Fix versions'])
  await expect(fields.locator('.f-key')).toHaveText(['NMB-194'])

  await fields.scrollIntoViewIfNeeded()
  await settle(page)
  await page.screenshot({ path: join(outDir, '01-detail-fields.png') })

  // The whole screen from the top, so the section can be read in its place —
  // after the description, before linked issues.
  await page.locator('.detail-layer main, .detail-layer .body').first().evaluate((el) => {
    el.scrollTop = 0
  })
  await settle(page)
  await page.screenshot({ path: join(outDir, '02-detail-top.png') })

  await fields.scrollIntoViewIfNeeded()
  await settle(page)
  await page.emulateMedia({ colorScheme: 'dark' })
  await settle(page)
  await page.screenshot({ path: join(outDir, '03-detail-fields-dark.png') })

  // The parent row is a place to go: tapping it swaps the detail in place.
  // The header is the summary, not the key, so the swap is proved by the
  // subject changing — without that the shot could photograph NMB-105 twice.
  // Scoped to the detail layer and taken last: the Search pane stays mounted
  // behind it with an <h1> of its own, and openIssue stacks a new layer.
  const subject = page.locator('.detail-layer h1.type-subject').last()
  const before = await subject.innerText()
  await fields.locator('.f-key').first().click()
  await expect(subject).not.toHaveText(before)
  await expect(subject).not.toHaveText('')
  await settle(page)
  await page.screenshot({ path: join(outDir, '04-parent-opened-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
})
