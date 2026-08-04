/*
 * Issue Navigator — 쓰기(Write) 상태/액션 스토어 (계약: 쓰기 프론트)
 *
 * 역할:
 *  - 개인 Jira 자격증명 상태(configured/jira_email/token_hint) + load/save/delete.
 *  - 쓰기 액션(상태 전환·담당자·코멘트·이슈 생성)의 **옵티미스틱** 래퍼.
 *      ① 로컬 풀(issues.pool)의 IssueLite 를 즉시 반영 → ② 서버 호출 →
 *      ③ 응답의 issue 로 확정 교체(+detail 캐시 무효화) → ④ 실패 시 원복 + 토스트.
 *  - 다이얼로그 열림 상태(자격증명 설정 / 새 이슈) + 전역 토스트 큐.
 *
 * 인증/자격증명 게이트:
 *  - 비로그인 → me.promptLogin().
 *  - 로그인했지만 자격증명 미설정 → 설정 다이얼로그 자동 오픈(맥락 유지, 재시도는 사용자 몫).
 *  - 서버가 409 credential_required 를 주면(선반영 실패한 경우) 동일하게 설정 다이얼로그를 연다.
 */

import { SvelteMap } from 'svelte/reactivity'
import * as api from '../lib/api'
import { t } from '../lib/i18n'
import { ApiError } from '../lib/api'
import { issues } from './issues.svelte'
import { me } from './me.svelte'
import { appendComment, invalidate } from '../components/detail/cache.svelte'
import { recordRecent } from '../lib/recency'
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

const WRITE_META_MS = 15 * 60 * 1000 // 쓰기 메타 재로드 주기

export type ToastKind = 'error' | 'info' | 'success'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const TOAST_MS = { error: 6000, info: 3000, success: 2500 }

class WriteStore {
  /* ── 자격증명 상태 ── */
  configured = $state(false)
  jiraEmail = $state('')
  displayName = $state('')
  tokenHint = $state('')
  verifiedAt = $state<string | null>(null)
  /** loadCredential 이 한 번이라도 끝났는지(중복 조회 방지). */
  credentialLoaded = $state(false)

  /* ── 쓰기 메타(로컬 우선) ── transition 맵 + create-meta. 부팅 시 선반영 + 15분 재로드. */
  writeMetaTransitions = $state<Record<string, Record<string, Transition[]>>>({})
  writeMetaProjects = $state<CreateMetaProject[]>([])
  writeMetaUpdatedAt = $state<string | null>(null)
  writeMetaLoaded = $state(false)

  /* ── 다이얼로그 ── */
  settingsOpen = $state(false)
  newIssueOpen = $state(false)

  /* ── 토스트 큐(셸이 렌더) ── */
  toasts = $state<Toast[]>([])

  /**
   * detail 재로딩 신호. 코멘트 확정 후 이 값을 올리면 DetailPanel 이 캐시에서 다시 읽어
   *  낙관적 임시 코멘트를 실제 코멘트로 교체한다(네트워크 왕복 없이 캐시 히트).
   */
  detailNonce = $state(0)

  /** issueKey → 낙관적 코멘트(서버 확정 전). CommentList 가 detail.comments 아래에 덧붙여 렌더. */
  pendingComments = new SvelteMap<string, DetailComment[]>()

  /** issueKey → 편집 가능한 QA 필드 메타(editmeta). 인라인 편집 드롭다운 소스. */
  editMeta = new SvelteMap<string, EditMetaResponse['fields']>()
  #editMetaLoading = new Set<string>()

  /**
   * 답글 요청 브릿지. CommentList 의 '답글' 버튼이 이 값을 세팅하면 CommentComposer 가
   * (effect 로 감지해) 작성자 멘션을 본문에 삽입하고 포커스한다. nonce 로 같은 대상 재요청도 감지.
   */
  replyRequest = $state<{ issueKey: string; user: CommentMention; nonce: number } | null>(null)
  #replyNonce = 0

  /** '답글' — 대상 작성자 멘션 삽입을 컴포저에 요청. account_id 없으면 무시(멘션 불가). */
  requestReply(issueKey: string, user: CommentMention): void {
    if (!user.account_id || !user.display_name) return
    this.replyRequest = { issueKey, user, nonce: ++this.#replyNonce }
  }

  #toastId = 0
  #tmpId = 0
  #writeMetaTimer: ReturnType<typeof setInterval> | null = null

  /* ── 쓰기 메타 (로컬 우선) ── */

  /**
   * 부팅 시퀀스(issues.init 과 병렬):
   *  ① IndexedDB 캐시 하이드레이션(재방문 즉시 0ms) ② 네트워크 최신화 ③ 15분 주기 재로드.
   *  meta/write/ 미배포(404)/오류는 조용히 넘어가고 컴포넌트가 lazy GET 으로 폴백한다.
   */
  async loadWriteMeta(): Promise<void> {
    if (!this.writeMetaLoaded) {
      try {
        const cached = await db.getWriteMeta()
        if (cached) {
          this.#applyWriteMeta(cached.transitions, cached.projects, cached.updated_at)
        }
      } catch {
        /* 캐시 없음/불가 — 무시 */
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
      // $state proxy 는 IndexedDB 에 structured clone 불가 → 평문 스냅샷으로 저장.
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
      // 404(미배포)는 조용히 — 폴백 경로가 처리. 그 외만 경고.
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

  /** 이슈의 워크플로 프로젝트 키(백엔드 project_of 와 동일: issue_key prefix 우선). */
  projectOf(issue: IssueLite): string {
    const k = issue.issue_key
    if (k && k.includes('-')) return k.split('-')[0]
    return issue.source_project ?? ''
  }

  /** 로컬 전환 맵에서 이 이슈의 전환 후보. 없으면 null(컴포넌트가 lazy GET 폴백). */
  transitionsFor(issue: IssueLite): Transition[] | null {
    const byStatus = this.writeMetaTransitions[this.projectOf(issue)]
    const list = byStatus?.[issue.status]
    return list && list.length ? list : null
  }

  /* ── 토스트 ── */

  toast(message: string, kind: ToastKind = 'info'): void {
    const id = ++this.#toastId
    this.toasts = [...this.toasts, { id, kind, message }]
    setTimeout(() => this.dismissToast(id), TOAST_MS[kind])
  }

  dismissToast(id: number): void {
    this.toasts = this.toasts.filter((t) => t.id !== id)
  }

  /* ── 자격증명 ── */

  async loadCredential(): Promise<void> {
    if (!me.authed) return
    try {
      const c = await api.getCredential()
      this.#applyCredential(c)
    } catch (e) {
      // 401 등은 조용히(비로그인 취급). 다른 오류는 로그만.
      if (!(e instanceof ApiError && e.status === 401)) {
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
      this.toast(t('write.credSaved'), 'success')
      return { ok: true }
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : t('write.credSaveFailed')
      return { ok: false, error: msg }
    }
  }

  /** 로그아웃 시 자격증명 상태 초기화(다음 로그인에서 재조회되도록). */
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

  /* ── 다이얼로그 ── */

  openSettings(): void {
    this.settingsOpen = true
  }
  closeSettings(): void {
    this.settingsOpen = false
  }
  openNewIssue(): void {
    // 새 이슈도 쓰기 게이트를 통과해야 열린다.
    void this.#gateThen(() => {
      this.newIssueOpen = true
    })
  }
  closeNewIssue(): void {
    this.newIssueOpen = false
  }

  /* ── 쓰기 게이트 ── */

  /**
   * 쓰기 가능 여부를 보장한다.
   *  - 비로그인 → 로그인 다이얼로그 + false.
   *  - 자격증명 미설정 → 설정 다이얼로그 + false.
   *  - 통과 → true.
   */
  async ensureWritable(): Promise<boolean> {
    // 자격증명이 곧 identity — 미설정이면 로그인이 아니라 설정으로 안내한다.
    if (!this.credentialLoaded) await this.loadCredential()
    if (!this.configured) {
      this.toast(t('write.needToken'), 'info')
      this.openSettings()
      return false
    }
    if (!me.authed) {
      me.promptLogin()
      return false
    }
    return true
  }

  /** 게이트 통과 시에만 fn 실행. */
  async #gateThen(fn: () => void): Promise<void> {
    if (await this.ensureWritable()) fn()
  }

  bumpDetail(): void {
    this.detailNonce++
  }

  /* ── IssueLite 로컬 반영/영속 ── */

  #applyIssue(issue: IssueLite | null | undefined): void {
    if (!issue || !issue.issue_key) return
    issues.pool.set(issue.issue_key, issue)
    void db.putIssues([issue]).catch(() => {})
  }

  /* ── 공통 옵티미스틱 이슈 쓰기 ──
   * patch 를 즉시 풀에 반영 → call() → 응답 issue 로 확정 → 실패 시 스냅샷 원복.
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
    try {
      const res = await call()
      this.#applyIssue(res.issue)
      invalidate(key) // detail(히스토리 등) 다음 열람 시 최신화
      return true
    } catch (e) {
      if (snapshot) issues.pool.set(key, snapshot) // 롤백
      this.#handleError(e, failMsg)
      return false
    }
  }

  /* ── 상태 전환 ── */

  async transition(key: string, tr: Transition): Promise<boolean> {
    const proj = this.projectOf(issues.pool.get(key) ?? ({ issue_key: key } as IssueLite))
    const ok = await this.#writeIssue(
      key,
      { status: tr.to_status, status_category: tr.to_category },
      () => api.doTransition(key, tr.id),
      t('write.transitionFailed'),
    )
    if (ok) recordRecent(`transition:${proj}`, tr.id) // 성공만 기록
    return ok
  }

  /* ── 담당자 ── */

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
    if (ok && user?.account_id) recordRecent('assignee', user.account_id) // 성공만 기록
    return ok
  }

  /* ── QA 필드 인라인 편집 ── */

  /**
   * 편집 시작 게이트: 쓰기 가능 보장 + 이 이슈의 editmeta(허용값) 로드.
   * 컴포넌트가 편집 UI 를 열기 직전에 호출한다. 통과 못하면 false(다이얼로그/토스트는 내부 처리).
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

  /** 이 이슈에서 해당 필드의 편집 메타. 없으면 null(로드 전이거나 편집 불가). */
  editFieldMeta(key: string, field: string): EditMetaField | null {
    return this.editMeta.get(key)?.[field] ?? null
  }

  /**
   * QA 필드 값 변경(옵티미스틱). ``patch`` 는 IssueLite 표시값을 즉시 반영하기 위한 부분 갱신
   * (옵션 표시값·담당자 이름·버전명 등). 서버 응답의 재동기화된 IssueLite 로 최종 확정된다.
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

  /* ── 코멘트 ── */

  /**
   * 코멘트 등록. 낙관적으로 임시 코멘트를 pendingComments 에 추가(CommentList 즉시 표시),
   *  성공 시 임시 제거 + 캐시에 실제 코멘트 append + detail 재읽기(nonce), 실패 시 임시 제거.
   * 반환값 false 면 호출부(컴포저)가 입력 텍스트를 복원한다.
   */
  async submitComment(
    key: string,
    text: string,
    mentions: CommentMention[] = [],
    attachments: UploadedAttachment[] = [],
  ): Promise<boolean> {
    if (!(await this.ensureWritable())) return false
    const tmpId = `temp-${++this.#tmpId}`
    // 낙관적 표시: 멘션은 평문 그대로, 첨부는 파일명 목록을 덧붙여 보여준다(확정 후 실제 렌더).
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
      appendComment(key, real) // 캐시에 반영 → 재읽기 시 실제 코멘트로 표시
      this.#applyIssue(res.issue) // comment_count 등 갱신
      this.bumpDetail() // DetailPanel 이 캐시에서 재로딩
      return true
    } catch (e) {
      this.#removePending(key, tmpId)
      this.#handleError(e, t('write.commentFailed'))
      return false
    }
  }

  /**
   * 코멘트용 첨부 업로드. 성공 시 업로드된 첨부 메타 배열, 실패 시 null(토스트).
   * 쓰기 게이트를 통과해야 하며, 여러 파일은 호출부가 병렬로 올린다.
   */
  async uploadAttachment(key: string, file: File): Promise<UploadedAttachment[] | null> {
    if (!(await this.ensureWritable())) return null
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

  /* ── 이슈 생성 ── */

  /** 성공 시 새 이슈를 풀에 추가하고 issue_key 반환(다이얼로그가 선택/닫기 처리). */
  async createIssue(payload: CreateIssuePayload): Promise<{ ok: boolean; key?: string; error?: string }> {
    if (!(await this.ensureWritable())) return { ok: false }
    try {
      const res = await api.createIssue(payload)
      this.#applyIssue(res.issue)
      // 성공만 기록: 프로젝트/타입/라벨 최근 사용(새 이슈 기본값·자동완성 개인화)
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

  /* ── 에러 처리 ── */

  #handleError(e: unknown, fallback: string): void {
    if (e instanceof ApiError) {
      if (e.code === 'credential_required') {
        // 선반영 실패(자격증명이 사라졌거나 최초 미설정) → 설정 다이얼로그
        this.configured = false
        this.toast(t('write.needToken'), 'info')
        this.openSettings()
        return
      }
      if (e.status === 401) {
        me.promptLogin()
        return
      }
      this.toast(e.message || fallback, 'error')
      return
    }
    console.warn('[write]', fallback, e)
    this.toast(fallback, 'error')
  }
}

/** 앱 전역 싱글턴. */
export const write = new WriteStore()
