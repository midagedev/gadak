// GDK-1503 captures: the phone's three attachment affordances.
//
// A1 (GDK-1497) taught the phone to render ADF, and images arrive through
// requestBlob. Videos and file chips kept the renderer's relative src/href,
// which is a real URL only behind the dev proxy — in the packaged app a video
// never plays and a chip tap copies a path that resolves nowhere. This spec
// photographs the close: a video that waits to be asked, a chip that says
// what its tap does, and an image that opens large.
//
// Why the payload is intercepted: `examples/demo.db` carries three
// attachments and all three are image/png (measured:
// `sqlite3 examples/demo.db "select mime_type, count(*) from attachments
// group by 1"` → `image/png|3`). There is no video and no plain file on the
// fixture, and regenerating it is a different round's gate (`make
// demo-fixture`, CLAUDE.md). So the video and the file chip are added to the
// *response* — the same JSON shape internal/server/read.go emits, fields and
// all — while the image half of the capture stays entirely real.
//
// The assertions here are the contract, not the looks: a video must reach the
// DOM with no `src` (anything else is an eager fetch, which is the defect),
// and the chip must carry the stamp that makes its tap a copy.
import { expect, test, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { SERVE_ORIGIN, UI_ORIGIN } from '../playwright.config'

// The captures are env-gated (GDK-1570): the three tests are behaviour
// tests first — video without src, chip with the copy stamp, viewer opens —
// and photograph the close only when a vision round names A7_SHOT_DIR.
// e2e/capture-guard.unit.ts holds every CI spec to this shape.
async function shootA7(page: Page, name: string): Promise<void> {
  const dir = process.env.A7_SHOT_DIR
  if (!dir) return
  mkdirSync(dir, { recursive: true })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.screenshot({ path: join(dir, name), animations: 'disabled' })
  console.log(`[a7] shot ${join(dir, name)}`)
}

type Attachment = {
  id: string
  filename: string
  mime_type: string
  size: number
  media_id: string
  media_collection: string
  is_image: boolean
  is_video: boolean
  cache_status: string
  created_at: string | null
  content_url: string
}
type Comment = {
  comment_id?: string
  author?: string | null
  author_email?: string | null
  body?: string
  raw_body?: unknown
  created_at?: string | null
}
type Detail = {
  issue_key: string
  attachments?: Attachment[]
  comments?: Comment[]
}

/** The first issue on the fixture whose detail already carries an image attachment. */
async function issueWithImage(): Promise<{ key: string; image: Attachment }> {
  const boot = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)).json()) as {
    issues: Array<{ issue_key: string }>
  }
  for (let i = 0; i < boot.issues.length; i += 24) {
    const details = await Promise.all(
      boot.issues.slice(i, i + 24).map(async (lite) => {
        const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/${lite.issue_key}/detail/`)
        return (await res.json()) as Detail
      }),
    )
    for (const doc of details) {
      const image = (doc.attachments ?? []).find((a) => a.is_image)
      if (image) return { key: doc.issue_key, image }
    }
  }
  throw new Error('no image attachment on the fixture to capture')
}

/** An attachment row shaped exactly as internal/server/read.go:355-369 emits it. */
function attachmentRow(key: string, id: string, filename: string, mime: string, size: number): Attachment {
  return {
    id,
    filename,
    mime_type: mime,
    size,
    media_id: '',
    media_collection: '',
    is_image: mime.startsWith('image/'),
    is_video: mime.startsWith('video/'),
    cache_status: 'ready',
    created_at: null,
    content_url: `/api/v1/issues/${key}/attachments/${id}/content/`,
  }
}

/** A comment body whose media nodes resolve to the rows above by filename. */
function mediaComment(names: string[]): Comment {
  return {
    comment_id: 'gdk1503',
    author: 'Nari Cho',
    author_email: null,
    body: 'Recording and notes from the session:',
    created_at: new Date().toISOString(),
    raw_body: {
      type: 'doc',
      version: 1,
      content: [
        {
          type: 'paragraph',
          content: [{ type: 'text', text: 'Recording and notes from the session:' }],
        },
        ...names.map((alt) => ({
          type: 'mediaSingle',
          content: [{ type: 'media', attrs: { type: 'file', alt } }],
        })),
      ],
    },
  }
}

/**
 * Adds one video and one plain file to whatever the serve answers for `key`,
 * plus a comment that references them. Everything else in the response — the
 * real image attachment included — passes through untouched.
 */
async function withSyntheticMedia(page: Page, key: string): Promise<void> {
  await page.route(`**/api/v1/issues/${key}/detail/`, async (route) => {
    const res = await route.fetch()
    // A 304 carries no body to widen (the screen re-uses its cached copy).
    if (res.status() !== 200) {
      await route.fulfill({ response: res })
      return
    }
    const doc = (await res.json()) as Detail
    const video = attachmentRow(key, 'gdk1503v', 'standup-walkthrough.mp4', 'video/mp4', 18_350_080)
    const file = attachmentRow(key, 'gdk1503f', 'migration-notes.pdf', 'application/pdf', 214_016)
    doc.attachments = [...(doc.attachments ?? []), video, file]
    doc.comments = [
      ...(doc.comments ?? []),
      mediaComment([video.filename, file.filename]),
    ]
    await route.fulfill({ response: res, json: doc })
  })
}

/** Search → row → detail, the pane's own road to any key. */
async function openIssue(page: Page, key: string): Promise<void> {
  await page.goto('/')
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('nav.safe-bottom button.tab').nth(1).click()
  await page.locator('.pane:not(.off) input').first().fill(key)
  const row = page.locator('.pane:not(.off) button.row', { hasText: key }).first()
  await row.waitFor()
  await row.click()
  await page.locator('button.back').waitFor()
}

test('a video waits to be asked, and a file chip says it copies', async ({ page }) => {
  const { key } = await issueWithImage()
  console.log(`[a7] issue ${key} — one real image, one synthetic video, one synthetic file`)
  await withSyntheticMedia(page, key)
  await openIssue(page, key)

  const poster = page.locator('.adf-video-poster')
  await expect(poster).toBeVisible()
  // The contract, and the defect this round closes: the element the renderer
  // wrote must reach the DOM with no src at all. A src here is an eager
  // fetch — the packaged app's silent no-op, the dev proxy's hidden pass.
  const video = page.locator('.adf-media-video video')
  await expect(video).toHaveCount(1)
  expect(await video.getAttribute('src')).toBeNull()
  await expect(video).toBeHidden()
  // The size the server sent, printed on the invitation (18,350,080 B).
  await expect(poster).toContainText('18 MB')

  // The chip is stamped so its tap copies rather than follows.
  const chip = page.locator('a.adf-media[data-attachment-kind="file"]')
  await expect(chip).toHaveCount(1)
  await expect(chip).toHaveAttribute('title', /copies/i)
  await expect(chip).toContainText('migration-notes.pdf')

  await poster.scrollIntoViewIfNeeded()
  await shootA7(page, 'attach-video-chip.png')

  // What the chip's tap actually puts on the clipboard: an absolute URL,
  // not the path the renderer wrote. In dev the paired endpoint is empty and
  // the proxy is the page origin, so that origin is the one that resolves.
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], {
    origin: UI_ORIGIN,
  })
  await chip.click()
  await expect(page.locator('.copied')).toBeVisible()
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toBe(`${UI_ORIGIN}/api/v1/issues/${key}/attachments/gdk1503f/content/`)

  // The failure path, on the real serve: this synthetic id has no bytes
  // behind it, so the fetch 404s. The poster must survive and say so — the
  // one outcome worth engineering around is a <video> left with a dead src.
  await poster.click()
  await expect(poster).toHaveAttribute('data-state', 'failed')
  await expect(poster).toContainText(/retry/i)
  await expect(video).toBeHidden()
  expect(await video.getAttribute('src')).toBeNull()
})

test('a tapped video takes the bytes and plays them', async ({ page }) => {
  const { key } = await issueWithImage()
  await withSyntheticMedia(page, key)
  // The success half needs bytes to exist. The fixture has none, so the
  // content route answers with a body: this asserts the road (blob in,
  // poster out, controls on), not the codec, which is the platform's.
  await page.route(`**/attachments/gdk1503v/content/`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'video/mp4',
      body: Buffer.from('gdk1503-not-a-real-mp4'),
    })
  })
  await openIssue(page, key)

  const poster = page.locator('.adf-video-poster')
  await expect(poster).toBeVisible()
  await poster.click()
  await expect(poster).toHaveCount(0)
  const video = page.locator('.adf-media-video video')
  await expect(video).toBeVisible()
  await expect(video).toHaveAttribute('src', /^blob:/)
  // controls arrive with the bytes, never before them.
  expect(await video.getAttribute('controls')).not.toBeNull()
})

test('a loaded image opens the full-screen viewer', async ({ page }) => {
  const { key } = await issueWithImage()
  await openIssue(page, key)

  // The image is real: the blob arrives through requestBlob and the object
  // URL replaces the parked path. Wait for that swap, not for the element.
  // Which image the body shows first is the fixture's business — the name is
  // read off the element that is actually tapped, not off the row that was
  // searched for (NMB-110 carries several).
  const img = page.locator('.adf-media-image img').first()
  await expect(img).toHaveAttribute('src', /^blob:/)
  const name = (await img.getAttribute('alt')) ?? ''
  console.log(`[a7] image ${name} on ${key}`)
  await page.locator('.adf-media-image').first().click()

  const viewer = page.locator('[data-testid="attachment-viewer"]')
  await expect(viewer).toBeVisible()
  // The viewer shows the bytes the body already had — no second fetch.
  await expect(viewer.locator('img')).toHaveAttribute('src', /^blob:/)
  expect(await viewer.locator('img').getAttribute('src')).toBe(await img.getAttribute('src'))
  await expect(viewer).toContainText(name)
  await shootA7(page, 'attach-image-viewer.png')

  // Close returns to the body, not to the list.
  await viewer.locator('button.close').click()
  await expect(viewer).toBeHidden()
  await expect(page.locator('button.back')).toBeVisible()
})
