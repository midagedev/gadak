/*
 * People-axis store — who is open in the right panel, and what they wrote.
 *
 * The person is held by account id when the directory has one, otherwise by
 * email. The comment request keys on `jira_account_id`, which is the same
 * identifier the mirror stores as `comments.author_id`. A member without one
 * is a person the mirror has never seen act, so the panel says so instead of
 * guessing by name: two people can share a display name, and a wrong match
 * here would attribute someone's words to someone else.
 *
 * Nothing is cached beyond the open person. The comment list is the one part of
 * this app that is a request rather than a pool read (bodies are not in the
 * pool), and a person is opened deliberately, one at a time.
 */

import * as api from '../lib/api'
import { ApiError } from '../lib/api'
import type { AuthorComment, Member } from '../lib/types'
import { issues } from './issues.svelte'
import { panel } from './panel.svelte'

/** How many of a person's comments the panel asks for. The server caps at 200;
 *  50 is a scroll or two, which is what "recent" means on this surface. */
const COMMENT_LIMIT = 50

class PersonStore {
  /** Open person's email (the member key), or null when the right panel is
   *  showing an issue, a document, or nothing. Read from the panel union — one
   *  detail surface at a time is that value's shape, not a rule this store has
   *  to re-apply on the way in. */
  #selectedEmail = $derived(panel.keyOf('person'))
  /** Comments by the open person, newest first. */
  comments = $state<AuthorComment[]>([])
  /** Full count for this author — usually larger than `comments.length`. */
  total = $state(0)
  /** Set while the request is in flight; the panel header renders regardless. */
  loading = $state(false)
  /** 'unlinked' = the member carries no account id, so there is nothing to ask for. */
  error = $state<null | 'network' | 'unlinked'>(null)

  /** Race guard — only the last load may write into the fields above. */
  #gen = 0

  constructor() {
    // Everything below the email is *this person's* — it has to go when the
    // panel moves to someone else, or to an issue, or to nothing at all. The
    // panel tells us it moved, whoever moved it; this store no longer has to be
    // told by every other store that can take the surface.
    panel.onLeave('person', () => this.#discard())
  }

  get selectedEmail(): string | null {
    return this.#selectedEmail
  }

  /** The open person's directory row, when the member set still has them. */
  get member(): Member | undefined {
    const id = this.selectedEmail
    return issues.memberOf(id) ?? issues.memberOfAccountId(id)
  }

  /** Open a person. `identity` is account id when known, else email. */
  select(identity: string): void {
    if (!identity || this.selectedEmail === identity) return
    panel.show('person', identity)
    this.comments = []
    this.total = 0
    this.error = null
    void this.#load(identity)
  }

  clear(): void {
    panel.close('person')
  }

  /** Drop what was loaded for whoever was open. */
  #discard(): void {
    this.#gen++ // invalidate anything in flight
    this.comments = []
    this.total = 0
    this.loading = false
    this.error = null
  }

  /** Retry the comment request for the open person. */
  reload(): void {
    const email = this.selectedEmail
    if (email) void this.#load(email)
  }

  async #load(identity: string): Promise<void> {
    const my = ++this.#gen
    const member = issues.memberOf(identity) ?? issues.memberOfAccountId(identity)
    const accountId = member?.jira_account_id || (identity.includes('@') ? '' : identity)
    if (!accountId) {
      this.error = 'unlinked'
      this.loading = false
      return
    }
    this.loading = true
    this.error = null
    try {
      const res = await api.getCommentsByAuthor(accountId, COMMENT_LIMIT)
      if (my !== this.#gen) return // stale
      this.comments = res.comments ?? []
      this.total = res.total
    } catch (e) {
      if (my !== this.#gen) return
      // A server too old for the endpoint (404) reads the same as one that is
      // unreachable: the panel keeps its header and offers a retry.
      console.warn('[person] 코멘트 로드 실패', e instanceof ApiError ? e.message : e)
      this.comments = []
      this.total = 0
      this.error = 'network'
    } finally {
      if (my === this.#gen) this.loading = false
    }
  }
}

export const person = new PersonStore()
