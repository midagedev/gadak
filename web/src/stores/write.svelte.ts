/*
 * Issue Navigator — write state/action store (contract: write frontend)
 *
 * Role:
 *  - Personal Jira credential state (configured/jira_email/token_hint) + load/save/delete.
 *  - **Optimistic** wrapper for write actions (transition · assignee · labels · priority · summary · comment · create):
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
import { writeErrorMessage } from '../lib/i18n/en'
import { ApiError } from '../lib/api'
import { issues } from './issues.svelte'
import { me } from './me.svelte'
import { appendComment, invalidate } from '../lib/detail-cache.svelte'
import { externalMoves } from '../lib/board-moves.svelte'
import { recordRecent } from '../lib/recency'
import { isHostedDemo, originTrackerName } from '../lib/config'
import { issueOriginUrl } from '../lib/issue-origin'
import type {
  CommentMention,
  CreateIssuePayload,
  CreateMetaProject,
  DetailComment,
  EditMetaField,
  EditMetaResponse,
  IssueLite,
  JiraUser,
  PageDetail,
  PriorityOption,
  Transition,
  UploadedAttachment,
  WriteMetaCache,
} from '../lib/types'
import * as db from '../lib/db'

const WRITE_META_MS = 15 * 60 * 1000 // write-meta reload interval

export type ToastKind = 'error' | 'info' | 'success'

export interface ToastAction {
  label: string
  /** ToastHost calls openIssueOrigin — write must not import desktop-links. */
  openIssueKey: string
}

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  action?: ToastAction
}

const TOAST_MS = { error: 6000, info: 3000, success: 2500 }

class WriteStore {
  /* ── Credential state ── */
  configured = $state(false)
  jiraEmail = $state('')
  displayName = $state('')
  tokenHint = $state('')
  verifiedAt = $state<string | null>(null)
  /** Profile has a Linear block (GET credential/.linear). */
  linear = $state(false)
  /** True after loadCredential has finished once (dedupe probes). */
  credentialLoaded = $state(false)

  /* ── Write meta (local-first) ── transition map + create-meta. Hydrate on boot + 15m reload. */
  writeMetaTransitions = $state<Record<string, Record<string, Transition[]>>>({})
  writeMetaProjects = $state<CreateMetaProject[]>([])
  writeMetaUpdatedAt = $state<string | null>(null)
  writeMetaLoaded = $state(false)

  /** Site priority catalog (GET priorities/), most urgent first. Create dialog. */
  priorities = $state<PriorityOption[]>([])
  prioritiesLoaded = $state(false)
  /** Per-key catalogs (GET {key}/priorities/). Open-issue picker. */
  #priorityByKey = new SvelteMap<string, PriorityOption[]>()

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

  toast(message: string, kind: ToastKind = 'info', action?: ToastAction): void {
    const id = ++this.#toastId
    const next: Toast = { id, kind, message }
    if (action) next.action = action
    this.toasts = [...this.toasts, next]
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

  async saveCredential(
    jiraEmail: string,
    apiToken: string,
    tokenExpiresAt?: string,
  ): Promise<{ ok: boolean; error?: string }> {
    try {
      const c = await api.saveCredential(jiraEmail, apiToken, tokenExpiresAt)
      this.#applyCredential(c)
      await me.refreshIdentity()
      this.toast(t('write.credSaved'), 'success')
      return { ok: true }
    } catch (e) {
      if (e instanceof ApiError) {
        console.warn('[write] saveCredential', e.code ?? e.message, e)
        return { ok: false, error: writeErrorMessage(e.code, t('write.credSaveFailed'), t) }
      }
      return { ok: false, error: t('write.credSaveFailed') }
    }
  }

  /** Clear local credential state (e.g. when identity disappears). */
  resetCredential(): void {
    this.configured = false
    this.jiraEmail = ''
    this.displayName = ''
    this.verifiedAt = null
    this.tokenHint = ''
    this.linear = false
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
    linear?: boolean
  }): void {
    this.configured = c.configured
    this.jiraEmail = c.jira_email
    this.displayName = c.display_name
    this.verifiedAt = c.verified_at
    this.tokenHint = c.token_hint
    this.linear = Boolean(c.linear)
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
   * Per-key write gate. Linear rows use credential.linear (no Jira token
   * dialog). Create / global pickers keep ensureWritable() — Jira only.
   */
  async ensureWritableFor(key: string): Promise<boolean> {
    if (isHostedDemo()) return true
    if (!this.credentialLoaded) await this.loadCredential()
    if (issues.pool.get(key)?.source === 'linear') {
      if (this.linear) return true
      this.toast(t('write.needToken'), 'info')
      this.openSettings()
      return false
    }
    return this.ensureWritable()
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
    onError?: (e: unknown) => void,
  ): Promise<boolean> {
    if (!(await this.ensureWritableFor(key))) return false
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
      if (onError) onError(e)
      else this.#handleError(e, failMsg)
      return false
    }
  }

  /* ── Status transition ── */

  async transition(
    key: string,
    tr: Transition,
    fields?: Record<string, unknown>,
  ): Promise<boolean> {
    const proj = this.projectOf(issues.pool.get(key) ?? ({ issue_key: key } as IssueLite))
    const failMsg = t('write.transitionFailed')
    // Before the patch, not after the confirm: the mirror's echo of this write
    // can land in the gap and must not read as somebody else's move (GDK-1176).
    externalMoves.noteSelf(key)
    const ok = await this.#writeIssue(
      key,
      { status: tr.to_status, status_category: tr.to_category },
      () => api.doTransition(key, tr.id, fields),
      failMsg,
      (e) => {
        if (e instanceof ApiError && e.status === 400) {
          this.toast(formatWriteRejection(e, failMsg), 'error', originAction(key))
          return
        }
        this.#handleError(e, failMsg)
      },
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
        assignee_id: user ? user.account_id : null,
        assignee_email: user ? user.email || null : null,
      },
      () => api.setAssignee(key, user ? user.account_id : null),
      t('write.assignFailed'),
    )
    if (ok && user?.account_id) recordRecent('assignee', user.account_id) // record successes only
    return ok
  }

  /* ── Labels ── */

  /**
   * Replace the issue's label array (optimistic). Empty clears. Trim and
   * de-dupe match the server so the chip row does not flash a form the PUT
   * would have dropped.
   */
  async setLabels(key: string, labels: string[]): Promise<boolean> {
    const next = normalizeLabels(labels)
    const prev = issues.pool.get(key)?.labels ?? []
    const ok = await this.#writeIssue(
      key,
      { labels: next },
      () => api.setLabels(key, next),
      t('write.labelsFailed'),
    )
    if (ok) {
      for (const l of next) {
        if (!prev.includes(l)) recordRecent('label', l)
      }
    }
    return ok
  }

  /* ── Priority ── */

  /**
   * Site catalog, most urgent first. Hosted demo synthesizes from the pool so
   * the picker still opens; a real site is one GET /priority.
   */
  async loadPriorities(): Promise<boolean> {
    if (this.prioritiesLoaded && this.priorities.length) return true
    if (!(await this.ensureWritable())) return false
    if (isHostedDemo()) {
      this.priorities = demoPriorities(issues.allIssues)
      this.prioritiesLoaded = true
      return true
    }
    try {
      const res = await api.getPriorities()
      this.priorities = res.priorities
      this.prioritiesLoaded = true
      return true
    } catch (e) {
      this.#handleError(e, t('write.prioritiesFailed'))
      return false
    }
  }

  async loadPrioritiesFor(key: string): Promise<boolean> {
    if (this.#priorityByKey.has(key)) return true
    if (!(await this.ensureWritableFor(key))) return false
    if (isHostedDemo()) {
      this.#priorityByKey.set(key, demoPriorities(issues.allIssues))
      return true
    }
    try {
      const res = await api.getPrioritiesFor(key)
      this.#priorityByKey.set(key, res.priorities)
      return true
    } catch (e) {
      this.#handleError(e, t('write.prioritiesFailed'))
      return false
    }
  }

  prioritiesFor(key: string): PriorityOption[] {
    return this.#priorityByKey.get(key) ?? []
  }

  hasPrioritiesFor(key: string): boolean {
    return this.#priorityByKey.has(key)
  }

  async setPriority(key: string, p: PriorityOption | null): Promise<boolean> {
    const catalog = this.#priorityByKey.get(key) ?? this.priorities
    const rank = !p ? 0 : p.id === '0' ? 0 : catalog.findIndex((x) => x.id === p.id) + 1
    return this.#writeIssue(
      key,
      { priority: p ? p.name : null, priority_rank: p ? rank : 0 },
      () => api.setPriority(key, p ? p.id : null),
      t('write.priorityFailed'),
    )
  }

  async setSummary(key: string, summary: string): Promise<boolean> {
    const next = summary.trim()
    if (!next) return false
    return this.#writeIssue(
      key,
      { summary: next },
      () => api.setSummary(key, next),
      t('write.summaryFailed'),
    )
  }

  /**
   * Replace the description as plain text. `null`/blank clears. No IssueLite
   * patch — description_adf is not on the row, and the client must not forge
   * ADF; #writeIssue's invalidate() re-reads detail the same way setSummary
   * does. In-place section — no success toast (GDK-301).
   */
  async setDescription(key: string, text: string | null): Promise<boolean> {
    const next = text == null ? null : text.trim() || null
    return this.#writeIssue(
      key,
      null,
      () => api.setDescription(key, next),
      t('write.descriptionFailed'),
    )
  }

  /* ── QA field inline edit ── */

  /**
   * Edit-start gate: ensure writable + load this issue's editmeta (allowed values).
   * Call just before opening the edit UI. false = blocked (dialog/toast handled inside).
   * `{ quiet: true }` skips the write-gate dialog and failure toast — used to
   * prefetch so empty editable rows can appear without forging editability.
   */
  async ensureEditMeta(key: string, opts?: { quiet?: boolean }): Promise<boolean> {
    if (isHostedDemo()) return false
    if (opts?.quiet) {
      if (!this.credentialLoaded) await this.loadCredential()
      if (issues.pool.get(key)?.source === 'linear') {
        if (!this.linear) return false
      } else if (!this.configured) {
        return false
      }
    } else if (!(await this.ensureWritableFor(key))) {
      return false
    }
    if (!this.editMeta.has(key) && !this.#editMetaLoading.has(key)) {
      this.#editMetaLoading.add(key)
      try {
        const res = await api.getEditMeta(key)
        this.editMeta.set(key, res.fields)
      } catch (e) {
        if (!opts?.quiet) this.#handleError(e, t('write.editMetaFailed'))
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

  /**
   * Set or clear the system due date. `iso` is YYYY-MM-DD; null clears.
   * In-place row update — no success toast (GDK-301).
   */
  async setDuedate(key: string, iso: string | null): Promise<boolean> {
    if (iso && !dueDateLiteral(iso)) return false
    return this.#writeIssue(
      key,
      { duedate: iso },
      () => api.setDuedate(key, iso),
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
    if (!(await this.ensureWritableFor(key))) return false
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
      // Same class as create: the new row is not guaranteed to stay on
      // screen (QuickComment closes; the thread may sit below the fold).
      this.toast(t('write.commentPosted', { key }), 'success')
      return true
    } catch (e) {
      this.#removePending(key, tmpId)
      this.#handleError(e, t('write.commentFailed'))
      return false
    }
  }

  /**
   * Post a wiki-page comment through the origin (GDK-381 / GDK-637).
   * Failures take the same #handleError path as issue comments (catalog
   * toast, never the wire message). Success returns the refreshed page so
   * the panel can overlay the thread; the hosted demo succeeds with no page.
   */
  async submitPageComment(
    pageId: string,
    text: string,
  ): Promise<{ ok: boolean; page?: PageDetail }> {
    const body = text.trim()
    if (!body) return { ok: false }
    if (!(await this.ensureWritable())) return { ok: false }
    if (isHostedDemo()) {
      this.#noteDemoEdit(pageId)
      return { ok: true }
    }
    try {
      const res = await api.commentOnPage(pageId, body)
      return { ok: true, page: res.page }
    } catch (e) {
      this.#handleError(e, t('write.commentFailed'))
      return { ok: false }
    }
  }

  /**
   * Upload an attachment for a comment. Returns uploaded attachment meta on success,
   * null on failure (toast). Must pass the write gate; callers may upload files in parallel.
   */
  async uploadAttachment(key: string, file: File): Promise<UploadedAttachment[] | null> {
    if (!(await this.ensureWritableFor(key))) return null
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

  /**
   * Palette instant-create: summary only. Same createIssue path as the form;
   * project/type are resolved server-side (internal/create).
   */
  async createFromSummary(summary: string): Promise<{ ok: boolean; key?: string; error?: string }> {
    const s = summary.trim()
    if (!s) return { ok: false, error: t('write.titleRequired') }
    return this.createIssue({ summary: s })
  }

  /** On success: add the new issue to the pool and return issue_key (dialog selects/closes). */
  async createIssue(payload: CreateIssuePayload): Promise<{ ok: boolean; key?: string; error?: string }> {
    if (!(await this.ensureWritable())) return { ok: false }
    const summary = payload.summary.trim()
    if (!summary) return { ok: false, error: t('write.titleRequired') }
    const body = compactCreatePayload({ ...payload, summary })
    if (body.duedate && !dueDateLiteral(body.duedate)) {
      return { ok: false, error: `duedate "${body.duedate}" is not a date (want YYYY-MM-DD)` }
    }
    // Which optional fields were left off — empty string is "omit", not "set empty".
    console.info('[write] create request', { body, omitted: omittedCreateFields(body) })
    try {
      const res = (await api.createIssue(body)) as CreateWriteResponse
      this.#applyIssue(res.issue)
      // Recency keys on ids, never display names (type id, not issue_type).
      const project =
        typeof body.project_key === 'string' && body.project_key
          ? body.project_key
          : this.projectOf(res.issue)
      const typeId =
        typeof body.issue_type === 'string' && body.issue_type
          ? body.issue_type
          : (res.issue.issue_type_id ?? '')
      if (project) recordRecent('create-project', project)
      if (project && typeId) recordRecent(`create-type:${project}`, typeId)
      const labels = Array.isArray(body.labels) ? body.labels.filter((l) => typeof l === 'string') : []
      for (const l of labels) recordRecent('label', l)
      this.toast(createdToast(res.issue), 'success')
      console.info('[write] create result', {
        key: res.issue.issue_key,
        type: res.issue.issue_type,
        project: res.issue.source_project,
        resolved: res.resolved,
      })
      return { ok: true, key: res.issue.issue_key }
    } catch (e) {
      if (e instanceof ApiError) {
        console.warn('[write] create', e.code ?? e.message, e)
        return { ok: false, error: formatWriteRejection(e, t('write.createFailed')) }
      }
      return { ok: false, error: t('write.createFailed') }
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
      // Raw code stays in the console; the toast is always a catalog sentence
      // (or Jira prose + jira_errors). Unknown snake_case codes use fallback,
      // never e.message.
      console.warn('[write]', e.code ?? e.message, e)
      this.toast(formatWriteRejection(e, fallback), 'error')
      return
    }
    console.warn('[write]', fallback, e)
    this.toast(fallback, 'error')
  }
}

function demoPriorities(pool: IssueLite[]): PriorityOption[] {
  const seen = new Map<string, number>()
  for (const it of pool) {
    if (!it.priority || seen.has(it.priority)) continue
    seen.set(it.priority, it.priority_rank ?? 99)
  }
  return [...seen.entries()]
    .sort((a, b) => a[1] - b[1])
    .map(([name]) => ({ id: name, name }))
}

/** Trim, drop empties, de-dupe. Same rules as the server's normalizeLabels. */
function normalizeLabels(input: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of input) {
    const s = raw.trim()
    if (!s || seen.has(s)) continue
    seen.add(s)
    out.push(s)
  }
  return out
}

type CreateWriteResponse = {
  issue: IssueLite
  resolved?: {
    project?: { value?: string; source?: string }
    issue_type?: { value?: string; source?: string }
  }
}

const CREATE_OPTIONAL = [
  'project_key',
  'issue_type',
  'description_text',
  'assignee_account_id',
  'priority_id',
  'labels',
  'duedate',
] as const

/** Literal YYYY-MM-DD. Not Date.parse — that turns a date-only into a timestamp. */
const DUE_DATE_LITERAL = /^(\d{4})-(\d{2})-(\d{2})$/

function dueDateLiteral(s: string): boolean {
  const m = DUE_DATE_LITERAL.exec(s)
  if (!m) return false
  const month = Number(m[2])
  const day = Number(m[3])
  return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

function compactCreatePayload(input: CreateIssuePayload): CreateIssuePayload {
  const body: CreateIssuePayload = { summary: input.summary.trim() }
  const project = input.project_key?.trim()
  if (project) body.project_key = project
  const type = input.issue_type?.trim()
  if (type) body.issue_type = type
  const desc = input.description_text?.trim()
  if (desc) body.description_text = desc
  const assignee = input.assignee_account_id?.trim()
  if (assignee) body.assignee_account_id = assignee
  const priority = input.priority_id?.trim()
  if (priority) body.priority_id = priority
  if (input.labels?.length) {
    const labels = normalizeLabels(input.labels)
    if (labels.length) body.labels = labels
  }
  const due = input.duedate?.trim()
  if (due) body.duedate = due
  return body
}

/** GDK-83 / GDK-52: 400 screen refusals offer the origin hatch only when it can open. */
function originAction(key: string): ToastAction | undefined {
  if (!issueOriginUrl(key)) return undefined
  return { label: t('detail.openJira', { tracker: originTrackerName() }), openIssueKey: key }
}

/** Jira's Message() plus any jira_errors the message did not already carry. */
function formatWriteRejection(e: ApiError, fallback: string): string {
  const head = writeErrorMessage(e.code, fallback, t)
  if (!e.jiraErrors) return head
  const extras: string[] = []
  for (const [field, val] of Object.entries(e.jiraErrors)) {
    if (typeof val !== 'string' || !val) continue
    if (head.includes(val)) continue
    extras.push(`${field}: ${val}`)
  }
  if (extras.length === 0) return head
  return [head, ...extras].join('; ')
}

function omittedCreateFields(body: CreateIssuePayload): string[] {
  return CREATE_OPTIONAL.filter((k) => !(k in body))
}

/** Visible guess only: key plus the type name and project the server filled. */
function createdToast(issue: IssueLite): string {
  const key = issue.issue_key
  const type = (issue.issue_type ?? '').trim()
  const project = (issue.source_project ?? '').trim()
  if (type && project) return t('write.issueCreatedFilled', { key, type, project })
  return t('write.issueCreated', { key })
}

/** App-wide singleton. */
export const write = new WriteStore()
