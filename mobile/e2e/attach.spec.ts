/*
 * GDK-1872 part 2 end to end: a photo goes with the comment.
 *
 * src/lib/composer-attach.test.ts measures the two pure decisions (may Send
 * fire, what the POST carries) and src/lib/attach.test.ts the transport.
 * Neither can measure the half that reaches a thumb: that the picker exists
 * at the touch floor, that it asks iOS for images, that the Send button says
 * what it is doing while bytes are in flight, and that a refused upload
 * degrades down the one refusal road this screen already has.
 *
 * Same route handler every write spec here registers, and for the same
 * reason (parentlabels.spec.ts): `gadak demo` is a credential-less serve, so
 * the store's writability probe answers off and every write control ships
 * disabled — there would be nothing to tap. Answering that one GET
 * configured:true is the smallest change that puts the real controls on
 * screen. No write is faked by it: the serve still has no origin credential.
 *
 * The upload responses ARE routed, on purpose. The serve's own 409 lands in
 * single-digit milliseconds, so "Send reads Uploading… while a file is in
 * flight" cannot be asserted against it without a race — the handler below
 * holds the request open until the assertion has been made, then answers.
 * The answer it gives is the serve's own: 409 credential_required, the code
 * demo.ts and the credential-less serve both produce (mobile/src/lib/demo.ts
 * ~121; measured on this fixture for transitions in e2e/viewport.spec.ts).
 *
 * Key: NMB-105 is a standard Bug in the demo fixture (the key parentlabels
 * .spec.ts already opens for its labels row).
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'

const ISSUE = 'NMB-105'

/** The 44pt floor DESIGN.md §4.2 puts on every control, in CSS pixels. */
const TOUCH_FLOOR = 44

/**
 * A real 1×1 PNG, built here rather than committed: an e2e fixture that is a
 * binary file is one more thing to keep in sync with nothing.
 */
const PNG_1PX = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
)

function pick(): { name: string; mimeType: string; buffer: Buffer } {
  return { name: 'field.png', mimeType: 'image/png', buffer: PNG_1PX }
}

async function armWrites(page: Page): Promise<void> {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })
}

/**
 * Let the detail layer's rise finish before a rect is read.
 *
 * Same reason viewport.spec.ts settles a sheet: a 44px control measured mid
 * transform at deviceScaleFactor 3 comes back a hair under the laid-out size
 * — 43.999996185302734, measured here on the attach button. Measuring a
 * settled layer is the honest reading; the floor itself is untouched.
 */
async function settle(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
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

test('the composer offers a picker at the touch floor, asking iOS for images', async ({
  page,
}) => {
  await armWrites(page)
  await openIssue(page, ISSUE)

  const attach = page.locator('.composer button.attach')
  await expect(attach).toBeVisible()
  await settle(page)
  const box = await attach.boundingBox()
  expect(box, 'attach control box').toBeTruthy()
  expect(box!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)
  expect(box!.width).toBeGreaterThanOrEqual(TOUCH_FLOOR)

  // What actually opens the native sheet is the input's own attributes: a
  // WKWebView reads accept/multiple to decide whether Take Photo is offered
  // and whether more than one image may come back.
  const file = page.locator('.composer-slab input[type="file"]')
  await expect(file).toHaveCount(1)
  await expect(file).toHaveAttribute('accept', 'image/*')
  expect(await file.evaluate((el: HTMLInputElement) => el.multiple)).toBe(true)
  // Hidden, not merely transparent: the viewport gate's census counts every
  // input that paints, and this one must not be one of them.
  expect(await file.evaluate((el) => el.getBoundingClientRect().height)).toBe(0)

  // Nothing attached yet, so no chip row and the slab is the height it was.
  await expect(page.locator('[data-testid="composer-attachments"]')).toHaveCount(0)
})

test('Send says it is uploading, then the refusal takes the one latch', async ({ page }) => {
  await armWrites(page)

  // Held open until the "Uploading…" assertion has been made, then answered
  // with the refusal the credential-less serve gives every non-GET.
  let release: (() => void) | null = null
  const held = new Promise<void>((resolve) => (release = resolve))
  await page.route('**/api/v1/issues/*/attachments/', async (route) => {
    await held
    await route.fulfill({
      status: 409,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'credential_required' }),
    })
  })

  await openIssue(page, ISSUE)
  const send = page.locator('.composer button.send')
  await page.locator('.composer-slab input[type="file"]').setInputFiles(pick())

  // State in the pressed control (DESIGN.md §3.5) — no spinner anywhere.
  await expect(send).toHaveText('Uploading… (1)')
  await expect(send).toBeDisabled()
  expect(await page.locator('.composer .spinner, .composer progress').count()).toBe(0)

  release!()

  // GDK-933: writes-off is one surface. The refusal latches the whole screen,
  // and the sentence is the status row's — .send-error hides rather than
  // repeating it. The composer recedes with it.
  await expect(page.locator('.composer.off')).toHaveCount(1)
  await expect(page.locator('button.status .status-err')).toBeVisible()
  await expect(page.locator('.composer .send-error')).toHaveCount(0)
  // Nothing was attached, so no chip was left behind claiming otherwise.
  await expect(page.locator('[data-testid="composer-attachments"]')).toHaveCount(0)
})

test('an upload that fails for another reason names the file and keeps writes on', async ({
  page,
}) => {
  await armWrites(page)
  await page.route('**/api/v1/issues/*/attachments/', async (route) => {
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'internal_error' }),
    })
  })

  await openIssue(page, ISSUE)
  await page.locator('.composer-slab input[type="file"]').setInputFiles(pick())

  // The catalog's own sentence, carrying the name of the file that did not
  // make it — the only place on this screen where a per-file failure can be
  // said. Writes stay on: this was not a refusal.
  await expect(page.locator('.composer .send-error')).toHaveText(
    'Attachment upload failed: field.png',
  )
  await expect(page.locator('.composer.off')).toHaveCount(0)
  await expect(page.locator('.composer button.send')).toHaveText('Comment')
})

test('a chip carries the file, and its × leaves the comment alone', async ({ page }) => {
  await armWrites(page)
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
            size: PNG_1PX.length,
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
  // The button is not clicked: a real tap opens the OS picker, which in a
  // browser is a file chooser this harness would have to fake. setInputFiles
  // is the same event the picker produces, without the fake.
  const slab = page.locator('.composer-slab')
  await settle(page)
  const bare = (await slab.boundingBox())!.height
  await page.locator('.composer-slab input[type="file"]').setInputFiles(pick())

  const chips = page.locator('[data-testid="composer-attachments"]')
  await expect(chips).toBeVisible()
  await expect(chips.locator('.att-name')).toHaveText('field.png')

  // The remove control is a control: 44pt like every other one on this screen.
  const x = chips.locator('button.att-x')
  await settle(page)
  const box = await x.boundingBox()
  expect(box, 'chip × box').toBeTruthy()
  expect(box!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)
  expect(box!.width).toBeGreaterThanOrEqual(TOUCH_FLOOR)

  // Typing is unaffected by the chip; the comment is still the comment.
  await page.locator('.composer input').fill('from the field')
  await expect(page.locator('.composer button.send')).toBeEnabled()

  // The slab grew by exactly one row of chips and by nothing else, and it
  // gives that row back when the chip goes (the round's contract: the
  // composer may only grow while something is attached).
  const withChip = (await slab.boundingBox())!.height
  expect(withChip).toBeGreaterThan(bare)
  expect(withChip - bare).toBeLessThanOrEqual(TOUCH_FLOOR + 16)

  await x.click()
  await expect(chips).toHaveCount(0)
  await expect(page.locator('.composer input')).toHaveValue('from the field')
  expect((await slab.boundingBox())!.height).toBe(bare)
})

test('an attachment alone does not arm Send — the server needs the text', async ({ page }) => {
  // internal/server/write.go:615 answers 400 text_required on an empty body
  // before it reads attachment_ids, so arming here would offer a control
  // whose only outcome is a refusal with no sentence of its own. The file is
  // already on the issue by then; the comment only embeds it.
  await armWrites(page)
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
            size: PNG_1PX.length,
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
  await page.locator('.composer-slab input[type="file"]').setInputFiles(pick())
  await expect(page.locator('[data-testid="composer-attachments"]')).toBeVisible()

  const send = page.locator('.composer button.send')
  await expect(send).toBeDisabled()
  await expect(send).not.toHaveClass(/armed/)

  await page.locator('.composer input').fill('one line')
  await expect(send).toBeEnabled()
  await expect(send).toHaveClass(/armed/)
})
