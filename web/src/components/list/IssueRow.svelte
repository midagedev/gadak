<script lang="ts">
  /*
   * 이슈 행 ([explore]). 고정 높이 42px(가상 스크롤 전제).
   *  구성: 우선순위 아이콘 · 상태 점 · 키(모노) · 제목 · 라벨칩(≤3+n) · 재오픈/정체 배지 · 담당자 · 상대시간
   *  모든 칩/점/아바타 클릭 = 해당 값 필터 추가(stopPropagation 로 행 선택과 분리).
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { filters } from '../../stores/filters.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import { me } from '../../stores/me.svelte'
  import { presence } from '../../stores/presence.svelte'
  import { categoryOf, CATEGORY_META, relativeTime, absTime, highlightSegments } from '../../lib/format'
  import { feature } from '../../lib/config'
  import { isStale, statusAgeHours } from '../../lib/view-config'
  import PriorityIcon from './PriorityIcon.svelte'
  import Avatar from './Avatar.svelte'
  import ViewerStack from '../presence/ViewerStack.svelte'
  import { prefetchDetail } from '../detail/cache.svelte'

  let {
    issue,
    active = false,
    cursor = false,
  }: { issue: IssueLite; active?: boolean; cursor?: boolean } = $props()

  const cat = $derived(categoryOf(issue))
  const catMeta = $derived(CATEGORY_META[cat])
  const isFavorite = $derived(me.favorites.has(issue.issue_key))
  const isWatching = $derived(me.watches.has(issue.issue_key))
  // 본인 제외 뷰어(O(1) Map 조회). 없으면 참조 안정한 빈 배열 → 아래 블록 스킵, 행 오버헤드 없음.
  const rowViewers = $derived(presence.viewersOf(issue.issue_key))
  // 정체(현재 상태 경과). 배지 문구는 일 단위 — 1일 미만도 "1일째"로 읽히게 최소 1.
  const stale = $derived(isStale(issue))
  const staleDays = $derived(Math.max(1, Math.round(statusAgeHours(issue) / 24)))
  const shownLabels = $derived(issue.labels.slice(0, 3))
  const extraLabels = $derived(Math.max(0, issue.labels.length - 3))
  // 노출 컬럼 집합(뷰 설정). O(1) 조회로 각 후행 필드 렌더 여부를 가른다.
  const cols = $derived(new Set(filters.display.columns))
  // 검색어 하이라이팅(제목·키). q 없으면 단일 조각이라 렌더 비용 동일.
  const summarySegs = $derived(highlightSegments(issue.summary, filters.filters.q))
  const keySegs = $derived(highlightSegments(issue.issue_key, filters.filters.q))
  const qaImpactMeta = $derived.by(() => {
    switch (issue.qa_impact_state) {
      case 'blocking':
        return { label: t('list.qaBlock'), cls: 'bg-status-reopen/15 text-status-reopen' }
      case 'retest':
        return { label: t('list.qaRetest'), cls: 'bg-status-stale/15 text-status-stale' }
      case 'verified':
        return { label: t('list.qaDone'), cls: 'bg-status-done/15 text-status-done' }
      case 'linked':
        return { label: t('list.qaRun'), cls: 'bg-accent-subtle/60 text-accent-text' }
      default:
        return null
    }
  })
  // 배포 단계 배지. QA팀 핵심 스캔 대상은 qa(스왑 완료)이므로 청록으로 확실히,
  //  그 외(대기/dev/prod)는 흐린 톤. none/merged 는 배지 없음(노이즈 방지).
  const deployState = $derived(issue.deploy_status?.state ?? 'none')
  const deployMeta = $derived.by(() => {
    switch (deployState) {
      case 'qa':
        // 스왑 완료 = QA 확인 가능 — 청록 점 + 라벨로 리스트에서 바로 잡히게
        return { label: 'QA', cls: 'bg-[#2dd4bf]/15 text-[#5eead4]', dot: true }
      case 'qa_preview':
        return { label: t('list.qaPending'), cls: 'bg-[#2dd4bf]/8 text-[#2dd4bf]/70', dot: false }
      case 'dev':
        return { label: 'dev', cls: 'bg-bg-active text-text-muted', dot: false }
      case 'prod':
        return { label: 'prod', cls: 'bg-accent-subtle/50 text-accent-text', dot: false }
      default:
        return null
    }
  })
  // 해결됨(done)인데 어느 릴리즈에도 미포함(merged 단계에 머묾) → 미묘한 경고 톤.
  const deployStale = $derived(cat === 'done' && deployState === 'merged')

  // 자기 이슈 하이라이팅. 단, 이미 "내 이슈"로 스코프된 뷰에선 전부 내 것이라 끈다.
  const mine = $derived(filters.isMine(issue) && !filters.scopedToMe)

  // 최신성 가중치: 24시간 내 갱신은 시간 표기를 액센트로 끌어올린다.
  const isFresh = $derived.by(() => {
    if (!issue.updated_at) return false
    const t = Date.parse(issue.updated_at)
    return Number.isFinite(t) && Date.now() - t < 24 * 60 * 60 * 1000
  })

  function stop(fn: () => void) {
    return (e: MouseEvent) => {
      e.stopPropagation()
      fn()
    }
  }

  // ── 멀티선택 ──
  const selected = $derived(bulk.has(issue.issue_key))
  // 체크박스: 평소 흐리게(hover 시 선명), 선택되거나 선택 모드면 항상 선명.
  const boxOpacity = $derived(
    selected || bulk.count > 0 ? 'opacity-100' : 'opacity-40 group-hover:opacity-100',
  )

  // 체크박스 영역 클릭 = 선택 토글(행 열기와 분리). shift 는 범위 선택.
  function onCheckClick(e: MouseEvent) {
    e.stopPropagation()
    if (e.shiftKey) bulk.selectRange(issue.issue_key)
    else bulk.toggle(issue.issue_key)
  }

  // 행 클릭: shift = 범위 선택, 선택 모드(1개 이상)면 토글, 그 외엔 상세 열기.
  function onRowClick(e: MouseEvent) {
    if (e.shiftKey) {
      e.preventDefault()
      bulk.selectRange(issue.issue_key)
      return
    }
    if (bulk.count > 0) {
      bulk.toggle(issue.issue_key)
      return
    }
    selection.toggle(issue.issue_key)
  }
</script>

<div
  class="group flex h-row cursor-pointer select-none items-center gap-2.5 border-b border-border-subtle/70 px-4 text-body transition-colors
    {selected
      ? 'bg-accent-subtle/30'
      : active
        ? 'bg-accent-subtle/20 shadow-[inset_3px_0_0_var(--color-accent)]'
        : cursor
          ? 'bg-bg-hover shadow-[inset_0_0_0_1px_var(--color-accent)]'
          : mine
            ? 'bg-accent-subtle/10 hover:bg-bg-hover'
            : 'hover:bg-bg-hover'}"
  role="button"
  tabindex="-1"
  aria-current={active ? 'true' : undefined}
  onclick={onRowClick}
  onmouseenter={() => prefetchDetail(issue.issue_key)}
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      selection.toggle(issue.issue_key)
    }
  }}
>
  <!-- 멀티선택 체크박스(체크박스 영역만 선택; shift=범위) -->
  <button
    type="button"
    class="flex h-4 w-4 flex-none items-center justify-center rounded border transition-all {boxOpacity}
      {selected ? 'border-accent bg-accent text-white' : 'border-border-strong'}"
    onclick={onCheckClick}
    aria-pressed={selected}
    aria-label={selected ? t('list.deselect') : t('list.select')}
    title={t('list.select')}
  >
    {#if selected}<span class="text-[9px]">✓</span>{/if}
  </button>

  <!-- 우선순위 -->
  <PriorityIcon priority={issue.priority} />

  <!-- 상태 점(클릭 = 분류 필터) -->
  <button
    type="button"
    class="h-2 w-2 flex-none rounded-full transition-transform hover:scale-125"
    style:background={catMeta.color}
    title={t('list.categoryTitle', { label: catMeta.label, status: issue.status })}
    onclick={stop(() => filters.addValue('status_category', cat))}
    aria-label={t('list.categoryFilter', { label: catMeta.label })}
  ></button>

  <!-- 키 (자기 이슈면 액센트 톤으로 소속 표시) -->
  <span class="w-[88px] flex-none truncate font-mono text-[12px] {mine ? 'text-accent-text' : 'text-text-secondary'}">
    {#each keySegs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
  </span>

  <!-- 개인화 마커(즐겨찾기/워치) — 과하지 않게, 제목 앞 -->
  {#if isFavorite || isWatching}
    <span class="flex flex-none items-center gap-0.5 text-[10px]" aria-hidden="true">
      {#if isFavorite}<span class="text-status-stale" title={t('common.favorite')}>★</span>{/if}
      {#if isWatching}<span class="text-accent-text" title={t('common.watching')}>👁</span>{/if}
    </span>
  {/if}

  <!-- 제목 -->
  <span class="min-w-0 flex-1 truncate font-medium text-text-primary" title={issue.summary}>
    {#each summarySegs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
    {#if filters.filters.reopened && issue.reopen_count > 0 && issue.reopen_reason}
      <!-- 사유 인라인은 재오픈 뷰에서만 — 일반 리스트에선 🔁 배지+툴팁으로 충분(노이즈 최소화) -->
      <span class="ml-1 text-[11px] text-status-reopen/80" title={issue.reopen_reason}>
        · {issue.reopen_reason}
      </span>
    {/if}
  </span>

  <!-- 배지: 재오픈 / 정체 -->
  {#if cols.has('reopen') && issue.reopen_count > 0}
    <button
      type="button"
      class="flex-none rounded bg-status-reopen/15 px-1.5 py-0.5 text-[10px] font-medium text-status-reopen transition-colors hover:bg-status-reopen/25"
      title={issue.reopen_reason ? t('list.reopenCountReason', { n: issue.reopen_count, reason: issue.reopen_reason }) : t('list.reopenCount', { n: issue.reopen_count })}
      onclick={stop(() => filters.toggleFlag('reopened'))}
    >
      🔁 {issue.reopen_count}
    </button>
  {/if}
  {#if cols.has('stale') && stale}
    <span
      class="flex-none rounded bg-status-stale/15 px-1.5 py-0.5 text-[10px] font-medium text-status-stale"
      title={t('list.staleDays', { n: staleDays })}
    >
      {t('list.staleDaysShort', { n: staleDays })}
    </span>
  {/if}

  {#if cols.has('qa_impact') && qaImpactMeta}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity hover:opacity-80 xl:inline-flex {qaImpactMeta.cls}"
      title={issue.qa_runs?.map((run) => run.label).join(', ') || issue.qa_impact_label}
      onclick={stop(() => filters.addValue('qa_impact', issue.qa_impact_state))}
    >
      {qaImpactMeta.label}
    </button>
  {/if}

  <!-- 배포 단계 배지 (qa=청록 강조 / 나머지 흐린 톤) -->
  {#if cols.has('deploy') && deployMeta}
    <button
      type="button"
      class="flex flex-none items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity hover:opacity-80 {deployMeta.cls}"
      title={deployState === 'qa'
        ? t('deploy.qaSwapDone')
        : t('deploy.stageTitle', { label: deployMeta.label })}
      onclick={stop(() => filters.addValue('deploy_state', deployState))}
    >
      {#if deployMeta.dot}
        <span class="h-1.5 w-1.5 flex-none rounded-full bg-[#2dd4bf]"></span>
      {/if}
      {deployMeta.label}
    </button>
  {:else if cols.has('deploy') && deployStale}
    <span
      class="flex-none rounded bg-status-stale/12 px-1.5 py-0.5 text-[10px] font-medium text-status-stale/80"
      title={t('deploy.resolvedNoRelease')}
    >
      {t('deploy.notDeployed')}
    </span>
  {/if}

  <!-- 선택 노출 컬럼(뷰 설정) — 값 있을 때만, 대부분 클릭 시 해당 값 필터 추가 -->
  {#if cols.has('severity') && issue.severity}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.severity'), value: issue.severity })}
      onclick={stop(() => filters.addValue('severity', issue.severity!))}
    >
      {issue.severity}
    </button>
  {/if}
  {#if cols.has('issue_type') && issue.issue_type}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.type'), value: issue.issue_type })}
      onclick={stop(() => filters.addValue('issue_type', issue.issue_type))}
    >
      {issue.issue_type}
    </button>
  {/if}
  {#if cols.has('status') && issue.status}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.status'), value: issue.status })}
      onclick={stop(() => filters.addValue('status', issue.status))}
    >
      {issue.status}
    </button>
  {/if}
  {#if cols.has('dev_test_result') && issue.development_test_result}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.dev_test_result'), value: issue.development_test_result })}
      onclick={stop(() => filters.addValue('development_test_result', issue.development_test_result!))}
    >
      {issue.development_test_result}
    </button>
  {/if}
  {#if cols.has('environment') && issue.environment}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.environment'), value: issue.environment })}
      onclick={stop(() => filters.addValue('environment', issue.environment!))}
    >
      {issue.environment}
    </button>
  {/if}
  {#if cols.has('d1_group') && issue.d1_group}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.d1_group'), value: issue.d1_group })}
      onclick={stop(() => filters.addValue('d1_group', issue.d1_group!))}
    >
      {issue.d1_group}
    </button>
  {/if}
  {#if cols.has('reporter') && issue.reporter}
    <button
      type="button"
      class="hidden max-w-[90px] flex-none truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('common.reporter'), value: issue.reporter })}
      onclick={issue.reporter_email
        ? stop(() => filters.addValue('reporter_email', issue.reporter_email!))
        : undefined}
    >
      {issue.reporter}
    </button>
  {/if}
  {#if cols.has('comment_count') && issue.comment_count > 0}
    <span class="hidden flex-none text-[10px] text-text-muted sm:inline" title={t('list.commentCount', { n: issue.comment_count })}>
      💬 {issue.comment_count}
    </span>
  {/if}
  {#if cols.has('fix_versions') && issue.fix_versions.length}
    <span class="hidden flex-none items-center gap-1 lg:flex">
      <button
        type="button"
        class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
        title={`Fix Version: ${issue.fix_versions.join(', ')}`}
        onclick={stop(() => filters.addValue('fix_versions', issue.fix_versions[0]))}
      >
        {issue.fix_versions[0]}
      </button>
      {#if issue.fix_versions.length > 1}
        <span class="text-[10px] text-text-muted">+{issue.fix_versions.length - 1}</span>
      {/if}
    </span>
  {/if}
  {#if cols.has('components') && issue.components.length}
    <span class="hidden flex-none items-center gap-1 lg:flex">
      <button
        type="button"
        class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
        title={t('list.fieldValue', { field: t('field.components'), value: issue.components.join(', ') })}
        onclick={stop(() => filters.addValue('components', issue.components[0]))}
      >
        {issue.components[0]}
      </button>
      {#if issue.components.length > 1}
        <span class="text-[10px] text-text-muted">+{issue.components.length - 1}</span>
      {/if}
    </span>
  {/if}
  {#if cols.has('created') && issue.created_at}
    <span class="hidden w-10 flex-none text-right text-[11px] text-text-muted sm:inline" title={t('list.createdAt', { time: absTime(issue.created_at) })}>
      {relativeTime(issue.created_at)}
    </span>
  {/if}

  <!-- 라벨 칩 -->
  {#if cols.has('labels') && shownLabels.length}
    <span class="hidden flex-none items-center gap-1 md:flex">
      {#each shownLabels as label (label)}
        <button
          type="button"
          class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[11px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          onclick={stop(() => filters.addValue('labels', label))}
          title={t('list.fieldValue', { field: t('common.labels'), value: label })}
        >
          {label}
        </button>
      {/each}
      {#if extraLabels}
        <span class="text-[11px] text-text-muted">+{extraLabels}</span>
      {/if}
    </span>
  {/if}

  <!-- 같이 보는 중(본인 제외) — 소형 스택. 뷰어 없으면 렌더 안 함. -->
  {#if feature('presence') && rowViewers.length > 0}
    <ViewerStack viewers={rowViewers} size={16} max={2} ringClass="ring-bg-base" />
  {/if}

  <!-- 담당자 -->
  {#if cols.has('assignee')}
    <Avatar
      email={issue.assignee_email}
      name={issue.assignee}
      onclick={issue.assignee_email
        ? stop(() => filters.addValue('assignee_email', issue.assignee_email!))
        : undefined}
    />
  {/if}

  <!-- 상대시간(갱신). 24h 내 갱신은 액센트로 최신성 강조 -->
  {#if cols.has('updated')}
    <span
      class="w-10 flex-none text-right text-[11px] {isFresh
        ? 'font-medium text-accent-text'
        : 'text-text-muted'}"
      title={absTime(issue.updated_at)}
    >
      {relativeTime(issue.updated_at)}
    </span>
  {/if}
</div>
