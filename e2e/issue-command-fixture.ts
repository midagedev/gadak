/*
 * The setup shared by the command-block gate (e2e/issue-command.spec.ts) and
 * its captures (e2e/demo/issue-command-shots.spec.ts).
 *
 * A module rather than an export from the spec: importing a spec file
 * registers its tests, so the two would run each other. This is also why the
 * captures could not simply stay in the gate file — they are a screenshot
 * helper, and the CI set is e2e/*.spec.ts minus demo/, hosted/ and perf/
 * (e2e/playwright.config.ts), so anything left here is a merge gate.
 */

import { expect, type Page } from '@playwright/test'
import { apiURL } from './helpers'

/* Two in-progress demo issues. One gets a shell, the other does not — the
 * second is both the disabled-▶ case and the "No shell here" case. */
export const BOUND_ISSUE = 'NMA-1'
export const LONE_ISSUE = 'NMS-3'

/* The marker rides printf's %s so the *placed* text and the text it would
 * print are different strings: "GDK1162-RAN" on screen can only have come
 * from running the line, never from echoing it. */
export const COMMAND = `printf 'GDK1162%s\\n' -RAN`
export const OUTPUT = 'GDK1162-RAN'
export const MULTILINE = 'cd web\nnpm run build'

/**
 * Rewrite the detail response for both issues so each carries one runnable
 * code block and one that is not runnable. The multi-line block is the pin
 * that "runnable" is narrow: the serve refuses a payload with a newline, so a
 * ▶ on that block would be a button that always fails.
 *
 * The body is injected by intercepting the detail response rather than by
 * editing examples/demo.db: the fixture is shared by every other suite, and a
 * code block added to it would be a change those suites have to absorb for a
 * feature they do not test.
 */
export async function injectCommandBodies(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/*/detail/', async (route) => {
    const res = await route.fetch()
    const body = (await res.json()) as Record<string, unknown>
    const key = String(body.issue_key ?? '')
    if (key !== BOUND_ISSUE && key !== LONE_ISSUE) {
      await route.fulfill({ response: res })
      return
    }
    body.description_adf = {
      type: 'doc',
      version: 1,
      content: [
        { type: 'paragraph', content: [{ type: 'text', text: 'Reproduce it:' }] },
        { type: 'codeBlock', attrs: { language: 'sh' }, content: [{ type: 'text', text: COMMAND }] },
        { type: 'codeBlock', content: [{ type: 'text', text: MULTILINE }] },
      ],
    }
    await route.fulfill({ response: res, json: body })
  })
}

/** The live session table, as the serve reports it. */
export async function sessions(page: Page): Promise<{ id: string; issue_key?: string }[]> {
  const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
  return ((await res.json()) as { sessions?: { id: string; issue_key?: string }[] }).sessions ?? []
}

/** Open the pane and bind its session to `key`, the way `gadak claim` does. */
export async function openPaneBoundTo(page: Page, key: string): Promise<string> {
  await page.keyboard.press('Control+Backquote')
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
  const [session] = await sessions(page)
  expect(session, 'the pane should have created a session').toBeTruthy()
  const bind = await page.request.post(apiURL(`/api/v1/terminal/sessions/${session.id}/issue/`), {
    data: { issue_key: key },
  })
  expect(bind.ok(), `binding ${key}: ${bind.status()}`).toBe(true)
  // A split, not an overlay: an overlay covers the body the ▶ lives in.
  await expect(page.getByTestId('terminal-pane')).not.toHaveAttribute('data-overlay', 'true')
  return session.id
}

export async function openIssue(page: Page, key: string): Promise<void> {
  await page.goto(`/#/?issue=${key}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('issue-detail-panel')).toBeVisible({ timeout: 15_000 })
}

export async function focusTerm(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) await host.first().click({ position: { x: 24, y: 24 } })
  else await pane.click({ position: { x: 24, y: 24 } })
  await page.evaluate(() => {
    document.querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')?.focus()
  })
}
