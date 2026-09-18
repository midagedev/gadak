// Capture for the GDK-1873 review round: the page detail's new comment
// composer, armed, light and dark. The walk next door reaches a page detail
// but never types into it, so the armed fill has no picture — and the armed
// fill is the one value this round does not own (app.css `button.send.armed`,
// GDK-1525), which is exactly why it needs looking at on this screen.
//
// Own output dir so the walk's rmSync cannot take it. Same fixture, same
// 402×874 viewport, same ports as the gate (GADAK_MOBILE_E2E_PORT /
// GADAK_MOBILE_API_PORT) so e2e/serve.ts's stamp check accepts the server.
//
// `gadak demo` is credential-less, so the composer ships disabled here; the
// same one-GET override the e2e spec uses arms it. The refused state gets a
// frame too — it is what a person on this fixture actually sees.
import { test, type Page } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = join(here, '..', '..', 'scratch', 'mobile-shots', 'pagecomment')

const TYPED = 'One line on the doc, from the phone.'

async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
}

test('page comment composer, armed, light and dark', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // Documents → Updated, then the first page. The labelled button inside the
  // panel closes the sheet; the scrim carries the same aria-label.
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  // Not the first row: the most recently updated page has a short body and
  // no comments, so the composer would be photographed over nothing twice.
  // This one carries two comments (examples/demo.db: confluence:622707), so
  // the slab is read against the thread it joins.
  await page
    .locator('.pane:not(.off) button.row[data-testid="doc-row"]', {
      hasText: 'Notification Snooze Spec',
    })
    .first()
    .click()
  await page.locator('.page-detail button.back').waitFor()
  await page.locator('.page-detail .comment').first().waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '01-page-detail-composer.png') })

  const input = page.locator('.page-detail .composer input')
  await input.fill(TYPED)
  await settle(page)
  await page.screenshot({ path: join(outDir, '02-composer-armed.png') })

  // The thread's foot, so the composer sits directly under a real comment
  // rather than under the body.
  await page.locator('.page-detail main').first().evaluate((el) => {
    el.scrollTop = el.scrollHeight
  })
  await settle(page)
  await page.screenshot({ path: join(outDir, '03-composer-over-thread.png') })

  await page.emulateMedia({ colorScheme: 'dark' })
  await settle(page)
  await page.screenshot({ path: join(outDir, '04-composer-armed-dark.png') })

  // The refusal this fixture can actually produce: the POST 409s
  // credential_required and every control recedes, with the sentence above
  // the composer where the dimming cannot reach it.
  await page.emulateMedia({ colorScheme: 'light' })
  await page.locator('.page-detail main').first().evaluate((el) => {
    el.scrollTop = 0
  })
  await page.locator('.page-detail button.send').click()
  await page.locator('.page-detail .slab-err').waitFor()
  await settle(page)
  await page.screenshot({ path: join(outDir, '05-composer-refused.png') })

  await page.emulateMedia({ colorScheme: 'dark' })
  await settle(page)
  await page.screenshot({ path: join(outDir, '06-composer-refused-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
})
