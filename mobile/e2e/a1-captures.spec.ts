// GDK-1497 A1 captures. The phone now renders ADF descriptions, comments,
// and wiki pages through the web's renderer (mobile/src/ui/AdfBody.svelte);
// this spec photographs that on the shared fixture (`gadak demo` on 7899,
// vite on 5182 — mobile/playwright.config.ts) for the lead's vision round.
//
// The picks are dynamic: the richest bodies in whatever the serve has, not
// hardcoded keys that rot with the fixture. The assertions here are not
// about looks (that judgement is the vision round's) — they pin that the
// ADF markup actually arrived, so a silent fall back to flattened text
// fails this spec instead of shipping a misleading capture.
import { expect, test } from './helpers'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { SERVE_ORIGIN } from '../playwright.config'

const SHOT_DIR = process.env.A1_SHOT_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '.shots')

type AdfNode = { type?: string; content?: AdfNode[] }
type IssueLite = { issue_key: string; comment_count: number }
type DetailDoc = {
  issue_key: string
  description_adf: AdfNode | null
  comments?: Array<{ raw_body?: AdfNode | null }>
  attachments?: Array<{ is_image: boolean }>
}
type PageDoc = { key: string; body_adf: AdfNode | null }

/** Distinct ADF node types under a node — the richness axis for the pick. */
function adfKinds(node: AdfNode | null | undefined, into = new Set<string>()): Set<string> {
  if (!node || typeof node !== 'object') return into
  if (node.type) into.add(node.type)
  for (const child of node.content ?? []) adfKinds(child, into)
  return into
}

/**
 * The most ADF-rich issue: description kinds first, then comment kinds,
 * then whether an image attachment exists (the phone's blob-load road —
 * the capture should exercise it when the fixture offers one). Every
 * detail is fetched — 534 loopback reads measured ~1s — because the lite
 * rows carry no ADF and comment_count proved to be no proxy for it: the
 * fixture's descriptions are all plain paragraphs, and the richness that
 * exists (a media node resolving to a PNG) hides in comments.
 */
async function richestIssue(): Promise<{
  key: string
  kinds: number
  commentKinds: number
  hasImage: boolean
}> {
  const boot = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/bootstrap/`)).json()) as {
    issues: IssueLite[]
  }
  const chunks: typeof boot.issues[] = []
  for (let i = 0; i < boot.issues.length; i += 24) chunks.push(boot.issues.slice(i, i + 24))
  let best = { key: '', kinds: 0, commentKinds: 0, hasImage: false }
  for (const chunk of chunks) {
    const details = await Promise.all(
      chunk.map(async (lite) => {
        const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/${lite.issue_key}/detail/`)
        return (await res.json()) as DetailDoc
      }),
    )
    for (const doc of details) {
      const kinds = adfKinds(doc.description_adf).size
      const commentKinds = doc.comments?.reduce(
        (n, c) => Math.max(n, adfKinds(c.raw_body).size),
        0,
      )
      const hasImage = (doc.attachments ?? []).some((a) => a.is_image)
      const better =
        kinds > best.kinds ||
        (kinds === best.kinds && commentKinds > best.commentKinds) ||
        (kinds === best.kinds && commentKinds === best.commentKinds && hasImage && !best.hasImage)
      if (better) best = { key: doc.issue_key, kinds, commentKinds, hasImage }
    }
  }
  if (!best.key) throw new Error('no issues on the fixture to capture')
  return best
}

/** The page with the most node kinds in its body (71 loopback reads). */
async function richestPage(): Promise<{ key: string; kinds: number }> {
  const list = (await (await fetch(`${SERVE_ORIGIN}/api/v1/issues/pages/`)).json()) as {
    pages: Array<{ key: string }>
  }
  const docs = await Promise.all(
    list.pages.map(async (p) => {
      const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/pages/${p.key}/`)
      return (await res.json()) as PageDoc
    }),
  )
  let chosen = docs[0]
  for (const doc of docs) {
    if (adfKinds(doc.body_adf).size > adfKinds(chosen.body_adf).size) chosen = doc
  }
  return { key: chosen.key, kinds: adfKinds(chosen.body_adf).size }
}

test('captures the ADF issue and page bodies for the vision round', async ({ page }) => {
  // Capture-only (v0.21 release audit, capture-hygiene finding): this whole
  // spec exists to photograph surfaces for a vision round. It runs when that
  // round asks by naming the directory, and is skipped otherwise.
  test.skip(!process.env.A1_SHOT_DIR, 'capture-only; set A1_SHOT_DIR to run')
  mkdirSync(SHOT_DIR, { recursive: true })
  const issue = await richestIssue()
  const wiki = await richestPage()
  // The report reads these: which keys, how rich.
  console.log(
    `[a1] issue ${issue.key} — ${issue.kinds} description node kinds, ${issue.commentKinds} in comments${issue.hasImage ? ', image attachment' : ''}`,
  )
  console.log(`[a1] page ${wiki.key} — ${wiki.kinds} body node kinds`)

  await page.goto('/')
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // Issue via search — the pane's own road to any key.
  const tabs = page.locator('nav.safe-bottom button.tab')
  await tabs.nth(1).click()
  await page.locator('.pane:not(.off) input').first().fill(issue.key)
  const row = page.locator('.pane:not(.off) button.row', { hasText: issue.key }).first()
  await row.waitFor()
  await row.click()
  await page.locator('button.back').waitFor()
  // The description section must hold rendered ADF, not flattened text —
  // the whole point of A1.
  const issueAdf = page.locator('.body .adf').first()
  await expect(issueAdf).toBeVisible()
  // Attachment images arrive as blob URLs after the fetch; let them (and
  // anything else in flight) settle before the photograph.
  await page.waitForLoadState('networkidle').catch(() => {})
  // animations: 'disabled' runs every CSS transition to its end frame first
  // — the first round shot the page detail mid slide-in (translated ~8%,
  // the list still showing through), which no judge can read.
  await page.screenshot({ path: join(SHOT_DIR, 'a1-issue.png'), fullPage: true, animations: 'disabled' })
  // The phone's Detail puts comments first and the description under its
  // own heading below them — say so in the log so the capture reads right.
  const descHeading = page.locator('.body h3', { hasText: /description/i })
  console.log(`[a1] description heading present: ${await descHeading.count()}`)
  // The screen scrolls inside its own pane, so fullPage stops at the
  // viewport and the description (below the comments) never reaches the
  // first shot. Scroll it into view and photograph that cut on its own.
  await descHeading.first().scrollIntoViewIfNeeded()
  await page.screenshot({ path: join(SHOT_DIR, 'a1-issue-description.png'), animations: 'disabled' })
  console.log(`[a1] shot ${join(SHOT_DIR, 'a1-issue-description.png')}`)
  console.log(`[a1] shot ${join(SHOT_DIR, 'a1-issue.png')}`)

  // Back, then to the Issues pane (back from a searched issue lands on the
  // Search pane, which has no scope button), and the page through Documents.
  await page.locator('button.back').first().click()
  await tabs.nth(0).click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('button.cancel').waitFor()
  await page.locator('.sheet .section', { hasText: 'Documents' }).waitFor()
  await page.locator('.sheet button.row', { hasText: 'Updated' }).click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })
  const docRow = page.locator(
    `.pane:not(.off) button.row[data-testid="doc-row"][data-doc-key="${wiki.key}"]`,
  )
  await docRow.waitFor()
  await docRow.click()
  await page.locator('.page-detail button.back').waitFor()
  await expect(page.locator('.page-detail .body .adf').first()).toBeVisible()
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.screenshot({ path: join(SHOT_DIR, 'a1-page.png'), fullPage: true, animations: 'disabled' })
  console.log(`[a1] shot ${join(SHOT_DIR, 'a1-page.png')}`)
})
