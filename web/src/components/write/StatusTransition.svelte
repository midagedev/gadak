<script lang="ts">
  /*
   * 상태 칩 + 전환 드롭다운 (쓰기, 로컬 우선).
   *  - 평시엔 기존 상태 칩과 동일한 모습. hover 시 편집 어포던스(쉐브론).
   *  - 클릭 → write.transitionsFor(issue)(로컬 맵)에서 0ms 렌더. 없으면 GET <key>/transitions/ 폴백.
   *  - 정렬(조용한 제안): ① 최근 사용 전환(프로젝트별) ② 워크플로 전진(new→inprogress→done) ③ 나머지.
   *    첫 항목에 포커스 → Enter 로 즉시 실행.
   *  - 선택 시 write.transition()(옵티미스틱). 로컬 실행이 실패하면(맵 낡음 등) 원격으로 재조회.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite, Transition } from '../../lib/types'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { recentOf } from '../../lib/recency'
  import { normalizeCategory } from '../detail/format'

  let { issue }: { issue: IssueLite } = $props()

  let open = $state(false)
  let loading = $state(false)
  let remote = $state<Transition[] | null>(null) // 폴백 GET 결과
  let source = $state<'local' | 'remote'>('local')
  let loadError = $state<string | null>(null)
  let busyId = $state<string | null>(null)
  let rootEl = $state<HTMLDivElement | null>(null)
  let listEl = $state<HTMLDivElement | null>(null)

  const cat = $derived(normalizeCategory(issue.status_category))
  const dotClass = $derived(
    cat === 'done' ? 'bg-status-done' : cat === 'new' ? 'bg-status-new' : 'bg-status-inprogress',
  )

  /** Jira statusCategory key → 도트 색 클래스. */
  function catDot(key: string): string {
    const c = (key || '').toLowerCase()
    if (c === 'done') return 'bg-status-done'
    if (c === 'new') return 'bg-status-new'
    return 'bg-status-inprogress'
  }

  /** 현재 카테고리 순위(new=0, inprogress=1, done=2) — 전진 방향 판별용. */
  const curRank = $derived(cat === 'new' ? 0 : cat === 'done' ? 2 : 1)
  function jiraRank(key: string): number {
    const c = (key || '').toLowerCase()
    if (c === 'new') return 0
    if (c === 'done') return 2
    return 1 // indeterminate 등
  }

  /** 정렬된 전환 목록(로컬 우선 → 폴백). */
  const sorted = $derived.by<Transition[]>(() => {
    const base = remote ?? write.transitionsFor(issue) ?? []
    if (base.length === 0) return []
    const proj = write.projectOf(issue)
    const recent = recentOf(`transition:${proj}`)
    const recIdx = (id: string) => {
      const i = recent.indexOf(id)
      return i === -1 ? Infinity : i
    }
    return [...base].sort((a, b) => {
      const ra = recIdx(a.id)
      const rb = recIdx(b.id)
      if (ra !== rb) return ra - rb // ① 최근 사용
      const fa = jiraRank(a.to_category) > curRank ? 0 : 1
      const fb = jiraRank(b.to_category) > curRank ? 0 : 1
      if (fa !== fb) return fa - fb // ② 전진 방향 우선
      return 0
    })
  })

  async function toggle() {
    if (open) {
      open = false
      return
    }
    remote = null
    source = 'local'
    loadError = null
    // 로컬 맵에 있으면 즉시(0ms). 없으면 원격 폴백.
    if (write.transitionsFor(issue)) {
      open = true
      focusFirst()
    } else {
      await loadRemote()
    }
  }

  async function loadRemote() {
    // 원격 GET 은 인증 필요 → 게이트 먼저.
    if (!(await write.ensureWritable())) {
      open = false
      return
    }
    open = true
    loading = true
    source = 'remote'
    loadError = null
    try {
      const res = await api.getTransitions(issue.issue_key)
      remote = res.transitions
      focusFirst()
    } catch (e) {
      if (
        e instanceof ApiError &&
        (e.code === 'credential_required' || e.code === 'credential_rejected')
      ) {
        open = false
        // #handleError path is inside write; surface the right copy here too.
        if (e.code === 'credential_rejected') write.toast(t('write.tokenRejected'), 'error')
        else write.toast(t('write.needToken'), 'info')
        write.openSettings()
        return
      }
      loadError = t('write.transitionsFailed')
      remote = []
    } finally {
      loading = false
    }
  }

  /** 첫 항목 포커스 → Enter 로 바로 실행. */
  function focusFirst() {
    queueMicrotask(() => listEl?.querySelector('button')?.focus())
  }

  async function pick(t: Transition) {
    busyId = t.id
    const wasLocal = source === 'local'
    const ok = await write.transition(issue.issue_key, t)
    busyId = null
    if (ok) {
      open = false
      remote = null // 상태가 바뀌었으니 다음엔 새로
    } else if (wasLocal) {
      // 로컬 맵이 낡아 실패했을 수 있음 → 원격으로 최신 목록 재조회(드롭다운 유지)
      await loadRemote()
    }
  }

  // 바깥 클릭 / Esc 로 닫기
  $effect(() => {
    if (!open) return
    function onDown(e: MouseEvent) {
      if (rootEl && !rootEl.contains(e.target as Node)) open = false
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') open = false
    }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  })

  const canEdit = $derived(me.identified)
</script>

<div class="relative inline-block" bind:this={rootEl}>
  <button
    type="button"
    onclick={toggle}
    data-testid="status-transition"
    class="group inline-flex items-center gap-1.5 rounded-md bg-bg-elevated px-2 py-0.5 text-[11px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
    aria-haspopup="listbox"
    aria-expanded={open}
    title={canEdit ? t('write.changeStatus') : issue.status}
  >
    <span class="h-1.5 w-1.5 rounded-full {dotClass}"></span>
    {issue.status}
    <svg
      width="9"
      height="9"
      viewBox="0 0 10 10"
      fill="none"
      aria-hidden="true"
      class="text-text-muted opacity-0 transition-opacity group-hover:opacity-100"
    >
      <path d="M2.5 4l2.5 2.5L7.5 4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </button>

  {#if open}
    <div
      bind:this={listEl}
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-52 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl"
      role="listbox"
    >
      {#if loading}
        <div class="px-3 py-2 text-[12px] text-text-muted">{t('write.loadingTransitions')}</div>
      {:else if loadError}
        <div class="px-3 py-2 text-[12px] text-status-reopen">{loadError}</div>
      {:else if sorted.length === 0}
        <div class="px-3 py-2 text-[12px] text-text-muted">{t('write.noTransitions')}</div>
      {:else}
        {#each sorted as t (t.id)}
          <button
            type="button"
            role="option"
            aria-selected="false"
            onclick={() => pick(t)}
            disabled={busyId !== null}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none disabled:opacity-50"
          >
            <span class="h-1.5 w-1.5 flex-none rounded-full {catDot(t.to_category)}"></span>
            <span class="min-w-0 flex-1 truncate">{t.name}</span>
            {#if busyId === t.id}
              <span class="flex-none text-[11px] text-text-muted">…</span>
            {:else if t.to_status && t.to_status !== t.name}
              <span class="flex-none text-[11px] text-text-muted">→ {t.to_status}</span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>
