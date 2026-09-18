/*
 * GDK-1879 end to end: a photo on a NEW issue.
 *
 * src/lib/create-attach.test.ts measures the two pure decisions (may create
 * fire, and the post-create upload sequencer that must never throw). Neither
 * can measure the half that reaches a thumb: that the create sheet offers a
 * picker at the touch floor, that it asks iOS for images, that a pick makes a
 * chip with a picture in it before anything has been uploaded, and that a
 * refused create on a read-only serve spends no upload.
 *
 * Two routes are armed on every test here, and both are necessary — measured
 * against this fixture's `gadak demo` serve on 2026-09-15:
 *
 *   GET /api/v1/credential/        → {"error":"not_found"}
 *   GET /api/v1/issues/create-meta/ → 409 {"error":"credential_required"}
 *
 * The first is what the store's writability probe reads, and without it every
 * write control ships disabled (the reason every write spec here arms it).
 * The second is read by the sheet itself at construction: a 409 there latches
 * `writesOff`, disables the title field, the paperclip and the create button,
 * and prints the credential sentence — so there would be nothing to tap. The
 * catalog answer is faked; no write is. The serve still has no origin
 * credential, which is exactly what the refusal test below relies on.
 *
 * Both are also routed BEFORE the sheet is first opened, because the catalog
 * is cached at module scope for the life of the page (CreateSheet.svelte's
 * `metaCache`): a sheet that has once seen the refusal keeps it.
 *
 * Keys: NMA-/NMB- only. The create itself is never allowed to land here — the
 * demo refuses it — so no key is invented.
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { pickChecker } from './checker-png'

/** The 44pt floor DESIGN.md §4.2 puts on every control, in CSS pixels. */
const TOUCH_FLOOR = 44

/** A standard Bug in the demo fixture — the key attach.spec.ts already opens. */
const ISSUE = 'NMB-105'

const META = {
  projects: [{ key: 'NMA', name: 'Northwind Mobile', issue_types: [{ id: '10001', name: 'Task' }] }],
}

async function armSheet(page: Page): Promise<void> {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })
  await page.route('**/api/v1/issues/create-meta/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(META),
    })
  })
}

/**
 * Let the sheet's rise finish before a rect is read. Same reason attach.spec
 * .ts settles the composer: a 44px control measured mid transform at
 * deviceScaleFactor 3 comes back a hair under the laid-out size.
 */
async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

/** The Issues tab's + action, which is the sheet's own front door. */
async function openCreateSheet(page: Page): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.head button.new').click()
  await page.locator('.create input#create-summary').waitFor()
  await settle(page)
}

/** Search → row → detail, the pane's own road to any key (a7-captures). */
async function openIssue(page: Page, key: string): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().fill(key)
  const row = page.locator('.pane:not(.off) button.row', { hasText: key }).first()
  await row.waitFor()
  await row.click()
  await page.locator('button.back').waitFor()
}

test('the create sheet offers a picker at the touch floor, asking iOS for images', async ({
  page,
}) => {
  await armSheet(page)
  await openCreateSheet(page)

  const attach = page.locator('.create .go-row button.attach')
  await expect(attach).toBeVisible()
  const box = await attach.boundingBox()
  expect(box, 'attach control box').toBeTruthy()
  expect(box!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)
  expect(box!.width).toBeGreaterThanOrEqual(TOUCH_FLOOR)

  // What actually opens the native sheet is the input's own attributes: a
  // WKWebView reads accept/multiple to decide whether Take Photo is offered
  // and whether more than one image may come back. Same spelling as the
  // composer's, because ui/AttachChips.svelte is the only place it is written.
  const file = page.locator('.sheet input[type="file"]')
  await expect(file).toHaveCount(1)
  await expect(file).toHaveAttribute('accept', 'image/*')
  expect(await file.evaluate((el: HTMLInputElement) => el.multiple)).toBe(true)
  // Hidden, not merely transparent: the viewport gate's input census counts
  // every input that paints, and this one must not be one of them.
  expect(await file.evaluate((el) => el.getBoundingClientRect().height)).toBe(0)

  // It sits outside .create, so `.create input` is still exactly one element
  // — the title field, which is how a2-captures.spec.ts spells it.
  await expect(page.locator('.create input')).toHaveCount(1)

  // Nothing picked yet, so no chip row.
  await expect(page.locator('[data-testid="create-attachments"]')).toHaveCount(0)
})

test('a pick makes a chip with the picture in it, and the × drops the file', async ({ page }) => {
  await armSheet(page)
  await openCreateSheet(page)

  // The button is not clicked: a real tap opens the OS picker, which in a
  // browser is a file chooser this harness would have to fake. setInputFiles
  // is the same event the picker produces, without the fake.
  await page.locator('.sheet input[type="file"]').setInputFiles(pickChecker())

  const chips = page.locator('[data-testid="create-attachments"]')
  await expect(chips).toBeVisible()
  await expect(chips.locator('.att-name')).toHaveText('field.png')

  // The thumbnail is the picked File's own bytes through an object URL —
  // nothing is fetched, because there is no key to fetch from yet. A painted
  // image, not merely a present <img>: the fixture has real pixels.
  const thumb = chips.locator('.att-thumb')
  await expect(thumb).toBeVisible()
  expect(await thumb.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0)
  expect(await thumb.evaluate((el: HTMLImageElement) => el.src.startsWith('blob:'))).toBe(true)

  // Nothing was uploaded by the pick: there is no issue to attach to.
  // (The refusal test below counts the requests; here the absence of a key is
  // the point — the app cannot have dialled an endpoint it cannot spell.)
  const x = chips.locator('button.att-x')
  await settle(page)
  const xBox = await x.boundingBox()
  expect(xBox, 'chip × box').toBeTruthy()
  expect(xBox!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)
  expect(xBox!.width).toBeGreaterThanOrEqual(TOUCH_FLOOR)

  // Typing is unaffected by the chip, and the create control arms on the title.
  await page.locator('.create input#create-summary').fill('Reproduced in the field')
  await expect(page.locator('.create button.go')).toBeEnabled()

  await x.click()
  await expect(chips).toHaveCount(0)
  await expect(page.locator('.create input#create-summary')).toHaveValue('Reproduced in the field')
})

test('a refused create on the demo spends no upload and says so in the sheet', async ({ page }) => {
  await armSheet(page)

  // Every request to the upload endpoint, counted. The contract is zero: the
  // create is refused, so there is never a key to attach to.
  const uploads: string[] = []
  page.on('request', (req) => {
    if (req.url().includes('/attachments/')) uploads.push(`${req.method()} ${req.url()}`)
  })

  await openCreateSheet(page)
  await page.locator('.sheet input[type="file"]').setInputFiles(pickChecker())
  await expect(page.locator('[data-testid="create-attachments"]')).toBeVisible()
  await page.locator('.create input#create-summary').fill('Portal login loops on a lowercase domain')

  const go = page.locator('.create button.go')
  await expect(go).toBeEnabled()
  await go.click()

  // The serve's own answer: POST issues/create/ → 409 credential_required
  // (measured on this fixture, 2026-09-15). That is the one refusal the sheet
  // latches rather than prints as a one-off, so the sentence lands in the
  // sheet's writes-off slot — the same `.off-note` a2-captures.spec.ts
  // photographs — and the controls recede with it.
  const note = page.locator('.create .off-note')
  await expect(note).toBeVisible()
  await expect(note).not.toHaveText('')
  await expect(page.locator('.create button.go')).toBeDisabled()
  await expect(page.locator('.create .go-row button.attach')).toBeDisabled()

  // Nothing was uploaded, and the chip is still there: no issue was filed, so
  // the file the person picked has not gone anywhere either.
  expect(uploads).toEqual([])
  await expect(page.locator('[data-testid="create-attachments"]')).toBeVisible()

  // No second sheet was opened and no issue was pushed: the detail layer is
  // absent, which is what "the create did not land" looks like on screen.
  await expect(page.locator('button.back')).toHaveCount(0)
})

test("the composer's own chips still work after the extraction", async ({ page }) => {
  // The smoke half of GDK-1879's extraction: ui/AttachChips.svelte now owns
  // the markup Detail.svelte used to spell inline, and e2e/attach.spec.ts
  // (untouched) is the full measure. This is the one assertion that would go
  // red here first if the two screens' picker ever forked again.
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })
  await page.route('**/api/v1/issues/*/attachments/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        origin: 'built-in',
        attachments: [
          {
            id: '10021',
            filename: 'field.png',
            mime_type: 'image/png',
            size: 1,
            media_id: '',
            is_image: true,
            is_video: false,
            content_url: `/api/v1/issues/${ISSUE}/attachments/10021/content/`,
          },
        ],
      }),
    })
  })

  await openIssue(page, ISSUE)
  await expect(page.locator('.composer button.attach')).toBeVisible()
  await page.locator('.composer-slab input[type="file"]').setInputFiles(pickChecker())

  const chips = page.locator('[data-testid="composer-attachments"]')
  await expect(chips).toBeVisible()
  await expect(chips.locator('.att-name')).toHaveText('field.png')
  // The comment field is still exactly one input under the selector three
  // other suites use — the reason the picker lives outside .composer.
  await expect(page.locator('.composer input')).toHaveCount(1)
})
