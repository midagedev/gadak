/*
 * People-axis store — who is open in the right panel, and what they wrote.
 *
 * The person is held by email because that is what keys `issues.members`, the
 * directory the client already has; the comment request keys on the account id
 * that member carries (`jira_account_id`), which is the same identifier the
 * mirror stores as `comments.author_id`. A member without one is a person the
 * mirror has never seen act, so the panel says so instead of guessing by name:
 * two people can share a display name, and a wrong match here would attribute
 * someone's words to someone else.
 *
 * Nothing is cached beyond the open person. The comment list is the one part of
 * this app that is a request rather than a pool read (bodies are not in the
 * pool), and a person is opened deliberately, one at a time.
 */

import * as api from '../lib/api'
import { ApiError } from '../lib/api'
import type { AuthorComment, Member } from '../lib/types'
import { issues } from './issues.svelte'
import { pages } from './pages.svelte'
import { selection } from './selection.svelte'

/** How many of a person's comments the panel asks for. The server caps at 200;
 *  50 is a scroll or two, which is what "recent" means on this surface. */
const COMMENT_LIMIT = 50

class PersonStore {
  /** Open person's email (the member key), or null. */
  selectedEmail = $state<string | null>(null)
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

  /** The open person's directory row, when the member set still has them. */
  get member(): Member | undefined {
    return issues.memberOf(this.selectedEmail)
  }

  /** Open a person. Takes the panel from an issue or a document — one detail
   *  surface at a time, the same rule pages.select() follows. */
  select(email: string): void {
    if (this.selectedEmail === email) return
    selection.clear()
    pages.clear()
    this.selectedEmail = email
    this.comments = []
    this.total = 0
    this.error = null
    void this.#load(email)
  }

  clear(): void {
    if (this.selectedEmail === null) return
    this.#gen++ // invalidate anything in flight
    this.selectedEmail = null
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

  async #load(email: string): Promise<void> {
    const my = ++this.#gen
    const accountId = issues.memberOf(email)?.jira_account_id
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
