import { spawn, execFileSync, type ChildProcess } from 'node:child_process'
import { mkdirSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from './helpers'
import {
  appConsoleErrors,
  attachConsoleErrors,
  builtinServePort,
  e2eDir,
  forceLocale,
  openServerSettings,
  repoRoot,
} from './helpers'

/**
 * GDK-1973: the declared-name field — "You on this tracker" — lives on the
 * Workspaces tab only where its verb exists, a gadak origin. The suite's
 * default serve is the connected (Jira) fixture, where the block must be
 * absent rather than disabled, so the positive half runs a second serve on
 * its own port over its own GADAK_HOME whose origin is the built-in tracker
 * (the same discipline as built-in-attachments.spec.ts).
 *
 * The assertions are what a person sees and what the document says: a typed
 * name saves with a toast that names it, GET settings/ carries the person
 * block the CLI's `gadak me` writes (kind person, slug person:*), a reload
 * holds the field, and emptying the field clears the block — the document
 * loses the actor key, it does not store an empty name.
 *
 * Port: this spec's own, never derived from the suite's (GDK-1789) —
 * builtinServePort() pins GADAK_E2E_BUILTIN_PORT or grabs an ephemeral one,
 * and the healthz poll below proves the answering server's identity.
 */

const BIN = join(e2eDir(), '.tmp', 'gadak')
let PORT = ''
let BASE = ''
let HOME = ''

let serve: ChildProcess | undefined
// Last ~4 KiB of the serve's output — a serve that cannot bind exits with
// the reason on stderr, and without this latch the poll below would report
// whichever impostor took the port instead (GDK-1789).
let serveLog = ''

function captureServeOutput(child: ChildProcess): void {
  const keep = (chunk: Buffer): void => {
    serveLog = (serveLog + chunk.toString()).slice(-4096)
  }
  child.stdout?.on('data', keep)
  child.stderr?.on('data', keep)
}

function cli(...args: string[]): string {
  return execFileSync(BIN, args, {
    env: { ...process.env, GADAK_HOME: HOME, GADAK_WORKSPACE: '' },
    encoding: 'utf8',
  })
}

/** 'up' once this spec's own serve answers — proven by the healthz home
 *  field, not by a bare 200 (GDK-1555/GDK-1789). */
async function pollServeIdentity(): Promise<'up' | 'down'> {
  let res: Response
  try {
    res = await fetch(`${BASE}/healthz`)
  } catch {
    return 'down'
  }
  if (!res.ok) return 'down'
  let doc: { home?: string; pid?: number } = {}
  try {
    doc = (await res.json()) as { home?: string; pid?: number }
  } catch {
    /* a 200 that is not our JSON is an impostor by definition */
  }
  if (doc.home !== HOME) {
    throw new Error(
      `the server answering ${BASE}/healthz is not this spec's serve: its home is ${JSON.stringify(doc.home ?? '(no home field)')}` +
        (doc.pid ? `, pid ${doc.pid}` : '') +
        `; this spec's home is ${HOME}. Something else holds port ${PORT} — free it, or pin GADAK_E2E_BUILTIN_PORT on a port you own.`,
    )
  }
  return 'up'
}

test.beforeAll(async () => {
  PORT = await builtinServePort()
  BASE = `http://127.0.0.1:${PORT}`
  HOME = join(e2eDir(), '.tmp', `identity-ui-${PORT}`)
  rmSync(HOME, { recursive: true, force: true })
  mkdirSync(HOME, { recursive: true })
  cli('init', '--local', '--json')
  // The serve below starts with --no-sync, so the mirror must be brought up
  // before it (the built-in-attachments discipline): a serve over a home
  // whose mirror has never synced boots the web app into its first-run
  // dialog, and the Settings gear sits under that overlay — every test in
  // this file then times out clicking a covered button.
  cli('sync')
  serve = spawn(
    BIN,
    ['serve', '--addr', `127.0.0.1:${PORT}`, '--static', 'dist/app', '--no-open', '--no-sync'],
    { cwd: repoRoot(), env: { ...process.env, GADAK_HOME: HOME, GADAK_WORKSPACE: '' } },
  )
  captureServeOutput(serve)
  const deadline = Date.now() + 60_000
  for (;;) {
    if (serve.exitCode !== null) {
      throw new Error(
        `built-in serve on port ${PORT} exited early (code ${serve.exitCode}); its last output was:\n${serveLog}`,
      )
    }
    if ((await pollServeIdentity()) === 'up') break
    if (Date.now() > deadline) throw new Error(`built-in serve did not come up on ${PORT}`)
    // why: polling a socket that is not open yet has no event to await.
    await new Promise((r) => setTimeout(r, 200))
  }
})

test.afterAll(() => {
  serve?.kill('SIGTERM')
  rmSync(HOME, { recursive: true, force: true })
})

/** Boot the SPA on this spec's serve and open Settings → Workspaces; answers
 *  the dialog, with the Workspaces tab already selected. */
async function openWorkspaces(page: import('@playwright/test').Page) {
  await forceLocale(page, 'en')
  await page.goto(`${BASE}/`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await openServerSettings(page)
  const dialog = page.getByRole('dialog', { name: 'Settings' })
  await dialog.getByRole('tab', { name: 'Workspaces', exact: true }).click()
  await expect(dialog.getByTestId('workspaces-tab')).toBeVisible()
  return dialog
}

async function settingsDoc(): Promise<{ actor?: { slug?: string; name?: string; kind?: string } }> {
  const res = await fetch(`${BASE}/api/v1/issues/settings/`)
  expect(res.status, `GET settings/ on ${BASE}`).toBe(200)
  return (await res.json()) as { actor?: { slug?: string; name?: string; kind?: string } }
}

test.describe('declared name (GDK-1973)', () => {
  test('a typed name saves, round-trips the document, and survives a reload', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const dialog = await openWorkspaces(page)

    const identity = dialog.getByTestId('identity')
    await expect(identity).toBeVisible()
    await expect(identity.getByTestId('identity-name')).toHaveValue('')
    await identity.getByTestId('identity-name').fill('Dana Kim')
    await identity.getByTestId('identity-save').click()
    // The toast is the completion signal — and it must name the name.
    await expect(page.getByTestId('toast')).toContainText('Dana Kim')

    const doc = await settingsDoc()
    expect(doc.actor?.kind).toBe('person')
    expect(doc.actor?.name).toBe('Dana Kim')
    expect(doc.actor?.slug, `slug was ${JSON.stringify(doc.actor?.slug)}`).toMatch(/^person:/)

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    // The reload restores the dialog itself: the open state lives in the URL
    // hash (App reads the settings param back), tab included — so the gear
    // sits under the restored dialog and the reopen is a wait, not a click.
    const reopened = page.getByRole('dialog', { name: 'Settings' })
    await expect(reopened).toBeVisible()
    await expect(reopened.getByTestId('workspaces-tab')).toBeVisible()
    await expect(reopened.getByTestId('identity-name')).toHaveValue('Dana Kim')
    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('emptying the field clears the person block — the key leaves the document', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const dialog = await openWorkspaces(page)

    // Establish a name through the field first: this test must not lean on
    // the previous one's stored state.
    const identity = dialog.getByTestId('identity')
    await identity.getByTestId('identity-name').fill('To Be Cleared')
    await identity.getByTestId('identity-save').click()
    await expect(page.getByTestId('toast')).toContainText('To Be Cleared')

    // The emptied field is the clear verb. No toast names it (there is no
    // name to name), so the document is the assertion: the actor key goes
    // away entirely — the server never stores an empty person name.
    await identity.getByTestId('identity-name').fill('')
    await identity.getByTestId('identity-save').click()
    await expect.poll(() => settingsDoc().then((d) => d.actor ?? null)).toBeNull()

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    // Same restore as above: the dialog reopens from the hash, tab included.
    const reopened = page.getByRole('dialog', { name: 'Settings' })
    await expect(reopened).toBeVisible()
    await expect(reopened.getByTestId('workspaces-tab')).toBeVisible()
    await expect(reopened.getByTestId('identity-name')).toHaveValue('')
    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('a connected workspace shows no identity block — absent, not disabled', async ({ page }) => {
    // The suite's default serve: Jira origin, where the account is the
    // identity and the field has no verb. '/' is that serve (baseURL).
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('tab', { name: 'Workspaces', exact: true }).click()
    await expect(dialog.getByTestId('workspaces-tab')).toBeVisible()
    await expect(dialog.getByTestId('identity')).toHaveCount(0)
    expect(appConsoleErrors(errors)).toEqual([])
  })
})
