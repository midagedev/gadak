// ADF link semantics for the phone (GDK-1497). The renderer
// (web/src/lib/adf.ts) emits plain HTML; what a tap on that HTML *means* is
// a phone decision, and it lives here as pure functions so the choice is
// testable without a WebView.
//
// Three meanings exist, in the delegation order AdfContent.svelte uses on
// the desktop (minus the ▶, which the phone never asks for):
//   1. an attachment trigger (image button carrying data-attachment-id)
//   2. an anchor to another issue in this workspace → open in-app detail
//   3. any other anchor → do NOT navigate (the phone has no opener); copy
//
// The renderer marks attachments with data-attachment-id but marks nothing
// on issue links — it has no notion of "this href is an issue". The one
// stable shape is the origin-tracker browse URL the renderer itself emits
// for unresolved media and that a person pastes into link marks:
// `https://<site>/browse/<KEY>`. Anchors that do not match stay in bucket 3.

/** What a tap inside an ADF body asks the app to do. */
export type AdfLinkAction =
  | { kind: 'issue'; key: string }
  | { kind: 'attachment'; id: string }
  | { kind: 'copy'; href: string }

/**
 * `<site>/browse/<KEY>` with optional query/fragment. The key shape is
 * Jira's (and the renderer's fixtures'): PROJECT-123, at least one letter
 * before the dash, digits after. Anything else — a /browse/ path with no
 * key, a key with spaces — is not an issue link and must not become one.
 */
const BROWSE_KEY = /^https?:\/\/[^/?#]+\/browse\/([A-Za-z][A-Za-z0-9]*-[0-9]+)(?:[?#][^\s]*)?$/

/** Classify an anchor's href. Exported for tests. */
export function classifyAdfHref(href: string | null): { kind: 'issue'; key: string } | { kind: 'copy'; href: string } | null {
  if (!href) return null
  const key = BROWSE_KEY.exec(href.trim())
  if (key) return { kind: 'issue', key: key[1] }
  // safeHref in the renderer already drops non-http(s) schemes; classify as
  // if it had not, so the fallback is still "copy", never "navigate".
  return { kind: 'copy', href }
}

/**
 * The tap target → action mapping, delegated from one click handler on the
 * body's root (the same shape as AdfContent.svelte: closest(), not per-node
 * listeners, because the HTML is injected as one string).
 *
 * Returns null when the tap is not on anything interactive — plain text,
 * whitespace, a checkbox glyph. The caller does nothing then.
 */
export function classifyAdfTarget(target: HTMLElement | null): AdfLinkAction | null {
  // Attachment images are buttons, not anchors: data-attachment-id is the
  // renderer's own marker and wins before any href logic.
  const trigger = target?.closest<HTMLElement>('[data-attachment-id]')
  if (trigger) {
    const id = trigger.dataset.attachmentId
    if (id) return { kind: 'attachment', id }
  }
  const anchor = target?.closest<HTMLAnchorElement>('a[href]')
  if (!anchor) return null
  return classifyAdfHref(anchor.getAttribute('href'))
}
