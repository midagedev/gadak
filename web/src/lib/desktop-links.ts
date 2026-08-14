/*
 * External links inside the desktop webview.
 *
 * wails v3's webview ships no new-window delegate, so a `target="_blank"`
 * click dies silently inside the app — the browser-tab behaviour every
 * external anchor here was written for. In desktop mode this intercepts those
 * clicks at the document and routes them:
 *
 *  - same origin as `config().jiraBaseUrl` → POST /desktop/browse, which opens
 *    a tab in the in-app browser pane (see browse.svelte)
 *  - anything else, or no jiraBaseUrl → POST /desktop/open (system browser)
 *
 * Only absolute http(s) anchors are taken: the SPA's own links are relative,
 * and same-app `_blank` anchors (attachment content) have no browser to go to
 * — the app has no TCP listener — so they are left to their own story.
 */

import { config, isDesktop } from './config'
import { browse } from './browse.svelte'

export type AtlassianLinkKind = 'issue' | 'page' | 'other'

export interface ClassifiedAtlassianLink {
  inApp: boolean
  kind: AtlassianLinkKind
  key: string | null
}

const ISSUE_KEY_RE = /^[A-Z][A-Z0-9]*-\d+$/
const BROWSE_PATH_RE = /\/browse\/([A-Za-z][A-Za-z0-9]*-\d+)(?:\/|$)/
const WIKI_PAGE_PATH_RE = /\/wiki\/spaces\/[^/]+\/pages\/(\d+)/

/** Issue key in a /browse/KEY path, uppercased. Null when the path is not one. */
export function extractBrowseKey(href: string): string | null {
  const m = href.match(BROWSE_PATH_RE)
  if (!m) return null
  const key = m[1].toUpperCase()
  return ISSUE_KEY_RE.test(key) ? key : null
}

/** Confluence content id in a /wiki/spaces/…/pages/ID path. */
export function extractWikiPageId(href: string): string | null {
  const m = href.match(WIKI_PAGE_PATH_RE)
  return m ? m[1] : null
}

/**
 * Classify an absolute Atlassian URL for in-app browse + optional resync.
 * Pure: no I/O, no config reads — callers pass jiraBaseUrl.
 */
export function classifyAtlassianLink(
  href: string,
  jiraBaseUrl: string | null,
): ClassifiedAtlassianLink {
  if (!jiraBaseUrl) {
    return { inApp: false, kind: 'other', key: null }
  }
  let url: URL
  let base: URL
  try {
    base = new URL(jiraBaseUrl)
    url = new URL(href)
  } catch {
    return { inApp: false, kind: 'other', key: null }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return { inApp: false, kind: 'other', key: null }
  }
  if (url.origin !== base.origin) {
    return { inApp: false, kind: 'other', key: null }
  }

  const issueKey = extractBrowseKey(url.pathname)
  if (issueKey) {
    return { inApp: true, kind: 'issue', key: issueKey }
  }

  const wikiId = extractWikiPageId(url.pathname)
  if (wikiId) {
    return { inApp: true, kind: 'page', key: wikiId }
  }

  return { inApp: true, kind: 'other', key: null }
}

function openSystemBrowser(url: string): void {
  void fetch('/desktop/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  })
}

/** In-app tab; on 400/503 (or any non-201) fall back to the system browser. */
async function openInAppBrowser(
  url: string,
  classified: ClassifiedAtlassianLink,
): Promise<void> {
  try {
    const res = await fetch('/desktop/browse', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    })
    if (res.status === 201) {
      const body = (await res.json()) as { id?: string }
      if (body.id) {
        browse.adopt(body.id, url, classified.kind, classified.key)
        return
      }
    }
  } catch {
    /* network / parse — fall through to system browser */
  }
  openSystemBrowser(url)
}

/**
 * Same-site Atlassian URL → in-app tab on desktop, system browser under serve.
 * Off-site URLs always go to the system browser (desktop) or a new tab (serve).
 */
export function openContainedUrl(url: string): void {
  const classified = classifyAtlassianLink(url, config().jiraBaseUrl || null)
  if (isDesktop()) {
    if (classified.inApp) void openInAppBrowser(url, classified)
    else openSystemBrowser(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

/** Install the interceptor; returns the uninstall function (noop off desktop). */
export function installDesktopLinkOpener(): () => void {
  if (!isDesktop()) return () => {}
  const onClick = (e: MouseEvent) => {
    if (e.defaultPrevented) return
    const anchor = (e.target as Element | null)?.closest?.('a[href]')
    if (!anchor) return
    const href = anchor.getAttribute('href') ?? ''
    if (!/^https?:\/\//i.test(href)) return
    e.preventDefault()
    const base = config().jiraBaseUrl || null
    const classified = classifyAtlassianLink(href, base)
    if (classified.inApp) {
      void openInAppBrowser(href, classified)
    } else {
      openSystemBrowser(href)
    }
  }
  document.addEventListener('click', onClick)
  return () => document.removeEventListener('click', onClick)
}
