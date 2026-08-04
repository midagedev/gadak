<script lang="ts">
  /*
   * 앱 셸. 3컬럼 레이아웃 골격 + 부팅 상태 분기.
   *  [explore] 배선: 사이드바=SidebarNav, 메인=ListView, 우측 패널 open=선택 이슈 여부.
   *  RightPanel children 은 [detail] 이 마운트하므로 여기선 Placeholder 유지(오케스트레이터가 교체).
   *  선택 이슈 ↔ URL(?issue=KEY) 양방향 동기화도 여기서 담당(계약 §2 selection).
   */
  import { onMount, untrack } from 'svelte'
  import { issues } from './stores/issues.svelte'
  import { views } from './stores/views.svelte'
  import { selection } from './stores/selection.svelte'
  import { filters } from './stores/filters.svelte'
  import { me } from './stores/me.svelte'
  import { presence } from './stores/presence.svelte'
  import { write } from './stores/write.svelte'
  import { router, setParams } from './lib/router.svelte'
  import { feature } from './lib/config'
  import {
    emptyConfig,
    parseConfig,
    VIEW_PARAM_KEYS,
    type ViewConfig,
  } from './lib/view-config'
  import Sidebar from './components/shell/Sidebar.svelte'
  import MainColumn from './components/shell/MainColumn.svelte'
  import RightPanel from './components/shell/RightPanel.svelte'
  import LoadingShell from './components/shell/LoadingShell.svelte'
  import AuthGate from './components/shell/AuthGate.svelte'
  import SidebarNav from './components/sidebar/SidebarNav.svelte'
  import ListView from './components/list/ListView.svelte'
  import DetailPanel from './components/detail/DetailPanel.svelte'
  import PersonalFeed from './components/personal/PersonalFeed.svelte'
  import LoginDialog from './components/personal/LoginDialog.svelte'
  import NewIssueDialog from './components/write/NewIssueDialog.svelte'
  import JiraKeySettings from './components/write/JiraKeySettings.svelte'
  import SettingsDialog from './components/settings/SettingsDialog.svelte'
  import ToastHost from './components/write/ToastHost.svelte'
  import MediaViewer from './components/detail/MediaViewer.svelte'
  import { mediaViewer } from './stores/media-viewer.svelte'

  const LAST_VIEW_KEY = 'issue-nav:last-view'

  /** 서버 설정 다이얼로그(사이드바 톱니). 스토어를 새로 만들 이유가 없어 셸 로컬 상태. */
  let serverSettingsOpen = $state(false)

  // 공유 링크/대시보드/푸시에서 직접 진입한 이슈를 첫 렌더 전에 복원한다.
  // 그렇지 않으면 selection → URL 이펙트가 빈 선택으로 `issue`를 먼저 지울 수 있다.
  const initialIssueKey = router.params.get('issue')
  if (initialIssueKey) selection.select(initialIssueKey)
  let syncedIssueKey = initialIssueKey

  onMount(() => {
    void issues.init()
    void me.init()
    void write.loadWriteMeta() // 쓰기 메타 선반영(issues.init 과 병렬)
    views.init()
    if (feature('presence')) presence.init() // 실시간 프레즌스(티켓→WS, 실패는 조용히)
  })

  // ── 상세 열림/닫힘을 프레즌스에 반영 ──
  //  selectedKey 만 추적 의존성. setViewing 은 반응형 상태를 읽지 않으므로 루프 없음.
  $effect(() => {
    if (!feature('presence')) return
    presence.setViewing(selection.selectedKey)
  })

  function retry() {
    void issues.refresh()
  }

  // ── 최근 본 이슈 기록(선택 시) ──
  //  untrack 필수: recordRecent 가 me.recent 를 읽고+쓰므로 추적되면 무한 이펙트 루프.
  $effect(() => {
    const key = selection.selectedKey
    if (key) untrack(() => me.recordRecent(key))
  })

  // 이슈를 어떤 경로(리스트/피드/푸시)로 열든 해당 이슈의 새 활동을 읽음 처리.
  $effect(() => {
    const key = selection.selectedKey
    const authed = me.authed
    if (key && authed) untrack(() => void me.markIssueRead(key))
  })

  // ── 로그인 상태 ↔ Jira 자격증명 로드/리셋 (사이드바 ⚙︎ 표시·쓰기 게이트용) ──
  $effect(() => {
    if (me.authed) {
      void write.loadCredential()
    } else if (me.authChecked) {
      write.resetCredential()
    }
  })

  // ── 전역 단축키: c = 새 이슈 (입력 필드 포커스/다이얼로그 열림 중엔 무시) ──
  function onGlobalKey(e: KeyboardEvent) {
    if (e.key !== 'c' || e.metaKey || e.ctrlKey || e.altKey) return
    const t = e.target as HTMLElement | null
    if (t) {
      const tag = t.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || t.isContentEditable) return
    }
    if (me.loginOpen || write.settingsOpen || write.newIssueOpen || serverSettingsOpen) return
    e.preventDefault()
    write.openNewIssue()
  }

  // ── 스마트 기본값: 최초 1회. URL 에 뷰 파람이 있으면 절대 덮지 않는다. ──
  //  우선순위: URL > 마지막 사용 뷰(localStorage) > 내 소속 파트 프리셋.
  //  데이터(members)·인증확인이 끝나야 파트 매칭이 가능하므로 그 시점까지 기다린다.
  let startupDone = false
  $effect(() => {
    if (startupDone) return
    if (!issues.ready || !me.authChecked) return
    startupDone = true
    applyStartupView()
  })

  // ── 마지막 사용 뷰 저장(스마트 기본값 적용 이후, 뷰 변경마다) ──
  $effect(() => {
    const vk = filters.viewKey
    if (!startupDone) return
    try {
      localStorage.setItem(LAST_VIEW_KEY, vk)
    } catch {
      /* noop */
    }
  })

  function applyStartupView() {
    // URL 이 뷰를 지정했으면(공유 링크/새로고침) 그대로 존중.
    if (VIEW_PARAM_KEYS.some((k) => router.params.get(k))) return

    // 1) 마지막 사용 뷰 복원
    let last: string | null = null
    try {
      last = localStorage.getItem(LAST_VIEW_KEY)
    } catch {
      last = null
    }
    if (last) {
      filters.applyConfig(parseConfig(new URLSearchParams(last)))
      return
    }

    // 2) 내 소속 파트 프리셋(파트 분류 사용 + 로그인 + group 존재 시)
    if (feature('teamGroups') && me.group) {
      const c: ViewConfig = emptyConfig()
      c.filters.d1_group = [me.group]
      c.filters.status_category = ['new', 'inprogress']
      filters.applyConfig(c)
      return
    }

    // 비로그인/파트 미지정 사용자는 전체 미해결을 기본으로 본다.
    const c: ViewConfig = emptyConfig()
    c.filters.status_category = ['new', 'inprogress']
    filters.applyConfig(c)
  }

  // ── 선택 이슈 ↔ URL 양방향 동기화 ──
  // 마지막 동기화 값을 기준으로 URL 이동과 사용자 선택 중 어느 쪽이 먼저
  // 바뀌었는지 구분한다. 두 방향을 별도 이펙트로 두면 뒤로가기/딥링크에서
  // 이전 선택이 새 URL을 덮을 수 있다.
  $effect(() => {
    const urlKey = router.params.get('issue')
    const key = selection.selectedKey
    if (urlKey !== syncedIssueKey) {
      syncedIssueKey = urlKey
      if (urlKey) selection.select(urlKey)
      else selection.clear()
      return
    }
    if (key !== syncedIssueKey) {
      syncedIssueKey = key
      setParams({ issue: key }, true)
    }
  })

  const detailOpen = $derived(selection.selectedKey !== null)
</script>

<svelte:window onkeydown={onGlobalKey} />

{#if !issues.ready}
  {#if issues.error === 'auth'}
    <AuthGate onRetry={retry} />
  {:else if issues.error === 'network'}
    <div class="flex h-screen flex-col items-center justify-center gap-4 bg-bg-base px-6 text-center">
      <p class="max-w-sm text-[13px] text-text-secondary">
        데이터를 불러오지 못했습니다. 네트워크/VPN 상태를 확인하세요.
      </p>
      <button
        onclick={retry}
        class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        다시 시도
      </button>
    </div>
  {:else}
    <LoadingShell />
  {/if}
{:else}
  <div class="issue-shell">
    <div
      class="issue-layout"
      class:detail-open={detailOpen}
      data-testid="issue-layout"
      data-detail-open={detailOpen}
    >
      <Sidebar>
        {#snippet children()}
          <SidebarNav onOpenSettings={() => (serverSettingsOpen = true)} />
        {/snippet}
      </Sidebar>

      <MainColumn>
        {#snippet children()}
          {#if me.feedOpen && feature('feed')}
            <PersonalFeed />
          {:else}
            <ListView />
          {/if}
        {/snippet}
      </MainColumn>

      <RightPanel open={detailOpen}>
        {#snippet children()}
          <DetailPanel />
        {/snippet}
      </RightPanel>
    </div>
  </div>
{/if}

{#if me.loginOpen}
  <LoginDialog onClose={() => me.closeLogin()} />
{/if}

{#if write.settingsOpen}
  <JiraKeySettings />
{/if}

{#if write.newIssueOpen}
  <NewIssueDialog />
{/if}

{#if serverSettingsOpen}
  <SettingsDialog onclose={() => (serverSettingsOpen = false)} />
{/if}

<!-- 토스트(우하단) — 항상 마운트 -->
<ToastHost />

{#if mediaViewer.attachment}
  <MediaViewer attachment={mediaViewer.attachment} onClose={() => mediaViewer.close()} />
{/if}
