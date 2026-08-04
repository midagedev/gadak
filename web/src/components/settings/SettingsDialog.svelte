<script lang="ts">
  /*
   * 서버 설정 편집 다이얼로그 (`~/.scry/config.json`, loopback 전용 API).
   *  - 열릴 때 GET settings/, 저장은 PUT settings/(전체 교체) → 성공 시 location.reload().
   *    config.json·bootstrap 멤버·그룹 주입이 전부 서버 파생이라 전체 리로드가 가장 정직하다.
   *  - Record/배열은 편집용 행 배열로 펼쳐 두고 저장 시 다시 조립한다(빈 키 행은 버림).
   *  - "고급" 의 JSON textarea 와 폼은 마지막 수정이 이긴다(파싱 성공 시 즉시 폼에 반영).
   *  Jira 개인 토큰은 JiraKeySettings 담당 — 여기선 링크만.
   *  JiraKeySettings 의 모달 패턴을 따른다(Esc/배경 클릭 닫기).
   */
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import type { ScrySettings } from '../../lib/api'
  import type { ScryFeatures } from '../../lib/config'
  import { write } from '../../stores/write.svelte'
  import KeyValueRows from './KeyValueRows.svelte'

  let { onclose }: { onclose: () => void } = $props()

  type Tab = 'sync' | 'features' | 'groups' | 'members' | 'fields'
  const TABS: [Tab, string][] = [
    ['sync', '동기화'],
    ['features', '기능'],
    ['groups', '팀/그룹'],
    ['members', '멤버'],
    ['fields', '필드 매핑'],
  ]

  const FEATURES: [keyof ScryFeatures, string, string][] = [
    ['presence', '프레즌스', '같은 이슈를 보고 있는 사람 표시 — 로컬 단일 사용자에선 의미 없음'],
    ['feed', '개인 피드', '멘션·워치·담당자 변경을 모은 활동 피드'],
    ['push', '웹 푸시', '피드 이벤트를 브라우저 알림으로 — 서버 VAPID 키 필요'],
    ['deploy', '배포 상태', '이슈별 배포 단계 — 외부 CI/CD 인덱스 필요'],
    ['qa', 'QA 컨텍스트', '이슈별 테스트 런·스위트 — 외부 QA 도구 필요'],
    ['teamGroups', '파트 분류', '멤버를 파트로 묶어 필터·그룹핑 — 아래 팀/그룹 설정 필요'],
  ]

  const INPUT =
    'w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent'
  const ADD_BTN =
    'self-start rounded-md border border-border-strong px-2 py-1 text-[11px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
  const DEL_BTN =
    'w-6 flex-none text-[12px] text-text-muted transition-colors hover:text-status-reopen'

  interface Kv {
    k: string
    v: string
  }
  interface GroupRow {
    key: string
    label: string
    color: string
  }
  interface ProductRow {
    group: string
    key: string
    label: string
  }
  interface RuleRow {
    group: string
    projects: string
    labels: string
    components: string
  }
  interface MemberRow {
    email: string
    name: string
    display_name: string
    group: string
    department: string
    job_role: string
    jira_account_id: string
    avatar_url: string
  }

  let loading = $state(true)
  let saving = $state(false)
  let error = $state<string | null>(null)
  let tab = $state<Tab>('sync')

  let projectsText = $state('')
  let staleText = $state('72')
  let qaDashboardUrl = $state('')
  let features = $state<Record<keyof ScryFeatures, boolean>>({
    presence: false,
    feed: false,
    push: false,
    deploy: false,
    qa: false,
    teamGroups: false,
  })
  let groupRows = $state<GroupRow[]>([])
  let productRows = $state<ProductRow[]>([])
  let ruleRows = $state<RuleRow[]>([])
  let memberRows = $state<MemberRow[]>([])
  let openMember = $state<number | null>(null)
  let fieldMapRows = $state<Kv[]>([])
  let editableRows = $state<Kv[]>([])
  let bodyFieldsText = $state('')

  let jsonText = $state('')
  let jsonError = $state<string | null>(null)

  const splitCsv = (s: string): string[] => s.split(',').map((v) => v.trim()).filter(Boolean)
  const joinCsv = (a: string[] | undefined): string => (a ?? []).join(', ')
  const kvRows = (r: Record<string, string> | undefined): Kv[] =>
    Object.entries(r ?? {}).map(([k, v]) => ({ k, v }))

  function rec(rows: Kv[]): Record<string, string> {
    const out: Record<string, string> = {}
    for (const row of rows) {
      const k = row.k.trim()
      if (k) out[k] = row.v.trim()
    }
    return out
  }

  /** 서버 응답(또는 JSON textarea)을 폼 상태로 펼친다. */
  function load(s: ScrySettings) {
    projectsText = joinCsv(s.projects)
    staleText = String(s.staleThresholdHours ?? 72)
    qaDashboardUrl = s.qaDashboardUrl ?? ''
    features = { ...features, ...(s.features ?? {}) }

    const groupKeys = [
      ...new Set([...Object.keys(s.groupLabels ?? {}), ...Object.keys(s.groupColors ?? {})]),
    ]
    groupRows = groupKeys.map((key) => ({
      key,
      label: s.groupLabels?.[key] ?? '',
      color: s.groupColors?.[key] ?? '',
    }))
    productRows = Object.entries(s.productByGroup ?? {}).map(([group, p]) => ({
      group,
      key: p?.key ?? '',
      label: p?.label ?? '',
    }))
    ruleRows = (s.groupRules ?? []).map((r) => ({
      group: r.group ?? '',
      projects: joinCsv(r.projects),
      labels: joinCsv(r.labels),
      components: joinCsv(r.components),
    }))
    memberRows = (s.members ?? []).map((m) => ({
      email: m.email ?? '',
      name: m.name ?? '',
      display_name: m.display_name ?? '',
      group: m.group ?? '',
      department: m.department ?? '',
      job_role: m.job_role ?? '',
      jira_account_id: m.jira_account_id ?? '',
      avatar_url: m.avatar_url ?? '',
    }))
    openMember = null

    fieldMapRows = kvRows(s.fieldMap)
    editableRows = kvRows(s.editableFields)
    bodyFieldsText = joinCsv(s.bodyFields)
  }

  /** 폼 상태 → PUT 페이로드(전체 교체). */
  function build(): ScrySettings {
    const groupLabels: Record<string, string> = {}
    const groupColors: Record<string, string> = {}
    for (const row of groupRows) {
      const key = row.key.trim()
      if (!key) continue
      if (row.label.trim()) groupLabels[key] = row.label.trim()
      if (row.color.trim()) groupColors[key] = row.color.trim()
    }
    const productByGroup: Record<string, { key: string; label: string }> = {}
    for (const row of productRows) {
      const group = row.group.trim()
      if (group) productByGroup[group] = { key: row.key.trim(), label: row.label.trim() }
    }
    const hours = Number(staleText)
    return {
      projects: splitCsv(projectsText),
      staleThresholdHours: Number.isFinite(hours) && hours > 0 ? hours : 72,
      qaDashboardUrl: qaDashboardUrl.trim(),
      features: { ...features },
      groupLabels,
      groupColors,
      productByGroup,
      groupRules: ruleRows
        .filter((r) => r.group.trim())
        .map((r) => ({
          group: r.group.trim(),
          projects: splitCsv(r.projects),
          labels: splitCsv(r.labels),
          components: splitCsv(r.components),
        })),
      members: memberRows
        .filter((m) => m.email.trim())
        .map((m) => ({ ...m, email: m.email.trim() })),
      fieldMap: rec(fieldMapRows),
      editableFields: rec(editableRows),
      bodyFields: splitCsv(bodyFieldsText),
    }
  }

  onMount(async () => {
    try {
      load(await api.getSettings())
    } catch (e) {
      error = e instanceof Error ? e.message : '설정을 불러오지 못했습니다.'
    }
    loading = false
  })

  function refreshJson() {
    jsonText = JSON.stringify(build(), null, 2)
    jsonError = null
  }

  function applyJson(text: string) {
    jsonText = text
    try {
      load(JSON.parse(text) as ScrySettings)
      jsonError = null
    } catch {
      jsonError = 'JSON 파싱 실패 — 고치면 저장이 다시 활성화됩니다.'
    }
  }

  async function save() {
    if (saving || jsonError) return
    saving = true
    error = null
    try {
      await api.putSettings(build())
      write.toast('설정을 저장했습니다. 새로고침합니다…', 'success')
      setTimeout(() => location.reload(), 600)
    } catch (e) {
      error = e instanceof Error ? e.message : '설정 저장에 실패했습니다.'
      saving = false
    }
  }

  function openJiraKey() {
    onclose()
    write.openSettings()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    class="anim-enter flex max-h-[88vh] w-full max-w-3xl flex-col rounded-lg border border-border-strong bg-bg-panel shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label="설정"
  >
    <!-- 헤더 + 탭 -->
    <div class="flex-none border-b border-border-subtle px-5 pt-4">
      <h2 class="mb-0.5 text-[14px] font-semibold text-text-primary">설정</h2>
      <p class="mb-3 text-[11px] text-text-muted">
        서버의 <span class="font-mono">~/.scry/config.json</span> 을 직접 편집합니다. 저장하면 앱이
        새로고침됩니다.
      </p>
      <div class="flex gap-1">
        {#each TABS as [id, label] (id)}
          <button
            type="button"
            class="-mb-px border-b-2 px-2.5 py-1.5 text-[12px] transition-colors {tab === id
              ? 'border-accent text-text-primary'
              : 'border-transparent text-text-secondary hover:text-text-primary'}"
            onclick={() => (tab = id)}
          >
            {label}
          </button>
        {/each}
      </div>
    </div>

    <!-- 본문 -->
    <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-[12px]">
      {#if loading}
        <p class="py-8 text-center text-text-muted">불러오는 중…</p>
      {:else if tab === 'sync'}
        <div class="flex flex-col gap-4">
          <label class="flex flex-col gap-1">
            <span class="text-[11px] text-text-secondary">미러링할 프로젝트 키 (콤마 구분)</span>
            <input class={INPUT} bind:value={projectsText} placeholder="NMB, NMA" />
          </label>
          <label class="flex max-w-[200px] flex-col gap-1">
            <span class="text-[11px] text-text-secondary">지연 판정 기준 (시간)</span>
            <input class={INPUT} type="number" min="1" bind:value={staleText} />
            <span class="text-[11px] text-text-muted">
              현재 상태에 이 시간 이상 머문 미해결 이슈를 지연으로 표시합니다.
            </span>
          </label>
          <div class="border-t border-border-subtle pt-3">
            <button
              type="button"
              class={ADD_BTN}
              onclick={openJiraKey}
            >
              개인 Jira API 토큰 설정 →
            </button>
            <p class="mt-1 text-[11px] text-text-muted">
              자격증명은 이 화면이 아니라 별도 다이얼로그에서 관리합니다.
            </p>
          </div>
        </div>
      {:else if tab === 'features'}
        <div class="flex flex-col gap-2.5">
          {#each FEATURES as [key, label, hint] (key)}
            <label class="flex cursor-pointer items-start gap-2.5">
              <input
                type="checkbox"
                class="mt-0.5 flex-none accent-[var(--color-accent,#3b82f6)]"
                bind:checked={features[key]}
              />
              <span class="min-w-0">
                <span class="text-text-primary">{label}</span>
                <span class="block text-[11px] leading-relaxed text-text-muted">{hint}</span>
              </span>
            </label>
          {/each}
          <label class="mt-2 flex flex-col gap-1 border-t border-border-subtle pt-3">
            <span class="text-[11px] text-text-secondary">QA 대시보드 URL (선택)</span>
            <input class={INPUT} bind:value={qaDashboardUrl} placeholder="https://qa.example.com" />
          </label>
        </div>
      {:else if tab === 'groups'}
        <div class="flex flex-col gap-5">
          <!-- 그룹 라벨 + 색상 -->
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              그룹 라벨·색상
            </div>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
              <span class="flex-1">그룹 키</span>
              <span class="flex-1">라벨</span>
              <span class="w-16 flex-none">색상</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each groupRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="cloud" />
                <input class="{INPUT} flex-1" bind:value={row.label} placeholder="Cloud 파트" />
                <input
                  type="color"
                  class="h-[26px] w-16 flex-none rounded-md border border-border-strong bg-bg-base"
                  value={row.color || '#888888'}
                  oninput={(e) => (row.color = e.currentTarget.value)}
                  title={row.color || '미지정'}
                />
                <button
                  type="button"
                  class={DEL_BTN}
                  title="행 삭제"
                  onclick={() => (groupRows = groupRows.filter((_, j) => j !== i))}>✕</button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() => (groupRows = [...groupRows, { key: '', label: '', color: '' }])}
              >+ 행 추가</button
            >
          </div>

          <!-- 제품 버킷 -->
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              그룹 → 제품
            </div>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
              <span class="flex-1">그룹 키</span>
              <span class="flex-1">제품 키</span>
              <span class="flex-1">제품 라벨</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each productRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1 font-mono" bind:value={row.group} placeholder="cloud" />
                <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="CLOUD" />
                <input class="{INPUT} flex-1" bind:value={row.label} placeholder="Cloud" />
                <button
                  type="button"
                  class={DEL_BTN}
                  title="행 삭제"
                  onclick={() => (productRows = productRows.filter((_, j) => j !== i))}>✕</button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() =>
                (productRows = [...productRows, { group: '', key: '', label: '' }])}
              >+ 행 추가</button
            >
          </div>

          <!-- 그룹 판정 규칙 -->
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              그룹 판정 규칙
            </div>
            <p class="text-[11px] leading-relaxed text-text-muted">
              위에서 아래로 <span class="text-text-secondary">첫 매치가 이깁니다</span>. 한 행의
              조건들은 AND, 각 목록 안은 OR, 빈 조건은 항상 참입니다.
            </p>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
              <span class="w-24 flex-none">그룹</span>
              <span class="flex-1">프로젝트</span>
              <span class="flex-1">라벨</span>
              <span class="flex-1">컴포넌트</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each ruleRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} w-24 flex-none font-mono" bind:value={row.group} placeholder="cloud" />
                <input class="{INPUT} flex-1" bind:value={row.projects} placeholder="NMA, NMB" />
                <input class="{INPUT} flex-1" bind:value={row.labels} placeholder="backend" />
                <input class="{INPUT} flex-1" bind:value={row.components} placeholder="api" />
                <button
                  type="button"
                  class={DEL_BTN}
                  title="행 삭제"
                  onclick={() => (ruleRows = ruleRows.filter((_, j) => j !== i))}>✕</button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() =>
                (ruleRows = [
                  ...ruleRows,
                  { group: '', projects: '', labels: '', components: '' },
                ])}>+ 행 추가</button
            >
          </div>
        </div>
      {:else if tab === 'members'}
        <div class="flex flex-col gap-1.5">
          <div class="flex gap-1.5 text-[11px] text-text-muted">
            <span class="flex-1">이메일</span>
            <span class="w-24 flex-none">이름</span>
            <span class="w-20 flex-none">그룹</span>
            <span class="flex-1">Jira accountId</span>
            <span class="w-12 flex-none"></span>
          </div>
          {#each memberRows as row, i (i)}
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1" bind:value={row.email} placeholder="a@b.c" />
                <input class="{INPUT} w-24 flex-none" bind:value={row.name} />
                <input class="{INPUT} w-20 flex-none font-mono" bind:value={row.group} />
                <input class="{INPUT} flex-1 font-mono" bind:value={row.jira_account_id} />
                <button
                  type="button"
                  class="w-6 flex-none text-[12px] text-text-muted transition-colors hover:text-text-primary"
                  title="상세"
                  onclick={() => (openMember = openMember === i ? null : i)}
                  >{openMember === i ? '▾' : '▸'}</button
                >
                <button
                  type="button"
                  class={DEL_BTN}
                  title="행 삭제"
                  onclick={() => {
                    memberRows = memberRows.filter((_, j) => j !== i)
                    openMember = null
                  }}>✕</button
                >
              </div>
              {#if openMember === i}
                <div class="ml-2 grid grid-cols-2 gap-1.5 border-l border-border-subtle pl-3 pb-1">
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">표시 이름</span>
                    <input class={INPUT} bind:value={row.display_name} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">부서</span>
                    <input class={INPUT} bind:value={row.department} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">직무</span>
                    <input class={INPUT} bind:value={row.job_role} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">아바타 URL</span>
                    <input class={INPUT} bind:value={row.avatar_url} />
                  </label>
                </div>
              {/if}
            </div>
          {/each}
          <button
            type="button"
            class={ADD_BTN}
            onclick={() =>
              (memberRows = [
                ...memberRows,
                {
                  email: '',
                  name: '',
                  display_name: '',
                  group: '',
                  department: '',
                  job_role: '',
                  jira_account_id: '',
                  avatar_url: '',
                },
              ])}>+ 멤버 추가</button
          >
        </div>
      {:else}
        <div class="flex flex-col gap-5">
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              필드 맵 (동기화 적재)
            </div>
            <KeyValueRows
              bind:rows={fieldMapRows}
              keyLabel="별칭"
              valueLabel="Jira 필드 id"
              keyPlaceholder="severity"
              valuePlaceholder="customfield_10050"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              인라인 편집 허용 필드
            </div>
            <KeyValueRows
              bind:rows={editableRows}
              keyLabel="별칭"
              valueLabel="Jira 필드 id"
              keyPlaceholder="solution"
              valuePlaceholder="customfield_10092"
            />
          </div>
          <label class="flex flex-col gap-1">
            <span class="text-[11px] text-text-secondary">
              본문 검색에 합칠 ADF 커스텀필드 id (콤마 구분)
            </span>
            <input class="{INPUT} font-mono" bind:value={bodyFieldsText} placeholder="customfield_10101" />
          </label>
        </div>
      {/if}

      <!-- 고급: JSON 원본 -->
      {#if !loading}
        <details
          class="mt-5 border-t border-border-subtle pt-3"
          ontoggle={(e) => {
            if (e.currentTarget.open) refreshJson()
          }}
        >
          <summary class="cursor-pointer text-[11px] text-text-secondary hover:text-text-primary">
            고급 — JSON 직접 편집
          </summary>
          <textarea
            class="mt-2 h-56 w-full rounded-md border border-border-strong bg-bg-base p-2 font-mono text-[11px] text-text-primary outline-none focus:border-accent"
            spellcheck="false"
            value={jsonText}
            oninput={(e) => applyJson(e.currentTarget.value)}
          ></textarea>
          {#if jsonError}
            <p class="mt-1 text-[11px] text-status-reopen">{jsonError}</p>
          {:else}
            <p class="mt-1 text-[11px] text-text-muted">
              폼과 JSON 은 마지막 수정이 이깁니다. 펼칠 때마다 현재 폼 값으로 다시 채워집니다.
            </p>
          {/if}
        </details>
      {/if}
    </div>

    <!-- 푸터 -->
    <div class="flex flex-none items-center justify-between gap-2 border-t border-border-subtle px-5 py-3">
      <span class="min-w-0 flex-1 truncate text-[12px] text-status-reopen">{error ?? ''}</span>
      <button
        type="button"
        onclick={onclose}
        class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
      >
        닫기
      </button>
      <button
        type="button"
        onclick={save}
        disabled={loading || saving || !!jsonError}
        class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
      >
        {saving ? '저장 중…' : '저장'}
      </button>
    </div>
  </div>
</div>
