<script lang="ts">
  /*
   * 새 이슈 생성 다이얼로그 (쓰기, 로컬 우선 + 개인화 기본값).
   *  - 프로젝트/이슈타입 = write.writeMetaProjects(로컬 create-meta, 0ms). 비었을 때만 GET create-meta/ 폴백.
   *  - 기본값(조용한 제안):
   *      프로젝트 ① 최근 생성 ② 현재 필터 source_project ③ 선택 이슈 프로젝트 ④ 첫 프로젝트
   *      이슈타입 = 프로젝트별 최근 사용(없으면 Bug, 없으면 첫 타입)
   *      담당자 = 비움(강요 X)
   *  - 라벨 입력 = 로컬 풀에서 해당 프로젝트 빈도순 자동완성 + 최근 사용 상단.
   *  - 성공 시 닫고 selection.select(새 키) → 상세 열림. (recency 기록은 write.createIssue)
   *  진입점: 사이드바 "+ 새 이슈" / 단축키 c (App.svelte).
   */
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { filters } from '../../stores/filters.svelte'
  import { recentOf } from '../../lib/recency'
  import type { CreateMetaProject, JiraUser } from '../../lib/types'

  const PRIORITIES = ['Highest', 'High', 'Medium', 'Low', 'Lowest']

  let loading = $state(true)
  let loadError = $state<string | null>(null)
  let fallbackProjects = $state<CreateMetaProject[]>([])

  // 로컬 메타 우선, 없으면 폴백.
  const projects = $derived(write.writeMetaProjects.length ? write.writeMetaProjects : fallbackProjects)

  let projectKey = $state('')
  let issueTypeId = $state('')
  let summary = $state('')
  let description = $state('')
  let priority = $state('')

  // 담당자(선택)
  let assignee = $state<JiraUser | null>(null)
  let userQuery = $state('')
  let userResults = $state<JiraUser[]>([])
  let userSearching = $state(false)
  let userMenuOpen = $state(false)

  // 라벨(선택)
  let labels = $state<string[]>([])
  let labelInput = $state('')
  let labelMenuOpen = $state(false)

  let submitting = $state(false)
  let submitError = $state<string | null>(null)
  let summaryEl: HTMLInputElement | null = $state(null)

  const selectedProject = $derived(projects.find((p) => p.key === projectKey))
  const issueTypes = $derived(selectedProject?.issue_types ?? [])

  onMount(() => {
    if (write.writeMetaProjects.length) {
      loading = false
      applyDefaults()
    } else {
      void loadFallback()
    }
  })

  async function loadFallback() {
    loading = true
    loadError = null
    try {
      const res = await api.getCreateMeta()
      fallbackProjects = res.projects
      applyDefaults()
    } catch (e) {
      if (e instanceof ApiError && e.code === 'credential_required') {
        write.closeNewIssue()
        write.openSettings()
        return
      }
      loadError = '생성 메타를 불러오지 못했습니다.'
    } finally {
      loading = false
    }
  }

  /** 프로젝트/이슈타입 기본값 유추(최초 1회). */
  let defaultsApplied = false
  function applyDefaults() {
    if (defaultsApplied || projects.length === 0) return
    defaultsApplied = true
    projectKey = inferProject()
    issueTypeId = inferType(projectKey)
    queueMicrotask(() => summaryEl?.focus())
  }

  function inferProject(): string {
    const keys = projects.map((p) => p.key)
    for (const r of recentOf('create-project')) if (keys.includes(r)) return r
    const fromFilter = filters.filters.jira_project?.[0]
    if (fromFilter && keys.includes(fromFilter)) return fromFilter
    const sel = selection.selectedKey ? issues.get(selection.selectedKey) : undefined
    if (sel) {
      const p = write.projectOf(sel)
      if (keys.includes(p)) return p
    }
    return keys[0] ?? ''
  }

  function inferType(pk: string): string {
    const p = projects.find((x) => x.key === pk)
    if (!p) return ''
    const types = p.issue_types
    for (const r of recentOf(`create-type:${pk}`)) if (types.some((t) => t.id === r)) return r
    const bug = types.find((t) => t.name.toLowerCase() === 'bug')
    if (bug) return bug.id
    return types[0]?.id ?? ''
  }

  // 프로젝트 변경 시 유효하지 않으면 해당 프로젝트의 추론 타입으로.
  $effect(() => {
    if (selectedProject && !issueTypes.some((t) => t.id === issueTypeId)) {
      issueTypeId = inferType(projectKey)
    }
  })

  // ── 담당자 검색(디바운스) ──
  let debounce: ReturnType<typeof setTimeout> | null = null
  $effect(() => {
    const q = userQuery.trim()
    if (debounce) clearTimeout(debounce)
    if (q.length < 2) {
      userResults = []
      return
    }
    debounce = setTimeout(async () => {
      userSearching = true
      try {
        const res = await api.searchUsers(q)
        userResults = res.users
        userMenuOpen = true
      } catch {
        userResults = []
      } finally {
        userSearching = false
      }
    }, 250)
  })

  function pickUser(u: JiraUser) {
    assignee = u
    userQuery = ''
    userResults = []
    userMenuOpen = false
  }
  function clearUser() {
    assignee = null
  }

  // ── 라벨 자동완성 ── 해당 프로젝트 빈도 + 최근 사용 상단.
  const labelFreq = $derived.by(() => {
    const freq = new Map<string, number>()
    for (const it of issues.allIssues) {
      if (write.projectOf(it) !== projectKey) continue
      for (const l of it.labels) freq.set(l, (freq.get(l) ?? 0) + 1)
    }
    return freq
  })
  const labelSuggestions = $derived.by<string[]>(() => {
    const q = labelInput.trim().toLowerCase()
    const recent = recentOf('label')
    const all = new Set<string>([...recent, ...labelFreq.keys()])
    const arr = [...all].filter(
      (l) => !labels.includes(l) && (!q || l.toLowerCase().includes(q)),
    )
    arr.sort((a, b) => {
      const ra = recent.indexOf(a)
      const rb = recent.indexOf(b)
      const rra = ra === -1 ? Infinity : ra
      const rrb = rb === -1 ? Infinity : rb
      if (rra !== rrb) return rra - rrb // 최근 상단
      return (labelFreq.get(b) ?? 0) - (labelFreq.get(a) ?? 0) // 빈도순
    })
    return arr.slice(0, 8)
  })

  function addLabel(l: string) {
    const v = l.trim()
    if (!v || labels.includes(v)) {
      labelInput = ''
      return
    }
    labels = [...labels, v]
    labelInput = ''
  }
  function removeLabel(l: string) {
    labels = labels.filter((x) => x !== l)
  }
  function onLabelKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      addLabel(labelSuggestions[0] ?? labelInput)
    } else if (e.key === 'Backspace' && !labelInput && labels.length) {
      labels = labels.slice(0, -1)
    }
  }

  async function submit(e: Event) {
    e.preventDefault()
    if (submitting) return
    const s = summary.trim()
    if (!projectKey || !issueTypeId || !s) {
      submitError = '프로젝트·유형·제목은 필수입니다.'
      return
    }
    submitting = true
    submitError = null
    const res = await write.createIssue({
      project_key: projectKey,
      issue_type: issueTypeId,
      summary: s,
      description_text: description.trim() || undefined,
      assignee_account_id: assignee?.account_id ?? undefined,
      priority: priority || undefined,
      labels: labels.length ? labels : undefined,
    })
    submitting = false
    if (res.ok && res.key) {
      write.closeNewIssue()
      selection.select(res.key)
    } else {
      submitError = res.error ?? '이슈 생성에 실패했습니다.'
    }
  }

  function close() {
    write.closeNewIssue()
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 pt-[8vh]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close()
  }}
>
  <div
    class="anim-enter w-full max-w-lg rounded-lg border border-border-strong bg-bg-panel p-5 shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label="새 이슈"
  >
    <h2 class="mb-4 text-[14px] font-semibold text-text-primary">새 이슈</h2>

    {#if loading}
      <div class="py-8 text-center text-[13px] text-text-muted">불러오는 중…</div>
    {:else if loadError}
      <div class="flex flex-col items-center gap-3 py-8 text-center">
        <p class="text-[13px] text-status-reopen">{loadError}</p>
        <button
          type="button"
          onclick={loadFallback}
          class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] text-text-secondary hover:bg-bg-hover"
          >다시 시도</button
        >
      </div>
    {:else}
      <form onsubmit={submit} class="flex flex-col gap-3">
        <!-- 프로젝트 + 유형 -->
        <div class="flex gap-3">
          <label class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-[11px] text-text-secondary">프로젝트</span>
            <select
              bind:value={projectKey}
              class="rounded-md border border-border-strong bg-bg-base px-2 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
            >
              {#each projects as p (p.key)}
                <option value={p.key}>{p.key} · {p.name}</option>
              {/each}
            </select>
          </label>
          <label class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-[11px] text-text-secondary">유형</span>
            <select
              bind:value={issueTypeId}
              class="rounded-md border border-border-strong bg-bg-base px-2 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
            >
              {#each issueTypes as t (t.id)}
                <option value={t.id}>{t.name}</option>
              {/each}
            </select>
          </label>
        </div>

        <!-- 제목 -->
        <label class="flex flex-col gap-1">
          <span class="text-[11px] text-text-secondary">제목 <span class="text-status-reopen">*</span></span>
          <input
            bind:this={summaryEl}
            bind:value={summary}
            type="text"
            required
            maxlength="255"
            class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
            placeholder="이슈 제목"
          />
        </label>

        <!-- 설명 -->
        <label class="flex flex-col gap-1">
          <span class="text-[11px] text-text-secondary">설명</span>
          <textarea
            bind:value={description}
            rows="4"
            class="resize-y rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
            placeholder="평문 (줄바꿈 유지)"
          ></textarea>
        </label>

        <!-- 담당자 + 우선순위 -->
        <div class="flex gap-3">
          <div class="relative flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-[11px] text-text-secondary">담당자</span>
            {#if assignee}
              <div class="flex items-center gap-2 rounded-md border border-border-strong bg-bg-base px-2 py-1.5 text-[13px]">
                <span class="min-w-0 flex-1 truncate text-text-primary">{assignee.display_name}</span>
                <button type="button" onclick={clearUser} class="flex-none text-text-muted hover:text-status-reopen">✕</button>
              </div>
            {:else}
              <input
                bind:value={userQuery}
                type="text"
                placeholder="이름/이메일 검색 (선택)"
                onfocus={() => (userMenuOpen = userResults.length > 0)}
                class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
              />
              {#if userMenuOpen && (userResults.length > 0 || userSearching)}
                <div class="absolute left-0 right-0 top-full z-20 mt-1 max-h-48 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl">
                  {#if userSearching}
                    <div class="px-3 py-1.5 text-[11px] text-text-muted">검색 중…</div>
                  {/if}
                  {#each userResults as u (u.account_id)}
                    <button
                      type="button"
                      onclick={() => pickUser(u)}
                      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                    >
                      {#if u.avatar_url}
                        <img src={u.avatar_url} alt={u.display_name} class="h-4 w-4 flex-none rounded-full object-cover" loading="lazy" />
                      {/if}
                      <span class="min-w-0 flex-1 truncate">{u.display_name}</span>
                    </button>
                  {/each}
                </div>
              {/if}
            {/if}
          </div>
          <label class="flex w-32 flex-none flex-col gap-1">
            <span class="text-[11px] text-text-secondary">우선순위</span>
            <select
              bind:value={priority}
              class="rounded-md border border-border-strong bg-bg-base px-2 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
            >
              <option value="">(기본)</option>
              {#each PRIORITIES as p (p)}
                <option value={p}>{p}</option>
              {/each}
            </select>
          </label>
        </div>

        <!-- 라벨 -->
        <div class="relative flex flex-col gap-1">
          <span class="text-[11px] text-text-secondary">라벨</span>
          <div class="flex flex-wrap items-center gap-1 rounded-md border border-border-strong bg-bg-base px-2 py-1.5">
            {#each labels as l (l)}
              <span class="inline-flex items-center gap-1 rounded bg-bg-elevated px-1.5 py-0.5 text-[11px] text-text-secondary">
                {l}
                <button type="button" onclick={() => removeLabel(l)} class="text-text-muted hover:text-status-reopen">✕</button>
              </span>
            {/each}
            <input
              bind:value={labelInput}
              onkeydown={onLabelKeydown}
              onfocus={() => (labelMenuOpen = true)}
              onblur={() => setTimeout(() => (labelMenuOpen = false), 120)}
              type="text"
              placeholder={labels.length ? '' : '라벨 추가 (선택)'}
              class="min-w-24 flex-1 bg-transparent text-[13px] text-text-primary outline-none"
            />
          </div>
          {#if labelMenuOpen && labelSuggestions.length > 0}
            <div class="absolute left-0 right-0 top-full z-20 mt-1 max-h-40 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl">
              {#each labelSuggestions as l (l)}
                <button
                  type="button"
                  onclick={() => addLabel(l)}
                  class="flex w-full items-center justify-between gap-2 px-3 py-1 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                >
                  <span class="min-w-0 flex-1 truncate">{l}</span>
                  {#if labelFreq.get(l)}<span class="flex-none text-[10px] text-text-muted">{labelFreq.get(l)}</span>{/if}
                </button>
              {/each}
            </div>
          {/if}
        </div>

        {#if submitError}
          <p class="text-[12px] text-status-reopen">{submitError}</p>
        {/if}

        <div class="mt-1 flex items-center justify-end gap-2">
          <button
            type="button"
            onclick={close}
            class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
            >취소</button
          >
          <button
            type="submit"
            disabled={submitting}
            class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {submitting ? '생성 중…' : '생성'}
          </button>
        </div>
      </form>
    {/if}
  </div>
</div>
