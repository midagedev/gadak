/*
 * Share an issue key from the detail screen (GDK-877).
 *
 * Runes-free: this module builds the payload and picks the road; the screen
 * hands it the platform pieces (navigator.share when the webview has one,
 * the clipboard, the toast callbacks). Two roads, one payload:
 *
 *   navigator.share present → the OS share sheet (text = "<KEY> <summary>",
 *                              url = the origin's own page when the row has
 *                              an absolute one — items.url, GDK-1308's rule:
 *                              the built-in tracker's relative /browse/KEY
 *                              is not an origin page and stays out).
 *   absent                  → clipboard copy of the same words, announced
 *                              on the app toast host with the copy toast
 *                              the link taps already use (GDK-1504).
 *
 * The absolute boundary (DESIGN.md §5, lib/offer.ts): a pairing offer is a
 * credential and never reaches a share path. buildSharePayload is the
 * single door, and it refuses anything offer-shaped — a subject carrying
 * offer fields, or any string that decodes as an offer. The refusal is a
 * thrown error, not a stripped field, so a caller cannot half-share by
 * accident. The error text never quotes the input.
 */

import { decodeOffer } from './offer'

/** The row fields the share reads. `url` is the origin's own page when
 *  the mirror stored one (items.url, verbatim). */
export interface ShareSubject {
  issue_key: string
  summary: string
  url?: string | null
}

/** Web Share API shape (the subset the phone sends). */
export interface SharePayload {
  title: string
  text: string
  url?: string
}

export class ShareRefused extends Error {}

/** Issue key shape — the same regex adf-links.ts and deeplink.ts use:
 *  letters before the dash, digits after. */
const ISSUE_KEY = /^[A-Za-z][A-Za-z0-9]*-[0-9]+$/
const ABSOLUTE_HTTP = /^https?:\/\//i
/** Field names an offer document carries (offer.ts). A subject with any
 *  of these is not an issue — refuse before reading anything else. */
const OFFER_FIELDS = ['token', 'tokens', 'endpoint', 'expires_at', 'label', 'v'] as const
/** A whitespace-free run long enough to be a base64url offer document. The
 *  smallest valid v1 vector is well over this; issue keys and words are
 *  well under it, so scanning only these keeps the check cheap. */
const OFFER_BLOB = /[A-Za-z0-9_=-]{40,}/g

/** True when `s` — whole, or any base64url-looking run inside it — decodes
 *  as a pairing offer. A decode failure means "not an offer", nothing more. */
export function looksLikeOffer(s: string): boolean {
  const candidates = [s.trim(), ...(s.match(OFFER_BLOB) ?? [])]
  for (const c of candidates) {
    if (c === '') continue
    try {
      decodeOffer(c)
      return true
    } catch {
      // not an offer; keep looking
    }
  }
  return false
}

/**
 * The payload for one issue. Throws ShareRefused when the subject is not
 * an issue row (bad key shape, offer fields present) or when any string it
 * carries decodes as a pairing offer.
 */
export function buildSharePayload(subject: ShareSubject): SharePayload {
  if (typeof subject !== 'object' || subject === null) {
    throw new ShareRefused('share: not an issue')
  }
  const rec = subject as unknown as Record<string, unknown>
  for (const f of OFFER_FIELDS) {
    if (f in rec) throw new ShareRefused('share: refused — subject carries a credential field')
  }
  const key = String(rec.issue_key ?? '').trim()
  if (!ISSUE_KEY.test(key)) throw new ShareRefused('share: refused — not an issue key')
  const summary = String(rec.summary ?? '').trim()
  const url = typeof rec.url === 'string' ? rec.url.trim() : ''
  for (const s of [key, summary, url]) {
    if (looksLikeOffer(s)) throw new ShareRefused('share: refused — the text is a pairing offer')
  }
  if (url !== '' && !ABSOLUTE_HTTP.test(url) && !url.startsWith('/')) {
    // A non-http scheme (gadak://, javascript:) is never a share url.
    throw new ShareRefused('share: refused — the url is not an origin page')
  }
  const payload: SharePayload = {
    title: key,
    text: summary === '' ? key : `${key} ${summary}`,
  }
  if (ABSOLUTE_HTTP.test(url)) payload.url = url
  return payload
}

/** The clipboard form of a payload: the words, then the page on its own
 *  line when there is one — the desk's copy-link order. */
export function clipboardText(p: SharePayload): string {
  return p.url ? `${p.text}\n${p.url}` : p.text
}

export type ShareOutcome = 'shared' | 'cancelled' | 'copied' | 'copyFailed'

export interface ShareDeps {
  /** navigator.share bound to navigator, or undefined when the webview
   *  has none — feature detection is the caller's, so a test can stand in
   *  either platform. */
  share?: (data: SharePayload) => Promise<void>
  writeClipboard: (text: string) => Promise<void>
  onCopied: () => void
  onCopyFailed: () => void
}

/**
 * Shares the payload by the best road the platform offers. A share sheet
 * the person dismissed (AbortError) is 'cancelled' and announces nothing;
 * a share that failed for any other reason falls through to the clipboard
 * so the words still land somewhere.
 */
export async function shareIssue(payload: SharePayload, deps: ShareDeps): Promise<ShareOutcome> {
  if (deps.share) {
    try {
      await deps.share(payload)
      return 'shared'
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return 'cancelled'
      // fall through to the clipboard
    }
  }
  try {
    await deps.writeClipboard(clipboardText(payload))
    deps.onCopied()
    return 'copied'
  } catch {
    deps.onCopyFailed()
    return 'copyFailed'
  }
}
