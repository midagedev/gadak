<script lang="ts">
  /*
   * 사이드바 내비게이션 ([explore]). 섹션: 기본 뷰 / 내 뷰(localStorage) / 팀 공유 뷰(api).
   *  상단: 이슈 총계 + 마지막 동기화. 하단: 개인화(Wave 3) 자리 — 주석 placeholder.
   *  뷰 클릭 = filters.applyConfig(config). 현재 뷰와 일치하면 활성 표시.
   */
  import { filterIssues, filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { views } from '../../stores/views.svelte'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { builtinViews } from '../../lib/builtin-views'
  import { configToParams, type ViewConfig } from '../../lib/view-config'
  import MyIssuesNav from '../personal/MyIssuesNav.svelte'
  import FavoritesNav from '../personal/FavoritesNav.svelte'

  const builtins = builtinViews()

  /** 뷰 적용 = 개인 피드가 열려 있으면 닫고(리스트로 복귀) 필터 적용. */
  function applyView(config: ViewConfig) {
    me.closeFeed()
    filters.applyConfig(config)
  }

  /** config 를 순서 무관 비교 가능한 정규 문자열로. */
  function canon(config: ViewConfig): string {
    const p = configToParams(config)
    return Object.keys(p)
      .sort()
      .map((k) => `${k}=${p[k] ?? ''}`)
      .join('&')
  }

  const currentCanon = $derived(canon(filters.currentConfig()))

  function activeId(views: { id: string; config: ViewConfig }[]): string | null {
    for (const v of views) if (canon(v.config) === currentCanon) return v.id
    return null
  }
  const activeBuiltin = $derived(activeId(builtins))
  const activePersonal = $derived(activeId(views.personal))
  const activeTeam = $derived(
    activeId(views.team.map((v) => ({ id: v.id, config: v.config as unknown as ViewConfig }))),
  )
  const builtinCounts = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const view of builtins) {
      counts.set(view.id, filterIssues(issues.allIssues, view.config.filters).length)
    }
    return counts
  })

  const lastSyncLabel = $derived(
    issues.lastSync ? new Date(issues.lastSync).toLocaleTimeString('ko-KR') : '',
  )

  const STATUS_LABEL: Record<string, string> = {
    healthy: '정상',
    running: '동기화 중',
    paused: '업무시간 외 대기',
    idle: '대기',
    stale: '지연',
    failed: '실패',
    missing: '기록 없음',
  }

  function relativeSync(value: string | null): string {
    if (!value) return ''
    const minutes = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 60_000))
    if (minutes < 1) return '방금 전'
    if (minutes < 60) return `${minutes}분 전`
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `${hours}시간 전`
    return `${Math.floor(hours / 24)}일 전`
  }

  const syncTitle = $derived.by(() =>
    issues.syncHealth?.sources
      .map((source) => {
        const time = relativeSync(source.synced_at)
        return `${source.label} · ${STATUS_LABEL[source.status] ?? source.status}${time ? ` · ${time}` : ''}${source.message && source.message !== '정상' ? `\n${source.message}` : ''}`
      })
      .join('\n'),
  )
  const syncLabel = $derived(
    issues.syncHealth?.overall === 'failed'
      ? '동기화 실패'
      : issues.syncHealth?.overall === 'warning'
        ? '동기화 지연'
        : lastSyncLabel
          ? `동기화 ${lastSyncLabel}`
          : '동기화 확인 중',
  )
  const syncColor = $derived(
    issues.syncHealth?.overall === 'failed'
      ? 'text-status-reopen'
      : issues.syncHealth?.overall === 'warning'
        ? 'text-status-stale'
        : 'text-text-muted',
  )
  const syncDot = $derived(
    issues.syncHealth?.overall === 'failed'
      ? 'bg-status-reopen'
      : issues.syncHealth?.overall === 'warning'
        ? 'bg-status-stale'
        : 'bg-status-done',
  )
</script>

<div class="flex h-full flex-col">
  <!-- 새 이슈 (단축키 c) -->
  <div class="flex-none px-3 pt-1 pb-2">
    <button
      type="button"
      onclick={() => write.openNewIssue()}
      class="flex w-full items-center justify-center gap-1.5 rounded-md bg-accent px-3 py-2 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover"
      title="새 이슈 (c)"
    >
      <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
        <path d="M6 2v8M2 6h8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
      새 이슈
    </button>
  </div>

  <!-- 총계 / 동기화 -->
  <div class="flex-none px-3 pb-2 pt-1 text-[11px] text-text-muted">
    <span class="text-text-secondary">{issues.pool.size.toLocaleString()}</span> 이슈
    <span class="ml-1">·</span>
    <span
      class="ml-1 inline-flex items-center gap-1 {syncColor}"
      title={syncTitle || syncLabel}
      aria-label={syncTitle || syncLabel}
    >
      <span class="h-1.5 w-1.5 flex-none rounded-full {syncDot}" aria-hidden="true"></span>
      {syncLabel}
    </span>
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto">
    <!-- 개인화 (Wave 3): My Issues / 최근 본 이슈 — 기본 뷰 위 -->
    <MyIssuesNav />
    <FavoritesNav />

    <!-- 기본 뷰 -->
    <div class="mb-3">
      <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
        기본 뷰
      </div>
      {#each builtins as v (v.id)}
        <button
          type="button"
          class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {activeBuiltin ===
          v.id
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          title={v.hint}
          onclick={() => applyView(v.config)}
        >
          <span class="flex-none text-[13px]">{v.icon}</span>
          <span class="min-w-0 flex-1 truncate">{v.name}</span>
          <span class="flex-none font-mono text-[11px] tabular-nums text-text-muted">
            {(builtinCounts.get(v.id) ?? 0).toLocaleString()}
          </span>
        </button>
      {/each}
    </div>

    <!-- 내 뷰 -->
    {#if views.personal.length}
      <div class="mb-3">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          내 뷰
        </div>
        {#each views.personal as v (v.id)}
          <div
            class="group flex min-h-7 items-center gap-2 rounded-md px-3 py-1.5 text-[13px] transition-colors {activePersonal ===
            v.id
              ? 'bg-bg-active'
              : 'hover:bg-bg-hover'}"
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate text-left {activePersonal === v.id
                ? 'text-text-primary'
                : 'text-text-secondary group-hover:text-text-primary'}"
              onclick={() => applyView(v.config)}
            >
              {v.name}
            </button>
            <button
              type="button"
              class="flex-none text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100"
              title="삭제"
              onclick={() => views.removePersonal(v.id)}
            >
              ✕
            </button>
          </div>
        {/each}
      </div>
    {/if}

    <!-- 팀 공유 뷰 -->
    {#if views.team.length}
      <div class="mb-3">
        <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
          팀 공유 뷰
        </div>
        {#each views.team as v (v.id)}
          <div
            class="group flex min-h-7 items-center gap-2 rounded-md px-3 py-1.5 text-[13px] transition-colors {activeTeam ===
            v.id
              ? 'bg-bg-active'
              : 'hover:bg-bg-hover'}"
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate text-left {activeTeam === v.id
                ? 'text-text-primary'
                : 'text-text-secondary group-hover:text-text-primary'}"
              onclick={() => applyView(v.config as unknown as ViewConfig)}
              title={v.owner_name ? `작성자: ${v.owner_name}` : undefined}
            >
              {v.name}
              {#if v.owner_name}<span class="ml-1 text-[11px] text-text-muted">· {v.owner_name}</span>{/if}
            </button>
            {#if me.email && v.owner_email === me.email}
              <button
                type="button"
                class="flex-none text-text-muted opacity-0 transition-opacity hover:text-status-reopen group-hover:opacity-100"
                title="삭제"
                onclick={() =>
                  views.removeTeam(v.id).catch(() => alert('뷰를 삭제하지 못했습니다.'))}
              >
                ✕
              </button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

  </div>

  <!-- 로그인 영역(사이드바 하단) -->
  <div class="flex-none border-t border-border-subtle px-3 py-2">
    {#if me.authed}
      <div class="flex items-center gap-2 text-[12px]">
        <span class="min-w-0 flex-1 truncate text-text-secondary" title={me.email ?? undefined}>
          {me.name ?? me.email}
        </span>
        <button
          type="button"
          class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary {write.configured
            ? ''
            : 'text-status-stale'}"
          onclick={() => write.openSettings()}
          title={write.configured ? 'Jira 자격증명 설정' : 'Jira API 토큰 미설정 — 쓰기하려면 설정하세요'}
          aria-label="Jira 자격증명 설정"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path
              d="M8 10.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z"
              stroke="currentColor"
              stroke-width="1.2"
            />
            <path
              d="M8 1.5l.7 1.6 1.7-.5.3 1.8 1.8.3-.5 1.7 1.6.7-1.6.7.5 1.7-1.8.3-.3 1.8-1.7-.5L8 14.5l-.7-1.6-1.7.5-.3-1.8-1.8-.3.5-1.7L1.9 8l1.6-.7-.5-1.7 1.8-.3.3-1.8 1.7.5L8 1.5z"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linejoin="round"
              opacity="0.5"
            />
          </svg>
        </button>
        <button
          type="button"
          class="flex-none text-[11px] text-text-muted transition-colors hover:text-text-primary"
          onclick={() => me.logout()}
        >
          로그아웃
        </button>
      </div>
    {:else if me.authChecked}
      <button
        type="button"
        class="flex w-full items-center justify-center gap-1.5 rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={() => me.promptLogin()}
      >
        로그인
      </button>
    {/if}
  </div>
</div>
