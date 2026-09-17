import { spawn, execFileSync, type ChildProcess } from 'node:child_process'
import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from './helpers'
import {
  appConsoleErrors,
  attachConsoleErrors,
  builtinServePort,
  e2eDir,
  forceLocale,
  repoRoot,
  searchInput,
} from './helpers'

/**
 * GDK-1982: on the built-in tracker's workflow, transition ids are
 * positional (1..n) and status ids are catalog ids, and where the two
 * number spaces overlap — transition 3 targeting Done (10003) while status
 * id 3 is a second In Progress — the transition the web and phone apps
 * picked from GET transitions/ came back as a 400. Done was the one
 * transition no UI could ever apply.
 *
 * The workflow here is the measured one: the default catalog beside a
 * migrated one (the cutover shape issuetap documents — "In Progress" as
 * both 3 and 10001), seeded through the product's own legacy-YAML path
 * (origin/issuetap.yaml seeds a fresh persist once, origin.go
 * LegacyYAMLRel) — not faked in the browser. beforeAll asserts the served
 * list really collides, so this spec cannot silently keep passing on a
 * workflow that stopped reproducing the defect.
 *
 * The journey is what a person performs: open the issue, use the status
 * control the app offers, choose Done. The write must land — no direct API
 * call stands in for the pick.
 *
 * Port: this spec's own, never derived from the suite's (GDK-1789) —
 * builtinServePort() pins GADAK_E2E_BUILTIN_PORT or grabs an ephemeral
 * one, and the healthz poll below proves the answering server's identity.
 */

const BIN = join(e2eDir(), '.tmp', 'gadak')
const KEY = 'STD-1'
let PORT = ''
let BASE = ''
let HOME = ''

// The default catalog (10000 To Do / 3 In Progress / 10003 Done) beside a
// migrated one (10001 In Progress / 10002 In Review) — five statuses, so
// an issue in To Do sees four transitions with positional ids 1..4, and
// transition id 3 (Done) collides with transition 4's target status id 3.
// The optional resolution screen on Done mirrors the seeded home
// (builtInFixture): Done is a close, and it must not block the pick.
const FIXTURE_YAML = `\
# GDK-1982 e2e: the cutover workflow — default catalog beside a migrated one.
projects:
  - id: "10000"
    key: STD
    name: Built-in
    type: software
    style: classic
statuses:
  - {id: "10000", name: To Do, category: new}
  - {id: "10001", name: In Progress, category: indeterminate}
  - {id: "10002", name: In Review, category: indeterminate}
  - {id: "10003", name: Done, category: done}
  - {id: "3", name: In Progress, category: indeterminate}
spaces:
  - id: "40000"
    key: LOC
    name: Local
    type: global
transitionScreens:
  - status: "10003"
    fields:
      resolution: {}
issues:
  - id: "1"
    key: STD-1
    summary: Transition id collides with a status id
    type: "10003"
    status: "10000"
    project: STD
`

type TransitionDoc = { id: string; to_id: string; to_status: string; to_category: string }

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

/** The transitions list the app itself will read — the document the pick
 *  is made from, and the proof this workflow still collides. */
async function transitionsDoc(): Promise<TransitionDoc[]> {
  const res = await fetch(`${BASE}/api/v1/issues/${KEY}/transitions/`)
  expect(res.status, `GET transitions/ on ${BASE}`).toBe(200)
  const body = (await res.json()) as { transitions?: TransitionDoc[] }
  return body.transitions ?? []
}

test.beforeAll(async () => {
  PORT = await builtinServePort()
  BASE = `http://127.0.0.1:${PORT}`
  HOME = join(e2eDir(), '.tmp', `builtin-transition-${PORT}`)
  rmSync(HOME, { recursive: true, force: true })
  mkdirSync(HOME, { recursive: true })
  // The legacy YAML must be in place before the origin is ever constructed:
  // init --local builds the persist, and a fresh persist seeds from this
  // sibling once (origin.go selectBuiltInSeed). One issue, To Do, on the
  // five-status cutover workflow above.
  mkdirSync(join(HOME, 'origin'), { recursive: true })
  writeFileSync(join(HOME, 'origin', 'issuetap.yaml'), FIXTURE_YAML)
  cli('init', '--local', '--json')
  // The serve below starts with --no-sync, so the mirror must be brought up
  // before it (the built-in-attachments discipline): a serve over a home
  // whose mirror has never synced boots the web app into its first-run
  // dialog, and the issue list under it never renders.
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

  // The reproduction's own guard (GDK-1982): transition id "3" must be
  // offered while a *different* transition targets status id "3" — that
  // overlap is the whole defect. A workflow that stops colliding must fail
  // here, loudly, with the measured list — not pass as a softer test.
  const list = await transitionsDoc()
  const byID = new Map(list.map((t) => [t.id, t]))
  const hitsStatus3 = list.filter((t) => t.to_id === '3').map((t) => t.id)
  if (!byID.has('3') || hitsStatus3.length === 0 || hitsStatus3.includes('3')) {
    throw new Error(
      `the built-in serve's workflow no longer collides on ${KEY}; the defect this spec reproduces needs transition id "3" and a different transition targeting status id "3". Measured list: ${JSON.stringify(list)}`,
    )
  }
})

test.afterAll(() => {
  serve?.kill('SIGTERM')
  rmSync(HOME, { recursive: true, force: true })
})

/** The row the server itself answers after the write — STD-1's lite out of
 *  bootstrap, re-read through the server (origin truth, not the optimistic
 *  patch the chip could have kept). */
async function servedCategory(): Promise<string | undefined> {
  const res = await fetch(`${BASE}/api/v1/issues/bootstrap/`)
  expect(res.status, `GET issues/bootstrap/ on ${BASE}`).toBe(200)
  const body = (await res.json()) as { issues?: { issue_key: string; status_category?: string }[] }
  return body.issues?.find((i) => i.issue_key === KEY)?.status_category
}

test.describe('transition id collision (GDK-1982)', () => {
  test('choosing Done from the status control lands the issue in a done-category status', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto(`${BASE}/`)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

    // Open STD-1 through the list (the write-through idiom), not the URL.
    const input = searchInput(page)
    await input.fill(KEY)
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: KEY })
      .first()
      .click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    const chip = panel.getByTestId('status-transition')
    await expect(chip).toBeVisible()
    await expect(chip).toContainText('To Do')

    await chip.click()
    // Done is transition id 3 — the id that collides with status id 3 on
    // this workflow. Two same-named In Progress options ride along, so the
    // pick is exact-matched on the unique name.
    const option = page.getByRole('option', { name: 'Done', exact: true })
    await expect(option).toBeVisible()
    await option.click()

    await expect(chip).toContainText('Done')
    await expect(chip).not.toContainText('To Do')
    await expect.poll(() => servedCategory()).toBe('done')

    expect(appConsoleErrors(errors)).toEqual([])
  })
})
