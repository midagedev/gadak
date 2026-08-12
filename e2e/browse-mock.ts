import type { Page } from '@playwright/test'

/*
 * The desktop app's /desktop/browse routes, in Chromium.
 *
 * The pane's whole job is chrome around a rectangle that a native WKWebView
 * fills — so the rectangle is the only part a browser cannot show, and every
 * other part is exactly what a browser is good at. Standing the four routes up
 * as a stateful mock gets the tab strip, the toolbar, the frame reports and the
 * ⌘W poll under test with no window, no signing and no Mac.
 *
 * Faithful to desktop/browse.go: ids are 1-based decimal strings, creating a
 * tab activates it, `open` is insertion order, and closing the visible tab
 * clears `active` rather than choosing a successor (the SPA chooses).
 */

export interface MockTab {
  id: string
  title: string
  url: string
}

export interface BrowseMock {
  /** Every id POSTed to /activate, in order. "" means "hide everything". */
  activates: string[]
  /** Every rectangle POSTed to /frame. */
  frames: { x: number; y: number; w: number; h: number }[]
  /** URLs handed to the system browser via /desktop/open. */
  opened: string[]
  /**
   * Targeted resyncs the pane asked for, as `<kind>:<key>`.
   *
   * Stubbed rather than left to the real endpoint on purpose: the fixture
   * credential is fake, so a live resync answers 5xx — which Chromium reports
   * as a console error and every spec here asserts against. The request being
   * made is the contract; what Jira would have said is not this suite's to know.
   */
  resynced: string[]
  /** Live tab list, as /state would answer right now. */
  tabs(): MockTab[]
  active(): string
  /** What ⌘W does: the native side closes a tab without telling the SPA. */
  closeNatively(id: string): void
}

export interface BrowseMockOptions {
  /** Page title the "loaded" tab reports. Defaults to a Jira-shaped one. */
  titleFor?: (url: string) => string
}

function defaultTitle(url: string): string {
  try {
    const u = new URL(url)
    const issue = u.pathname.match(/\/browse\/([A-Z][A-Z0-9]*-\d+)/)
    if (issue) return `[${issue[1]}] Jira`
    const wiki = u.pathname.match(/\/wiki\/spaces\/([^/]+)\/pages\/(\d+)/)
    if (wiki) return `${wiki[1]} page ${wiki[2]} - Confluence`
    return u.host
  } catch {
    return url
  }
}

/** Serve the config the desktop app serves: same document, plus `desktop`. */
export async function pretendDesktop(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = JSON.parse(await res.text())
    doc.desktop = true
    await route.fulfill({ response: res, body: JSON.stringify(doc) })
  })
}

/** Install the browse routes. Call before `gotoApp`. */
export async function mockBrowseRoutes(
  page: Page,
  opts: BrowseMockOptions = {},
): Promise<BrowseMock> {
  const titleFor = opts.titleFor ?? defaultTitle
  const tabs: MockTab[] = []
  let seq = 0
  let active = ''

  const mock: BrowseMock = {
    activates: [],
    frames: [],
    opened: [],
    resynced: [],
    tabs: () => tabs.map((t) => ({ ...t })),
    active: () => active,
    closeNatively(id) {
      const i = tabs.findIndex((t) => t.id === id)
      if (i < 0) return
      tabs.splice(i, 1)
      if (active === id) active = ''
    },
  }

  const json = (body: unknown, status = 200) => ({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })

  await page.route('**/desktop/browse', async (route) => {
    const { url } = route.request().postDataJSON() as { url?: string }
    if (!url || !/^https?:\/\//i.test(url)) {
      await route.fulfill(json({ error: 'refused' }, 400))
      return
    }
    seq += 1
    const id = String(seq)
    tabs.push({ id, title: titleFor(url), url })
    active = id
    await route.fulfill(json({ id }, 201))
  })

  await page.route('**/desktop/browse/state', async (route) => {
    await route.fulfill(json({ open: mock.tabs(), active }))
  })

  await page.route('**/desktop/browse/activate', async (route) => {
    const { id } = route.request().postDataJSON() as { id?: string }
    const next = id ?? ''
    if (next !== '' && !tabs.some((t) => t.id === next)) {
      await route.fulfill(json({ error: 'no such tab' }, 400))
      return
    }
    active = next
    mock.activates.push(next)
    await route.fulfill({ status: 204, body: '' })
  })

  await page.route('**/desktop/browse/close', async (route) => {
    const { id } = route.request().postDataJSON() as { id?: string }
    const i = tabs.findIndex((t) => t.id === id)
    if (i < 0) {
      await route.fulfill(json({ error: 'no such tab' }, 400))
      return
    }
    tabs.splice(i, 1)
    if (active === id) active = ''
    await route.fulfill({ status: 204, body: '' })
  })

  await page.route('**/desktop/browse/frame', async (route) => {
    mock.frames.push(route.request().postDataJSON())
    await route.fulfill({ status: 204, body: '' })
  })

  await page.route('**/desktop/open', async (route) => {
    const { url } = route.request().postDataJSON() as { url?: string }
    if (url) mock.opened.push(url)
    await route.fulfill({ status: 204, body: '' })
  })

  await page.route('**/resync/', async (route) => {
    const path = new URL(route.request().url()).pathname
    const page_ = path.match(/\/pages\/([^/]+)\/resync\/$/)
    const issue = path.match(/\/([^/]+)\/resync\/$/)
    mock.resynced.push(page_ ? `page:${page_[1]}` : `issue:${issue?.[1] ?? path}`)
    // `issue: null` is a well-formed IssueWriteResponse that changes no state —
    // what the mirror ends up holding is the resync endpoint's story, not this
    // one's, and the pane only ever forwards the answer.
    await route.fulfill(json({ issue: null }))
  })

  return mock
}
