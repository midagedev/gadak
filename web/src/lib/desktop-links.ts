/*
 * External links inside the desktop webview.
 *
 * wails v3's webview ships no new-window delegate, so a `target="_blank"`
 * click dies silently inside the app — the browser-tab behaviour every
 * external anchor here was written for. In desktop mode this intercepts those
 * clicks at the document and routes them to the app's POST /desktop/open,
 * which opens the system browser (desktop/main.go).
 *
 * Only absolute http(s) anchors are taken: the SPA's own links are relative,
 * and same-app `_blank` anchors (attachment content) have no browser to go to
 * — the app has no TCP listener — so they are left to their own story.
 */

import { isDesktop } from './config'

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
    void fetch('/desktop/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: href }),
    })
  }
  document.addEventListener('click', onClick)
  return () => document.removeEventListener('click', onClick)
}
