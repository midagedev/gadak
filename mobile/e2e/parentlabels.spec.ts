/*
 * GDK-1871 end to end: from an epic the phone files a child, and labels and
 * the due date are one-line edits.
 *
 * src/lib/labels.test.ts and src/lib/writes.test.ts measure the pure halves
 * — which rows the picker can offer, and the exact body each write sends.
 * They cannot measure the halves that reach a person: that the New child
 * control exists on an epic and nowhere else, that the sheet it opens knows
 * what it is filing under, that the labels row is a control at the touch
 * floor, and that a refused save keeps the set the person chose instead of
 * eating it.
 *
 * One route handler, the same one pagecomment.spec.ts and drafts.spec.ts
 * register and for the same reason: `gadak demo` is a credential-less serve,
 * so the store's writability probe (GET credential/) says off and every
 * write control ships disabled on this fixture — there would be nothing to
 * tap. Answering that one GET configured:true is the smallest change that
 * puts the real controls on screen. No write is faked: the serve still has
 * no origin credential, so the PUT answers 409 credential_required, which is
 * exactly the refusal the last test measures.
 *
 * Keys: NMB-194 is an epic in the demo fixture (hierarchy_level 1, the axis
 * the control keys on — never the type's display name) and NMB-105 is a
 * standard Bug under it, carrying the label `papercut`.
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'

const EPIC = 'NMB-194'
const STANDARD = 'NMB-105'

/** The 44pt floor DESIGN.md §4.2 puts on every control, in CSS pixels. */
const TOUCH_FLOOR = 44

async function armWrites(page: Page): Promise<void> {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
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

test('New child is offered on an epic and on nothing else', async ({ page }) => {
  await armWrites(page)
  await openIssue(page, EPIC)
  const onEpic = page.getByRole('button', { name: 'New child' })
  await expect(onEpic).toBeVisible()
  const box = await onEpic.boundingBox()
  expect(box, 'New child box').toBeTruthy()
  expect(box!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)

  // The standard row under that same epic. The server resolves the issue
  // type independently of `parent`, so a child filed here would be a
  // standard-type-with-parent, which Jira refuses — the absence is the
  // design, not an oversight.
  await openIssue(page, STANDARD)
  await expect(page.getByRole('button', { name: 'New child' })).toHaveCount(0)
})

test('the child sheet says what it is filing under and asks no project', async ({ page }) => {
  await armWrites(page)
  await openIssue(page, EPIC)
  await page.getByRole('button', { name: 'New child' }).click()
  await page.locator('.sheet').waitFor()

  const under = page.locator('[data-testid="create-parent"]')
  await expect(under).toBeVisible()
  await expect(under).toContainText(EPIC)

  // The project is not a question under a parent: the child goes where its
  // parent lives, so the picker the tab's own create can show is absent.
  await expect(page.locator('.sheet select#create-project')).toHaveCount(0)
  await expect(page.locator('.sheet input#create-summary')).toBeVisible()
})

test('the labels row is a control at the touch floor, and it knows the workspace’s labels', async ({
  page,
}) => {
  await armWrites(page)
  await openIssue(page, STANDARD)

  const row = page.locator('[data-testid="field-labels"]')
  await expect(row).toBeVisible()
  expect(await row.evaluate((el) => el.tagName.toLowerCase())).toBe('button')
  const box = await row.boundingBox()
  expect(box, 'labels row box').toBeTruthy()
  expect(box!.height).toBeGreaterThanOrEqual(TOUCH_FLOOR)
  await expect(row).toContainText('papercut')

  await row.click()
  await page.locator('.sheet').waitFor()
  // The rows are the labels the snapshot already holds, so this issue's own
  // must be among them — and it must be marked as on.
  const own = page.locator('.sheet button.t-row', { hasText: 'papercut' }).first()
  await expect(own).toBeVisible()
  await expect(own).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.sheet button.t-row')).not.toHaveCount(1)
  // The free line is the exception, not the way in: it sits under the rows.
  await expect(page.locator('.sheet .label-add input')).toBeVisible()
})

test('the due date on the meta line is a control even when there is none', async ({ page }) => {
  await armWrites(page)
  await openIssue(page, STANDARD)

  const due = page.locator('.meta button.m-btn.due')
  await expect(due).toBeVisible()
  await expect(due).toBeEnabled()
  await due.click()
  await expect(page.locator('.sheet input[type="date"]')).toBeVisible()
})

test('a refused save keeps the set the person chose', async ({ page }) => {
  await armWrites(page)
  await openIssue(page, STANDARD)
  await page.locator('[data-testid="field-labels"]').click()
  await page.locator('.sheet').waitFor()

  // Turn this issue's own label off and another one on: a set the row does
  // not hold, which is what the refusal must not eat.
  const own = page.locator('.sheet button.t-row', { hasText: 'papercut' }).first()
  await own.click()
  await expect(own).toHaveAttribute('aria-pressed', 'false')
  // Named, not positional: `[aria-pressed="false"]` stops matching the
  // moment the row is toggled on, so a live locator built from it would
  // resolve to a different row on the next read.
  const otherName = (
    await page.locator('.sheet button.t-row[aria-pressed="false"]').nth(1).innerText()
  ).trim()
  const other = page.locator('.sheet button.t-row', { hasText: otherName }).first()
  await other.click()
  await expect(other).toHaveAttribute('aria-pressed', 'true')

  // The serve has no origin credential, so the PUT answers 409
  // credential_required. Read after the refusal is on screen, never straight
  // after the click.
  const save = page.locator('.sheet button.save')
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page.locator('.sheet .field-err')).toBeVisible()

  // The sheet is still standing and still holds what was chosen. The other
  // sheets drop on a refusal so the sentence lands on the status row; these
  // hold something composed, and dropping that is the defect GDK-1863 closed
  // for words.
  await expect(page.locator('.sheet')).toBeVisible()
  await expect(own).toHaveAttribute('aria-pressed', 'false')
  await expect(other).toHaveAttribute('aria-pressed', 'true')
})
