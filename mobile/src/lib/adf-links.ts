// ADF link semantics for the phone (GDK-1497, extended GDK-1503). The
// renderer (web/src/lib/adf.ts) emits plain HTML; what a tap on that HTML
// *means* is a phone decision, and it lives here as pure functions so the
// choice is testable without a WebView.
//
// Five meanings exist, in the delegation order AdfContent.svelte uses on the
// desktop (minus the ▶, which the phone never asks for):
//   1. an image attachment      → open the full-screen viewer (retry if the
//                                 bytes never arrived)
//   2. a video attachment       → fetch the bytes now and play
//   3. a file chip              → copy its absolute URL
//   4. an anchor to another issue in this workspace → open in-app detail
//   5. any other anchor         → do NOT navigate (the phone has no opener);
//                                 copy
//
// The renderer marks only images (`data-attachment-id` on the image button).
// Videos are a <figure> and file chips are anchors, so AdfBody's prime pass
// stamps `data-attachment-kind` on those two while it strips their eager
// src/href fetches — and this module turns that stamp into a decision. An
// unstamped trigger is the renderer's own image button, which is why the
// absent kind means image.
//
// For issue links the renderer marks nothing: it has no notion of "this href
// is an issue". The one stable shape is the origin-tracker browse URL the
// renderer itself emits for unresolved media and that a person pastes into
// link marks: `https://<site>/browse/<KEY>`. Anchors that do not match stay
// in bucket 5.

/** What a tap inside an ADF body asks the app to do. */
export type AdfLinkAction =
  | { kind: 'issue'; key: string }
  | { kind: 'image'; id: string }
  | { kind: 'video'; id: string }
  | { kind: 'file'; id: string; href: string }
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
 * whitespace, a checkbox glyph, or the <video> element once it is playing
 * (the prime pass drops the poster's stamp then, so the native controls get
 * the tap instead of a second fetch). The caller does nothing then.
 */
export function classifyAdfTarget(target: HTMLElement | null): AdfLinkAction | null {
  // Attachment triggers are marked, not inferred: data-attachment-id is the
  // renderer's own marker (image) or the prime pass's (video, file), and it
  // wins before any href logic.
  const trigger = target?.closest<HTMLElement>('[data-attachment-id]')
  if (trigger) {
    const id = trigger.dataset.attachmentId
    if (id) {
      const kind = trigger.dataset.attachmentKind
      if (kind === 'video') return { kind: 'video', id }
      if (kind === 'file') return { kind: 'file', id, href: trigger.getAttribute('href') ?? '' }
      // No stamp (or one this build does not know) is the image button. It
      // must never fall through to the anchor branch: an unknown stamp that
      // navigated would be a worse failure than an inert enlarge.
      return { kind: 'image', id }
    }
  }
  const anchor = target?.closest<HTMLAnchorElement>('a[href]')
  if (!anchor) return null
  return classifyAdfHref(anchor.getAttribute('href'))
}

/**
 * A byte count as the phone prints it on a video poster or a file chip
 * (GDK-1503).
 *
 * The same ladder the desktop gallery uses (web/src/components/detail/
 * AttachmentGallery.svelte) so one attachment reads identically on both
 * surfaces; it is duplicated rather than imported because that function is
 * a private helper inside a Svelte component with web-only imports.
 *
 * A missing size is the empty string, never "0 B": the server sends 0 for a
 * row it does not know, and printing it would be a claim about the file.
 */
export function formatAttachmentSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(Math.floor(Math.log(size) / Math.log(1024)), units.length - 1)
  const value = size / 1024 ** index
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}
