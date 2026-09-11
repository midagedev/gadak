import { spawn, execFileSync, type ChildProcess } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { builtinServePort, e2eDir, openServerSettings, repoRoot } from './helpers'

/**
 * GDK-1617: attachment bytes on a built-in workspace moved out of the
 * origin's database and into a content-addressed directory, streamed both
 * ways. Every other attachment spec runs against the connected fixture,
 * whose bytes are seeded into the byte cache by serve.sh — so nothing in
 * the suite ever exercised the built-in origin's own byte path, and the two
 * defects this round found there (a 502 on every view, GDK-1613; a
 * download truncated at exactly 64 MiB) were both found by hand.
 *
 * This spec runs a second serve on its own port over its own GADAK_HOME,
 * whose origin is the built-in tracker with a real PNG and a real h264 MP4
 * uploaded through the CLI. It asserts what a person actually sees: the
 * image decodes in the browser (naturalWidth, not "the request was 200"),
 * the video plays and seeks, and the bytes that come back are the bytes
 * that went in.
 *
 * Port: this spec's own, never derived from the suite's (GDK-1789). The
 * default is a free ephemeral port grabbed at run time
 * (helpers.ts builtinServePort); GADAK_E2E_BUILTIN_PORT pins one instead.
 * The port used to be suite-port+1 — a derivation, not an assignment — so a
 * parallel round on the neighbouring port silently held it, this spec's
 * serve failed to bind, and the healthz poll below adopted the neighbour:
 * all four tests then failed against a server that was never this spec's.
 * The poll now checks the answering server's identity (the healthz home
 * field, GDK-1555), so even a real collision fails in one named sentence.
 */

const BIN = join(e2eDir(), '.tmp', 'gadak')
let PORT = ''
let BASE = ''
let HOME = ''

const IMAGE = join(repoRoot(), 'examples', 'attachments', '10000.png')
// A real h264 file, not synthetic bytes: "the video element got a src" is
// not the claim — the claim is that it decodes and seeks.
const VIDEO = join(repoRoot(), 'docs', 'media', 'mcp.mp4')

let serve: ChildProcess | undefined
let issueKey = ''
// Last ~4 KiB of the serve's output. A serve that cannot bind exits with the
// reason on stderr; without this latch that reason is lost and the poll
// below reports the impostor that took the port instead (GDK-1789).
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

function sha256(path: string): string {
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}

/**
 * 'up' once this spec's own serve answers — proven by identity, not by 200.
 * Any other healthy server on the port (a neighbour suite's serve, a stray
 * listener) is named and refused; an unanswered poll is just 'down'.
 */
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
    // A 200 that is not our JSON: an impostor by definition.
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
  HOME = join(e2eDir(), '.tmp', `builtin-attach-${PORT}`)
  rmSync(HOME, { recursive: true, force: true })
  mkdirSync(HOME, { recursive: true })

  cli('init', '--local', '--json')
  const created = cli('create', 'attachment media probe')
  issueKey = created.split('\t')[0].trim()
  expect(issueKey, `create printed ${JSON.stringify(created)}`).toMatch(/^[A-Z]+-\d+$/)
  cli('attach', issueKey, IMAGE)
  cli('attach', issueKey, VIDEO)
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

async function openDetail(page: Page): Promise<ReturnType<Page['getByTestId']>> {
  await page.goto(`${BASE}/#/?issue=${issueKey}`)
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible({ timeout: 30_000 })
  return panel
}

test.describe('built-in workspace attachments (GDK-1617)', () => {
  test('an uploaded image decodes in the browser', async ({ page }) => {
    const panel = await openDetail(page)
    const thumb = panel.locator('button.group[aria-label^="Enlarge "]').first()
    await expect(thumb).toBeVisible({ timeout: 15_000 })

    const img = thumb.locator('img')
    // naturalWidth, not a 200: a 502 body and a broken image both "load".
    // Before GDK-1613 every built-in attachment answered 502 here.
    await expect
      .poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth), { timeout: 15_000 })
      .toBeGreaterThan(0)
    expect(await img.evaluate((el: HTMLImageElement) => el.naturalHeight)).toBeGreaterThan(0)

    // The enlarged view is the same bytes through the same route.
    await thumb.click()
    const viewer = page.getByRole('dialog')
    await expect(viewer).toBeVisible()
    const big = viewer.locator('img')
    await expect
      .poll(() => big.evaluate((el: HTMLImageElement) => el.naturalWidth), { timeout: 15_000 })
      .toBeGreaterThan(0)
  })

  test('an uploaded video plays and seeks', async ({ page }) => {
    const panel = await openDetail(page)
    const tile = panel.locator('button.group[aria-label^="Play "]').first()
    await expect(tile).toBeVisible({ timeout: 15_000 })
    await tile.click()

    const video = page.getByRole('dialog').locator('video')
    await expect(video).toBeVisible()

    // Metadata means the container parsed — readyState >= 1 (HAVE_METADATA).
    await expect
      .poll(() => video.evaluate((el: HTMLVideoElement) => el.readyState), { timeout: 20_000 })
      .toBeGreaterThan(0)
    const duration = await video.evaluate((el: HTMLVideoElement) => el.duration)
    expect(duration, 'the browser could not read a duration, so the file did not decode').toBeGreaterThan(1)

    // Seeking is the whole point of the Range work: jumping forward has to
    // land, which it cannot if the server answers 200 to every request.
    await video.evaluate((el: HTMLVideoElement) => {
      el.currentTime = el.duration / 2
    })
    // Wait on the completion itself, not on currentTime: assigning
    // currentTime updates it right away, so polling that value passes
    // while `seeking` is still true and proves nothing about whether any
    // bytes arrived. seeking going false is the browser saying the new
    // position is playable — which is what a 206 buys.
    await expect
      .poll(
        () =>
          video.evaluate((el: HTMLVideoElement) => (el.seeking ? -1 : el.currentTime)),
        { timeout: 20_000 },
      )
      .toBeGreaterThan(1)
  })

  test('the bytes served are the bytes uploaded, and Range is honoured', async ({ page }) => {
    const panel = await openDetail(page)
    const tile = panel.locator('button.group[aria-label^="Play "]').first()
    await expect(tile).toBeVisible({ timeout: 15_000 })
    await tile.click()
    const src = await page.getByRole('dialog').locator('video').getAttribute('src')
    expect(src).toBeTruthy()
    const url = new URL(src!, BASE).toString()

    const whole = await fetch(url)
    expect(whole.status).toBe(200)
    const got = Buffer.from(await whole.arrayBuffer())
    expect(createHash('sha256').update(got).digest('hex')).toBe(sha256(VIDEO))

    const part = await fetch(url, { headers: { Range: 'bytes=100-199' } })
    expect(part.status, 'a Range request must be answered with 206').toBe(206)
    expect(part.headers.get('content-range')).toBe(`bytes 100-199/${got.length}`)
    const slice = Buffer.from(await part.arrayBuffer())
    expect(slice.equals(got.subarray(100, 200))).toBe(true)

    // A revalidating browser must not get a 502 (GDK-1617 review finding 3).
    const etag = whole.headers.get('etag')
    expect(etag, 'no validator, so a second view can never be a 304').toBeTruthy()
    const again = await fetch(url, { headers: { 'If-None-Match': etag! } })
    expect(again.status).toBe(304)
  })

  /*
   * The settings panel's size number used to be the mirror's alone, and the
   * mirror is a cache. Moving attachment bytes into their own directory
   * took the largest thing in a workspace out of the only file that number
   * described — so what someone reads there had nothing to do with what
   * their workspace costs. The totals come from the origin, which is also
   * the only party that can answer when it is on another machine.
   */
  test('the uploaded bytes show up in settings', async ({ page }) => {
    const uploaded = readFileSync(IMAGE).length + readFileSync(VIDEO).length

    await page.goto(`${BASE}/#/`)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await openServerSettings(page)

    const row = page.getByTestId('runtime-attachments')
    await expect(row).toBeVisible({ timeout: 15_000 })
    // Two attachments, two distinct files, and a date — a panel that says
    // "0 B" for a workspace holding a video is the failure this pins.
    await expect(row).toContainText('2 attachments in 2 files')
    await expect(row).toContainText(/\d{4}-\d{2}-\d{2}/)
    await expect(page.getByTestId('runtime-origin')).toBeVisible()

    // The human string is rounded, so the number is checked over the API
    // the panel reads. Greater-or-equal: the fixture seed may add its own.
    const doc = await (await fetch(`${BASE}/api/v1/issues/settings/`)).json()
    expect(doc.runtime.attachmentsBytes).toBeGreaterThanOrEqual(uploaded)
    expect(doc.runtime.attachmentCount).toBe(2)
    expect(doc.runtime.attachmentsFileCount).toBe(2)
    expect(doc.runtime.attachmentsPath, 'the panel must name where the bytes are').toContain('blobs')
  })
})
