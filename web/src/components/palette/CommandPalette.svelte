<script lang="ts">
  /*
   * 커맨드 팔레트(⌘K/Ctrl+K) — 이슈 점프 / 뷰 적용 / 액션 실행.
   *  - 매칭은 전부 메모리 풀 로컬 연산(네트워크 0). 이슈는 리스트와 같은 filterIssues +
   *    관련도 정렬을 재사용하므로 초성 검색·키 단축형이 그대로 동작한다.
   *  - 항목은 섹션 순서대로 이어붙인 평면 배열 → ↑↓ 는 단일 인덱스만 움직인다.
   *  - 열림/닫힘과 키 바인딩은 App.svelte 소관(입력 필드 포커스 중에도 열려야 하므로).
   */
  import { onMount } from 'svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import {
    formatTimeOfDay,
    locale,
    setLocale,
    t,
  } from '../../lib/i18n'
  import { extractChosung, isChosungQuery } from '../../lib/korean'
  import { builtinViews } from '../../lib/builtin-views'
  import { emptyFilters, type ViewConfig } from '../../lib/view-config'
  import {
    filterIssues,
    filters,
    sortIssues,
    type RelevanceContext,
  } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import type { IssueLite } from '../../lib/types'

  let { onclose, onOpenSettings }: { onclose: () => void; onOpenSettings: () => void } = $props()

  type Section = 'issue' | 'view' | 'action'

  interface Item {
    id: string
    section: Section
    /** 매칭·표시에 쓰는 본문. */
    label: string
    icon?: string
    /** 우측 보조 텍스트(이슈 제목 / 뷰 출처 / 단축키). */
    sub?: string
    kbd?: string
    mono?: boolean
    run: () => void
  }

  let query = $state('')
  let idx = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)
  let listEl = $state<HTMLElement | null>(null)

  onMount(() => inputEl?.focus())

  const raw = $derived(query.trim())
  const needle = $derived(raw.toLowerCase())
  const chosungQuery = $derived(raw ? isChosungQuery(raw) : false)

  /** 뷰/액션 이름 매칭 — 부분일치 + 초성 쿼리. */
  function matches(text: string): boolean {
    if (!needle) return true
    if (text.toLowerCase().includes(needle)) return true
    return chosungQuery && extractChosung(text).includes(needle)
  }

  function issueItem(issue: IssueLite): Item {
    return {
      id: `i:${issue.issue_key}`,
      section: 'issue',
      label: issue.issue_key,
      sub: issue.summary,
      mono: true,
      run: () => selection.select(issue.issue_key),
    }
  }

  const issueItems = $derived.by<Item[]>(() => {
    // 빈 입력 = 최근 본 이슈(풀에 남아 있는 것만).
    if (!needle) {
      const out: Item[] = []
      for (const visit of me.recent) {
        const issue = issues.pool.get(visit.key)
        if (issue) out.push(issueItem(issue))
        if (out.length >= 5) break
      }
      return out
    }
    const f = emptyFilters()
    f.q = raw
    // 관련도 컨텍스트는 리스트와 같은 규칙(매칭 강도 + 최근성 + 개인화).
    const ctx: RelevanceContext = {
      needle,
      chosungQuery,
      now: Date.now(),
      myEmail: me.email,
      recentKeys: new Set(me.recent.map((v) => v.key)),
    }
    return sortIssues(filterIssues(issues.allIssues, f), 'relevance', 'desc', ctx)
      .slice(0, 8)
      .map(issueItem)
  })

  function applyView(config: ViewConfig) {
    me.closeFeed()
    filters.applyConfig(config)
  }

  const viewItems = $derived.by<Item[]>(() => {
    const out: Item[] = []
    const push = (id: string, name: string, sub: string, config: ViewConfig, icon?: string) => {
      if (matches(name)) out.push({ id, section: 'view', label: name, sub, icon, run: () => applyView(config) })
    }
    for (const v of builtinViews()) push(`vb:${v.id}`, v.name, t('palette.viewBuiltin'), v.config, v.icon)
    for (const v of views.personal) push(`vp:${v.id}`, v.name, t('palette.viewPersonal'), v.config)
    for (const v of views.team)
      push(`vt:${v.id}`, v.name, t('palette.viewTeam'), v.config as unknown as ViewConfig)
    return out
  })

  function syncStatusToast() {
    const overall = issues.syncHealth?.overall
    const label =
      overall === 'failed'
        ? t('sidebar.syncFailTitle')
        : overall === 'warning'
          ? t('sidebar.syncDelayedTitle')
          : t('sidebar.syncOk')
    const when = issues.lastSync ? formatTimeOfDay(issues.lastSync) : t('common.unknown')
    write.toast(t('palette.syncToast', { overall: label, when }), overall === 'failed' ? 'error' : 'info')
  }

  const actionItems = $derived.by<Item[]>(() => {
    const defs: Omit<Item, 'section'>[] = [
      { id: 'a:new', label: t('palette.actionNewIssue'), kbd: 'c', run: () => void write.openNewIssue() },
      { id: 'a:settings', label: t('palette.actionSettings'), run: onOpenSettings },
      { id: 'a:reset', label: t('palette.actionResetFilters'), run: () => filters.clearAll() },
      { id: 'a:reopened', label: t('palette.actionToggleReopened'), run: () => filters.toggleFlag('reopened') },
      { id: 'a:unassigned', label: t('palette.actionToggleUnassigned'), run: () => filters.toggleFlag('unassigned') },
      { id: 'a:stale', label: t('palette.actionToggleStale'), run: () => filters.toggleFlag('stale') },
      {
        id: 'a:locale',
        label: t('palette.actionLocale', {
          lang: locale() === 'ko' ? t('settings.localeEn') : t('settings.localeKo'),
        }),
        run: () => setLocale(locale() === 'ko' ? 'en' : 'ko'),
      },
      { id: 'a:sync', label: t('palette.actionSyncStatus'), run: syncStatusToast },
    ]
    return defs.filter((d) => matches(d.label)).map((d) => ({ ...d, section: 'action' as const }))
  })

  const items = $derived([...issueItems, ...viewItems, ...actionItems])

  const SECTION_LABEL: Record<Section, string> = {
    issue: t('palette.sectionIssues'),
    view: t('palette.sectionViews'),
    action: t('palette.sectionActions'),
  }

  // 후보가 바뀌면 하이라이트를 처음으로.
  $effect(() => {
    void items
    idx = 0
  })

  // 하이라이트가 뷰포트 밖으로 나가지 않게.
  $effect(() => {
    listEl?.querySelector(`[data-idx="${idx}"]`)?.scrollIntoView({ block: 'nearest' })
  })

  function run(item: Item) {
    item.run()
    onclose()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onclose()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (items.length) idx = (idx + 1) % items.length
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (items.length) idx = (idx - 1 + items.length) % items.length
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const item = items[idx]
      if (item) run(item)
    }
  }
</script>

<div
  class="fixed inset-0 z-50 flex items-start justify-center bg-black/60 p-4 pt-[12vh]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    use:trapFocus
    class="anim-pop flex max-h-[70vh] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-border-strong bg-bg-panel shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label={t('palette.title')}
  >
    <div class="flex h-11 flex-none items-center gap-2 border-b border-border-subtle px-3">
      <span class="flex-none text-text-muted" aria-hidden="true">⌕</span>
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={onKeydown}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls="palette-list"
        aria-autocomplete="list"
        aria-activedescendant={items.length ? `palette-opt-${idx}` : undefined}
        placeholder={t('palette.placeholder')}
        class="min-w-0 flex-1 bg-transparent text-[13px] text-text-primary placeholder:text-text-muted focus:outline-none"
        spellcheck="false"
        autocomplete="off"
      />
    </div>

    <div
      bind:this={listEl}
      id="palette-list"
      role="listbox"
      aria-label={t('palette.title')}
      class="min-h-0 flex-1 overflow-y-auto p-1"
    >
      {#if items.length === 0}
        <p class="px-2 py-6 text-center text-[12px] text-text-muted">{t('palette.empty')}</p>
      {/if}
      {#each items as item, i (item.id)}
        {#if i === 0 || items[i - 1].section !== item.section}
          <div
            role="presentation"
            class="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wide text-text-muted"
          >
            {item.section === 'issue' && !needle ? t('palette.recent') : SECTION_LABEL[item.section]}
          </div>
        {/if}
        <button
          type="button"
          role="option"
          id="palette-opt-{i}"
          data-idx={i}
          aria-selected={i === idx}
          class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-[12px] {i === idx
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover'}"
          onmousemove={() => (idx = i)}
          onmousedown={(e) => {
            e.preventDefault()
            run(item)
          }}
        >
          {#if item.icon}<span class="flex-none" aria-hidden="true">{item.icon}</span>{/if}
          <span class="flex-none {item.mono ? 'font-mono text-accent-text' : ''}">{item.label}</span>
          {#if item.sub}
            <span class="min-w-0 flex-1 truncate text-text-muted">{item.sub}</span>
          {/if}
          {#if item.kbd}
            <span class="ml-auto flex-none">
              <kbd class="rounded border border-border-subtle px-1 text-[10px] text-text-muted">
                {item.kbd}
              </kbd>
            </span>
          {/if}
        </button>
      {/each}
    </div>

    <div
      class="flex-none border-t border-border-subtle px-3 py-1.5 text-[11px] text-text-muted"
    >
      {t('palette.hintNav')} · <kbd class="font-mono">?</kbd> {t('palette.hintHelp')}
    </div>
  </div>
</div>
