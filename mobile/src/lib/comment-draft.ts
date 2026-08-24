/*
 * Comment draft store — the single owner of the mobile draft contract.
 *
 * Key: `gadak-mobile.comment-draft:<endpoint>:<issue_key>`. The paired
 * endpoint is the phone's scope equivalent of the web's workspace+site
 * (web/src/lib/storage.ts composeCommentDraftKey): re-pairing to another
 * home must never resurrect a draft into the wrong mirror.
 *
 * The contract (ux-report Q2; unsaved drafts are market complaint #6 —
 * the structural fix lives here, not in the screen):
 *   - save on every input; a blank body deletes the key
 *   - a send in flight does not touch the stored draft (an app kill
 *     mid-send loses nothing)
 *   - the draft is deleted only after the post succeeded — the ack
 *   - a failed post leaves the draft in place; the screen restores the
 *     textarea and says so
 *
 * Storage failures (private mode / quota) are swallowed: the draft is
 * best-effort and never a reason to block typing or sending.
 */

const PREFIX = 'gadak-mobile.comment-draft'

export function commentDraftKey(endpoint: string, issueKey: string): string {
  return `${PREFIX}:${endpoint}:${issueKey}`
}

export function readDraft(endpoint: string, issueKey: string): string {
  try {
    return localStorage.getItem(commentDraftKey(endpoint, issueKey)) ?? ''
  } catch {
    return ''
  }
}

/** Save-on-input: a whitespace-only body deletes the key (web parity). */
export function saveDraft(endpoint: string, issueKey: string, body: string): void {
  const key = commentDraftKey(endpoint, issueKey)
  try {
    if (body.trim() !== '') localStorage.setItem(key, body)
    else localStorage.removeItem(key)
  } catch {
    /* best-effort */
  }
}

/** The success-ack delete — the only path that removes a sent draft. */
export function clearDraft(endpoint: string, issueKey: string): void {
  try {
    localStorage.removeItem(commentDraftKey(endpoint, issueKey))
  } catch {
    /* best-effort */
  }
}

export type DraftSendResult<T> = { ok: true; value: T } | { ok: false; error: unknown }

/**
 * Runs a comment post under the draft contract above: `post` is the api.ts
 * wrapper (the origin write — this module never transports anything), the
 * stored draft survives until it resolves, and only a success clears it.
 * The screen optimistically empties its textarea and restores it on
 * `{ ok: false }`; storage agrees because nothing was cleared.
 */
export async function sendWithDraft<T>(
  endpoint: string,
  issueKey: string,
  body: string,
  post: (body: string) => Promise<T>,
): Promise<DraftSendResult<T>> {
  saveDraft(endpoint, issueKey, body)
  try {
    const value = await post(body)
    clearDraft(endpoint, issueKey)
    return { ok: true, value }
  } catch (error) {
    return { ok: false, error }
  }
}
