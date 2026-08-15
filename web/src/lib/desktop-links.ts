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
import {
  classifyAtlassianLink,
  extractBrowseKey,
  extractWikiPageId,
  type AtlassianLinkKind,
  type ClassifiedAtlassianLink,
} from './browse-classify'

// Re-exported for the omnibox and anything else that classified through this
// module; the implementations live in the leaf (see browse-classify.ts).
export {
  classifyAtlassianLink,
  extractBrowseKey,
  extractWikiPageId,
  type AtlassianLinkKind,
  type ClassifiedAtlassianLink,
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
