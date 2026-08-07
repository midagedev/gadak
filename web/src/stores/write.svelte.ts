/*
 * Issue Navigator — write state/action store (contract: write frontend)
 *
 * Role:
 *  - Personal Jira credential state (configured/jira_email/token_hint) + load/save/delete.
 *  - **Optimistic** wrapper for write actions (transition · assignee · comment · create):
 *      ① Apply IssueLite to local pool (issues.pool) immediately → ② server call →
 *      ③ Confirm-replace with response issue (+ invalidate detail cache) →
 *      ④ On failure: restore snapshot + toast.
 *  - Dialog open state (credential settings / new issue) + global toast queue.
 *
 * Credential gate (identity = stored Jira credential):
 *  - No credential → open settings dialog + toast; retry is the user's job.
 *  - Server 409 credential_required → same settings dialog.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity'
import * as api from '../lib/api'
import { t } from '../lib/i18n'
import { ApiError } from '../lib/api'
import { issues } from './issues.svelte'
import { me } from './me.svelte'
import { appendComment, invalidate } from '../lib/detail-cache.svelte'
import { recordRecent } from '../lib/recency'
import { isHostedDemo } from '../lib/config'
import type {
  CommentMention,
  CreateIssuePayload,
  CreateMetaProject,
  DetailComment,
  EditMetaField,
  EditMetaResponse,
  IssueLite,
  JiraUser,
  Transition,
  UploadedAttachment,
  WriteMetaCache,
} from '../lib/types'
import * as db from '../lib/db'

const WRITE_META_MS = 15 * 60 * 1000 // write-meta reload interval

export type ToastKind = 'error' | 'info' | 'success'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const TOAST_MS = { error: 6000, info: 3000, success: 2500 }

class WriteStore {
  /* ── Credential state ── */
  configured = $state(false)
  jiraEmail = $state('')
  displayName = $state('')
  tokenHint = $state('')
  verifiedAt = $state<string | null>(null)
  /** True after loadCredential has finished once (dedupe probes). */
  credentialLoaded = $state(false)

  /* ── Write meta (local-first) ── transition map + create-meta. Hydrate on boot + 15m reload. */
  writeMetaTransitions = $state<Record<string, Record<string, Transition[]>>>({})
  writeMetaProjects = $state<CreateMetaProject[]>([])
  writeMetaUpdatedAt = $state<string | null>(null)
  writeMetaLoaded = $state(false)

  /* ── Dialogs ── */
  settingsOpen = $state(false)
  newIssueOpen = $state(false)

  /* ── Toast queue (shell renders) ── */
  toasts = $state<Toast[]>([])

  /**
   * Detail reload signal. Bump after a comment commits so DetailPanel re-reads
   *  the cache and swaps optimistic temps for real comments (cache hit, no round-trip).
   */
  detailNonce = $state(0)

  /** issueKey → optimistic comments (pre-server confirm). CommentList appends under detail.comments. */
  pendingComments = new SvelteMap<string, DetailComment[]>()

  /**
   * Hosted demo only: issue keys this session has "edited". The demo keeps the
   * optimistic update so the interaction is worth trying, but there is no server
   * behind it — so the count is surfaced (banner, detail panel) and nothing is
   * written to IndexedDB. A reload restores the snapshot, which is the honest
   * behavior and also the cheapest way to say "this was never saved".
   */
  demoEdits = new SvelteSet<string>()
  #demoNoticeShown = false

  /** issueKey → editable QA field meta (editmeta). Source for inline-edit dropdowns. */
  editMeta = new SvelteMap<string, EditMetaResponse['fields']>()
  #editMetaLoading = new Set<string>()

  /**
   * Reply-request bridge. CommentList's Reply button sets this; CommentComposer
   * (via effect) inserts the author mention and focuses. nonce detects re-requests
   * for the same target.
   */
  replyRequest = $state<{ issueKey: string; user: CommentMention; nonce: number } | null>(null)
  #replyNonce = 0

  /** Reply — ask the composer to insert the target author's mention. No-op without account_id. */
  requestReply(issueKey: string, user: CommentMention): void {
    if (!user.account_id || !user.display_name) return
    this.replyRequest = { issueKey, user, nonce: ++this.#replyNonce }
  }

  #toastId = 0
  #tmpId = 0
  #writeMetaTimer: ReturnType<typeof setInterval> | null = null

  /* ── Write meta (local-first) ── */

  /**
   * Boot sequence (parallel with issues.init):
   *  ① IndexedDB cache hydrate (0ms on revisit) ② network refresh ③ 15m reload loop.
   *  meta/write/ missing (404) / errors are quiet; components fall back to lazy GET.
   */
  async loadWriteMeta(): Promise<void> {
    if (!this.writeMetaLoaded) {
      try {
        const cached = await db.getWriteMeta()
        if (cached) {
          this.#applyWriteMeta(cached.transitions, cached.projects, cached.updated_at)
        }
      } catch {
        /* no/unusable cache — ignore */
      }
    }
    await this.#refreshWriteMeta()
    if (!this.#writeMetaTimer && typeof window !== 'undefined') {
      this.#writeMetaTimer = setInterval(() => void this.#refreshWriteMeta(), WRITE_META_MS)
    }
  }

  async #refreshWriteMeta(): Promise<void> {
    try {
      const m = await api.getWriteMeta()
      const projects = m.create_meta?.projects ?? []
      this.#applyWriteMeta(m.transitions ?? {}, projects, m.updated_at ?? null)
      // $state proxies cannot structured-clone into IndexedDB → store a plain snapshot.
      await db.putWriteMeta(
        $state.snapshot({
          key: 'write',
          transitions: this.writeMetaTransitions,
          projects: this.writeMetaProjects,
          updated_at: this.writeMetaUpdatedAt,
          cached_at: new Date().toISOString(),
        }) as WriteMetaCache,
      )
    } catch (e) {
      // 404 (not deployed) stays quiet — fallback path handles it. Warn on other errors.
      if (!(e instanceof ApiError && e.status === 404)) {
        console.warn('[write] meta/write 로드 실패(폴백 사용)', e)
      }
    }
  }

  #applyWriteMeta(
    transitions: Record<string, Record<string, Transition[]>>,
    projects: CreateMetaProject[],
    updatedAt: string | null,
  ): void {
    this.writeMetaTransitions = transitions
    this.writeMetaProjects = projects
    this.writeMetaUpdatedAt = updatedAt
    this.writeMetaLoaded = true
  }

  /** Workflow project key for an issue (same as backend project_of: issue_key prefix first). */
  projectOf(issue: IssueLite): string {
    const k = issue.issue_key
    if (k && k.includes('-')) return k.split('-')[0]
    return issue.source_project ?? ''
  }

  /** Local transition candidates for this issue. null when missing (component lazy-GET fallback). */
  transitionsFor(issue: IssueLite): Transition[] | null {
    const byStatus = this.writeMetaTransitions[this.projectOf(issue)]
    const list = byStatus?.[issue.status]
    return list && list.length ? list : null
  }

  /* ── Toasts ── */

  toast(message: string, kind: ToastKind = 'info'): void {
    const id = ++this.#toastId
    this.toasts = [...this.toasts, { id, kind, message }]
    setTimeout(() => this.dismissToast(id), TOAST_MS[kind])
  }

  dismissToast(id: number): void {
    this.toasts = this.toasts.filter((t) => t.id !== id)
  }

  /* ── Credentials ── */

  async loadCredential(): Promise<void> {
    try {
      const c = await api.getCredential()
      this.#applyCredential(c)
    } catch (e) {
      // Quiet on missing/unauthorized; log other failures.
      if (!(e instanceof ApiError && (e.status === 401 || e.status === 404))) {
        console.warn('[write] 자격증명 로드 실패', e)
      }
    } finally {
      this.credentialLoaded = true
    }
  }

  async saveCredential(jiraEmail: string, apiToken: string): Promise<{ ok: boolean; error?: string }> {
    try {
      const c = await api.saveCredential(jiraEmail, apiToken)
      this.#applyCredential(c)
      await me.refreshIdentity()
      this.toast(t('write.credSaved'), 'success')
      return { ok: true }
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : t('write.credSaveFailed')
      return { ok: false, error: msg }
    }
  }

  /** Clear local credential state (e.g. when identity disappears). */
  resetCredential(): void {
    this.configured = false
    this.jiraEmail = ''
    this.displayName = ''
    this.verifiedAt = null
    this.tokenHint = ''
    this.credentialLoaded = false
  }

  async deleteCredential(): Promise<void> {
    try {
      const c = await api.deleteCredential()
      this.#applyCredential(c)
      await me.refreshIdentity()
      this.toast(t('write.credDeleted'), 'info')
    } catch (e) {
      this.#handleError(e, t('write.credDeleteFailed'))
    }
  }

  #applyCredential(c: {
    configured: boolean
    jira_email: string
    display_name: string
    verified_at: string | null
    token_hint: string
  }): void {
    this.configured = c.configured
    this.jiraEmail = c.jira_email
    this.displayName = c.display_name
    this.verifiedAt = c.verified_at
    this.tokenHint = c.token_hint
    this.credentialLoaded = true
  }

  /* ── Dialogs ── */

  openSettings(): void {
    this.settingsOpen = true
  }
  closeSettings(): void {
    this.settingsOpen = false
  }
  openNewIssue(): void {
    // New issue also has to pass the write gate before opening.
    void this.#gateThen(() => {
      this.newIssueOpen = true
    })
  }
  closeNewIssue(): void {
    this.newIssueOpen = false
  }

  /* ── Write gate ── */

  /**
   * Ensure a write can proceed.
   *  - No credential → settings dialog + false.
   *  - Configured → true.
   */
  async ensureWritable(): Promise<boolean> {
    // The demo has no credential and cannot get one, but the write surfaces are
    // most of what there is to look at — so let them run locally instead of
    // sending visitors to a settings dialog that leads nowhere.
    if (isHostedDemo()) return true
    if (!this.credentialLoaded) await this.loadCredential()
    if (!this.configured) {
      this.toast(t('write.needToken'), 'info')
      this.openSettings()
      return false
    }
    return true
  }

  /**
   * Record a demo-local edit and explain it once. One toast per session: the
   * banner carries the running count, so repeating the message on every edit
   * would be noise, and saying nothing at all would be misleading.
   */
  #noteDemoEdit(key: string): void {
    this.demoEdits.add(key)
    if (!this.#demoNoticeShown) {
      this.#demoNoticeShown = true
      this.toast(t('app.demoWriteNotice'), 'info')
    }
  }

  /** Run fn only after the write gate passes. */
  async #gateThen(fn: () => void): Promise<void> {
    if (await this.ensureWritable()) fn()
  }

  bumpDetail(): void {
    this.detailNonce++
  }

  /* ── IssueLite local apply / persist ── */

  #applyIssue(issue: IssueLite | null | undefined): void {
    if (!issue || !issue.issue_key) return
    issues.pool.set(issue.issue_key, issue)
    void db.putIssues([issue]).catch(() => {})
  }

  /* ── Shared optimistic issue write ──
   * Apply patch to the pool immediately → call() → confirm with response issue →
   * restore snapshot on failure.
   */
  async #writeIssue(
    key: string,
    patch: Partial<IssueLite> | null,
    call: () => Promise<{ issue: IssueLite }>,
    failMsg: string,
  ): Promise<boolean> {
    if (!(await this.ensureWritable())) return false
    const snapshot = issues.pool.get(key)
    if (snapshot && patch) {
      issues.pool.set(key, { ...snapshot, ...patch })
    }
    // Demo: the optimistic patch is the whole result. Stop before the request —
    // catching the 501 afterwards would mean rolling back the change we want to
    // keep, and an error toast for something that worked as intended.
    if (isHostedDemo()) {
      this.#noteDemoEdit(key)
      return true
    }
    try {
      const res = await call()
      this.#applyIssue(res.issue)
      invalidate(key) // refresh detail (history, …) on next open
      return true
    } catch (e) {
      if (snapshot) issues.pool.set(key, snapshot) // rollback
      this.#handleError(e, failMsg)
      return false
    }
  }

  /* ── Status transition ── */

  async transition(key: string, tr: Transition): Promise<boolean> {
    const proj = this.projectOf(issues.pool.get(key) ?? ({ issue_key: key } as IssueLite))
    const ok = await this.#writeIssue(
      key,
      { status: tr.to_status, status_category: tr.to_category },
      () => api.doTransition(key, tr.id),
      t('write.transitionFailed'),
    )
    if (ok) recordRecent(`transition:${proj}`, tr.id) // record successes only
    return ok
  }

  /* ── Assignee ── */

  async assign(key: string, user: JiraUser | null): Promise<boolean> {
    const ok = await this.#writeIssue(
      key,
      {
        assignee: user ? user.display_name : null,
        assignee_email: user ? user.email || null : null,
      },
      () => api.setAssignee(key, user ? user.account_id : null),
      t('write.assignFailed'),
    )
    if (ok && user?.account_id) recordRecent('assignee', user.account_id) // record successes only
    return ok
  }

  /* ── QA field inline edit ── */

  /**
   * Edit-start gate: ensure writable + load this issue's editmeta (allowed values).
   * Call just before opening the edit UI. false = blocked (dialog/toast handled inside).
   */
  async ensureEditMeta(key: string): Promise<boolean> {
    if (!(await this.ensureWritable())) return false
    if (!this.editMeta.has(key) && !this.#editMetaLoading.has(key)) {
      this.#editMetaLoading.add(key)
      try {
        const res = await api.getEditMeta(key)
        this.editMeta.set(key, res.fields)
      } catch (e) {
        this.#handleError(e, t('write.editMetaFailed'))
        return false
      } finally {
        this.#editMetaLoading.delete(key)
      }
    }
    return true
  }

  /** Edit meta for a field on this issue. null if not loaded yet or not editable. */
  editFieldMeta(key: string, field: string): EditMetaField | null {
    return this.editMeta.get(key)?.[field] ?? null
  }

  /**
   * Change a QA field value (optimistic). ``patch`` is a partial IssueLite update so
   * display values (option labels · assignee name · version names, …) refresh immediately.
   * Server response's resynced IssueLite is the final confirmation.
   */
  async setField(
    key: string,
    field: string,
    value: string | string[] | null,
    patch: Partial<IssueLite>,
  ): Promise<boolean> {
    return this.#writeIssue(
      key,
      patch,
      () => api.setIssueField(key, field, value),
      t('write.fieldFailed'),
    )
  }

  /* ── Comments ── */

  /**
   * Post a comment. Optimistically push a temp into pendingComments (CommentList shows
   *  it immediately); on success drop the temp, append the real comment to cache, and
   *  re-read detail (nonce); on failure drop the temp. false → caller restores input text.
   */
  async submitComment(
    key: string,
    text: string,
    mentions: CommentMention[] = [],
    attachments: UploadedAttachment[] = [],
  ): Promise<boolean> {
    if (!(await this.ensureWritable())) return false
    const tmpId = `temp-${++this.#tmpId}`
    // Optimistic paint: mentions as plain text; attachments as a filename list
    // (real render after confirm).
    const attachNote = attachments.length
      ? `\n${attachments.map((a) => `📎 ${a.filename}`).join('\n')}`
      : ''
    const temp: DetailComment = {
      comment_id: tmpId,
      author: me.name,
      author_email: me.email,
      body: text + attachNote,
      raw_body: null,
      created_at: new Date().toISOString(),
    }
    this.#pushPending(key, temp)
    // Demo: leave the pending comment in place. It lives in memory only, so it
    // reads as posted until the visitor reloads — which is exactly true.
    if (isHostedDemo()) {
      this.#noteDemoEdit(key)
      return true
    }
    try {
      const res = await api.postComment(
        key,
        text,
        mentions,
        attachments.map((a) => a.id),
      )
      this.#removePending(key, tmpId)
      const real: DetailComment = {
        comment_id: res.comment.comment_id,
        author: res.comment.author ?? me.name,
        author_email: me.email,
        body: res.comment.body,
        raw_body: null,
        created_at: res.comment.created_at,
      }
      appendComment(key, real) // into cache → re-read shows the real comment
      this.#applyIssue(res.issue) // comment_count etc.
      this.bumpDetail() // DetailPanel reloads from cache
      return true
    } catch (e) {
      this.#removePending(key, tmpId)
      this.#handleError(e, t('write.commentFailed'))
      return false
    }
  }

  /**
   * Upload an attachment for a comment. Returns uploaded attachment meta on success,
   * null on failure (toast). Must pass the write gate; callers may upload files in parallel.
   */
  async uploadAttachment(key: string, file: File): Promise<UploadedAttachment[] | null> {
    if (!(await this.ensureWritable())) return null
    // Unlike a status change, an upload has no local stand-in: the bytes would
    // have to come back from a server to be rendered anywhere.
    if (isHostedDemo()) {
      this.toast(t('app.demoAttachDisabled'), 'info')
      return null
    }
    try {
      const res = await api.uploadCommentAttachment(key, file)
      return res.attachments
    } catch (e) {
      this.#handleError(e, t('write.attachFailed', { name: file.name }))
      return null
    }
  }

  pending(key: string): DetailComment[] {
    return this.pendingComments.get(key) ?? []
  }

  #pushPending(key: string, c: DetailComment): void {
    this.pendingComments.set(key, [...(this.pendingComments.get(key) ?? []), c])
  }

  #removePending(key: string, id: string): void {
    const list = this.pendingComments.get(key)
    if (!list) return
    const next = list.filter((c) => c.comment_id !== id)
    if (next.length) this.pendingComments.set(key, next)
    else this.pendingComments.delete(key)
  }

  /* ── Create issue ── */

  /** On success: add the new issue to the pool and return issue_key (dialog selects/closes). */
  async createIssue(payload: CreateIssuePayload): Promise<{ ok: boolean; key?: string; error?: string }> {
    if (!(await this.ensureWritable())) return { ok: false }
    try {
      const res = await api.createIssue(payload)
      this.#applyIssue(res.issue)
      // Success only: project/type/label recency (new-issue defaults + autocomplete personalization)
      recordRecent('create-project', payload.project_key)
      recordRecent(`create-type:${payload.project_key}`, payload.issue_type)
      for (const l of payload.labels ?? []) recordRecent('label', l)
      this.toast(t('write.issueCreated', { key: res.issue.issue_key }), 'success')
      return { ok: true, key: res.issue.issue_key }
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : t('write.createFailed')
      return { ok: false, error: msg }
    }
  }

  /* ── Error handling ── */

  #handleError(e: unknown, fallback: string): void {
    if (e instanceof ApiError) {
      if (e.code === 'credential_required') {
        // No token stored → settings dialog with "set token" copy
        this.configured = false
        this.toast(t('write.needToken'), 'info')
        this.openSettings()
        return
      }
      if (e.code === 'credential_rejected' || e.status === 401) {
        // Stored token expired/refused → settings with "replace token" copy
        this.configured = false
        this.toast(t('write.tokenRejected'), 'error')
        this.openSettings()
        return
      }
      this.toast(e.message || fallback, 'error')
      return
    }
    console.warn('[write]', fallback, e)
    this.toast(fallback, 'error')
  }
}

/** App-wide singleton. */
export const write = new WriteStore()
