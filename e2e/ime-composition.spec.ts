/*
 * GDK-169: Korean IME mid-composition must not be committed as a search.
 *
 * Playwright cannot type through a real IME. Svelte listens to native DOM
 * events, so a CompositionEvent + InputEvent(isComposing) sequence on the
 * SearchBox is the same path a Hangul IME uses.
 *
 * examples/demo.db has 0 title/key/assignee/label hits for every 딥링크
 * intermediate and for the final string. Seed a committed query first so
 * "the last committed results stay on screen" is observable — without a
 * seed the list is empty the whole way and the flash is invisible.
 */
import { test, expect, type Locator } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

const STEPS = ['디', '딥', '딥ㄹ', '딥리', '딥링', '딥링ㅋ', '딥링크'] as const
const FINAL = '딥링크'
const SEED = 'NMB-110'

async function compositionStart(input: Locator): Promise<void> {
  await input.evaluate((el) => {
    el.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true, data: '' }))
  })
}

async function compositionUpdate(input: Locator, text: string): Promise<void> {
  await input.evaluate((el, value) => {
    const node = el as HTMLInputElement
    node.value = value
    node.dispatchEvent(new CompositionEvent('compositionupdate', { bubbles: true, data: value }))
    node.dispatchEvent(
      new InputEvent('input', {
        bubbles: true,
        data: value,
        isComposing: true,
        inputType: 'insertCompositionText',
      }),
    )
  }, text)
}

async function compositionEnd(input: Locator, text: string): Promise<void> {
  await input.evaluate((el, value) => {
    const node = el as HTMLInputElement
    node.value = value
    node.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true, data: value }))
    node.dispatchEvent(
      new InputEvent('input', {
        bubbles: true,
        data: value,
        isComposing: false,
        inputType: 'insertCompositionText',
      }),
    )
  }, text)
}

test.describe('IME composition (GDK-169)', () => {
  test('SearchBox keeps the last committed rows through 딥링크 jamo states', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.fill(SEED)
    await expect(page.getByTestId('list-count')).toHaveText('1 issues')
    await expect(page.getByText('No issues match', { exact: true })).toHaveCount(0)
    const seedCount = await page.getByTestId('list-count').textContent()

    await compositionStart(input)
    const mid: { q: string; count: string | null; empty: boolean }[] = []
    for (const q of STEPS) {
      await compositionUpdate(input, q)
      mid.push({
        q,
        count: await page.getByTestId('list-count').textContent(),
        empty: await page.getByText('No issues match', { exact: true }).isVisible(),
      })
    }

    expect(mid, `mid-composition snapshots: ${JSON.stringify(mid)}`).toEqual(
      STEPS.map((q) => ({ q, count: seedCount, empty: false })),
    )
    await expect(input).toHaveValue(FINAL)

    await compositionEnd(input, FINAL)
    await expect(page.getByText('No issues match', { exact: true })).toBeVisible()
    await expect(page.getByTestId('list-count')).toHaveText('0 issues')
    await expect(input).toHaveValue(FINAL)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
