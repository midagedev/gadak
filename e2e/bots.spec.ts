/*
 * Bot workers on the web (GDK-590). The serve.sh fixture injects one agent
 * account (acc-e2e-bot, "Claude (build 1)") touching NMB-112 (comment) and
 * NMB-139 (comment + a dev-panel PR link it attached). NMB-139 also has the
 * deterministic wait span: created 14:56:24.755Z → first in-progress
 * 15:03:12.577Z on 2026-07-20 = 6m47s → "Waited 6m". Progress runs to Now,
 * so only its presence is asserted, never its number.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, searchInput } from './helpers'

const BOT = 'Claude (build 1)'

async function openDetail(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: key })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('bot workers (GDK-590)', () => {
  test('a bot comment wears the badge; human comments wear none', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // NMB-112: 3 human comments + the bot's claim.
    const panel = await openDetail(page, 'NMB-112')
    const badges = panel.getByTestId('bot-badge')
    await expect(badges).toHaveCount(1)
    await expect(badges.first()).toHaveText('Bot')
    // The badge sits in the bot's own comment header, next to its name.
    await expect(panel.getByText(BOT)).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the actor filter narrows to the issues the bot touched', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/?ac=acc-e2e-bot')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    // Sidebar pool stays 534; the narrowed list is what this filter owes.
    await expect(page.getByTestId('list-count')).toHaveText('2 issues', { timeout: 30_000 })

    // Both touched keys are in the list, and the chip names the member, not
    // the raw account id.
    const rows = page.locator('[data-testid="issue-list-scroller"] [role="button"]')
    await expect(rows.filter({ hasText: 'NMB-112' })).toHaveCount(1)
    await expect(rows.filter({ hasText: 'NMB-139' })).toHaveCount(1)
    await expect(page.getByText('Actor: Claude (build 1)')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('detail shows the wait/progress chip and names who linked the PR', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const panel = await openDetail(page, 'NMB-139')
    const chip = panel.getByTestId('duration-chip')
    await expect(chip).toBeVisible()
    await expect(chip).toContainText('Waited 6m')
    // In progress runs to now — presence only, the number would be a flake.
    await expect(chip).toContainText('In progress')

    // The dev-panel link the bot attached: linked-by names it (with badge),
    // separate from the PR's own author.
    await expect(panel.getByText(`Linked by ${BOT}`)).toBeVisible()
    await expect(panel.getByTestId('bot-badge').first()).toHaveText('Bot')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the breakdown axis sections the list by actor, shared issues in every bucket', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/?ac=acc-e2e-bot')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('list-count')).toHaveText('2 issues', { timeout: 30_000 })

    // Pick the actor axis from the breakdown menu the way a person does.
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Actor', exact: true }).click()

    // The bot's bucket counts both issues; Priya's bucket also counts both —
    // NMB-112 and NMB-139 were touched by bot and human alike, and grouping
    // must not pick a winner (multi-membership, GDK-590).
    const headers = page.getByTestId('group-header')
    const botHeader = headers.filter({ hasText: BOT })
    await expect(botHeader).toBeVisible({ timeout: 30_000 })
    await expect(botHeader).toContainText('2')
    const priyaHeader = headers.filter({ hasText: 'Priya Sharma' })
    await expect(priyaHeader).toContainText('2')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
