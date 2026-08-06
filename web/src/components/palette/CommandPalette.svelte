<script lang="ts">
  /*
   * Command palette (⌘K/Ctrl+K) — issue jump / apply view / run action.
   *  - Matching is all local on the memory pool (zero network). Issues reuse list
   *    filterIssues + relevance sort, so chosung search and key short-forms work.
   *  - Items are a flat array in section order → ↑↓ moves a single index.
   *  - Open/close and key bindings live in App.svelte (must open even while focused).
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
  import { bulk } from '../../stores/bulk.svelte'
  import { triage } from '../../stores/triage.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import type { IssueLite, PageLite } from '../../lib/types'

  let { onclose, onOpenSettings }: { onclose: () => void; onOpenSettings: () => void } = $props()

  type Section = 'issue' | 'view' | 'action'

  interface Item {
    id: string
    section: Section
    /** Body used for match + display. */
    label: string
    icon?: string
    /** Leading chip — what kind of thing the row opens (documents only; an
     *  issue says so with its monospace key). */
    badge?: string
    /** Right-side secondary text (issue title / view source / shortcut). */
    sub?: string
    kbd?: string
    mono?: boolean
    testid?: string
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

  /** Match view/action names — substring + chosung query. */
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

  /** A mirrored page reads as its title; the badge carries the kind, since a
   *  page key is an opaque id nobody recognizes. */
  function docItem(page: PageLite): Item {
    return {
      id: `d:${page.key}`,
      section: 'issue',
      label: page.title,
      badge: t('doc.badge'),
      sub: pages.spaceLabel(page.space_key),
      testid: 'palette-doc-row',
      run: () => pages.select(page.key),
    }
  }

  const issueItems = $derived.by<Item[]>(() => {
    // Empty input = recently opened issues and documents, in visit order.
    if (!needle) {
      const out: Item[] = []
      for (const visit of me.recent) {
        if (visit.kind === 'doc') {
          const page = pages.byKey.get(visit.key)
          if (page) out.push(docItem(page))
        } else {
          const issue = issues.pool.get(visit.key)
          if (issue) out.push(issueItem(issue))
        }
        if (out.length >= 5) break
      }
      return out
    }
    const f = emptyFilters()
    f.q = raw
    // Relevance context matches the list (match strength + recency + personalization).
    const ctx: RelevanceContext = {
      needle,
      chosungQuery,
      now: Date.now(),
      myEmail: me.email,
      recentKeys: new Set(me.recentIssues.map((v) => v.key)),
    }
    return sortIssues(filterIssues(issues.allIssues, f), 'relevance', 'desc', ctx)
      .slice(0, 8)
      .map(issueItem)
  })

  function applyView(config: ViewConfig) {
    me.closeFeed()
    pages.closeDocs()
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

  /**
   * Triage commands carry their target in the label — "Change status · 3
   * selected" vs "· NMB-110" — because the palette covers the list while it is
   * open, and a bare "Change status" would not say what it is about to change.
   * They only appear when there is something to act on.
   */
  const triageItems = $derived.by<Omit<Item, 'section'>[]>(() => {
    const count = bulk.count
    const cursor = triage.listActive ? triage.cursorKey : null
    if (!count && !cursor) return []
    const target = count ? t('palette.triageSelected', { n: count }) : (cursor as string)
    const out: Omit<Item, 'section'>[] = [
      {
        id: 'a:triage-status',
        label: t('palette.actionTriageStatus', { target }),
        kbd: 's',
        run: () => triage.requestMenu('status'),
      },
      {
        id: 'a:triage-assignee',
        label: t('palette.actionTriageAssignee', { target }),
        kbd: 'a',
        run: () => triage.requestMenu('assignee'),
      },
    ]
    if (cursor) {
      out.push({
        id: 'a:triage-comment',
        label: t('palette.actionTriageComment', { key: cursor }),
        kbd: 'c',
        run: () => triage.openComment(cursor),
      })
      out.push({
        id: 'a:triage-select',
        label: bulk.has(cursor)
          ? t('palette.actionTriageDeselect', { key: cursor })
          : t('palette.actionTriageSelect', { key: cursor }),
        kbd: 'x',
        run: () => bulk.toggle(cursor),
      })
    }
    if (count) {
      out.push({
        id: 'a:triage-clear',
        label: t('palette.actionTriageClear', { n: count }),
        kbd: 'Esc',
        run: () => bulk.clear(),
      })
    }
    return out
  })

  const actionItems = $derived.by<Item[]>(() => {
    // `c` belongs to the cursor row while there is one, so the badge moves with
    // it — two rows both claiming `c` would be a lie about one of them.
    const cIsComment = triageItems.some((d) => d.id === 'a:triage-comment')
    const defs: Omit<Item, 'section'>[] = [
      ...triageItems,
      {
        id: 'a:new',
        label: t('palette.actionNewIssue'),
        kbd: cIsComment ? undefined : 'c',
        run: () => void write.openNewIssue(),
      },
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
      {
        id: 'a:sync-now',
        label: t('palette.actionSyncNow'),
        run: () => void runSyncNow('incremental'),
      },
    ]
    return defs.filter((d) => matches(d.label)).map((d) => ({ ...d, section: 'action' as const }))
  })

  const items = $derived([...issueItems, ...viewItems, ...actionItems])

  const SECTION_LABEL: Record<Section, string> = {
    issue: t('palette.sectionIssues'),
    view: t('palette.sectionViews'),
    action: t('palette.sectionActions'),
  }

  // Reset highlight to the first item when candidates change.
  $effect(() => {
    void items
    idx = 0
  })

  // Keep the highlight inside the viewport.
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
          data-testid={item.testid}
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
          {#if item.badge}
            <span
              class="flex-none rounded bg-bg-active px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-text-muted"
            >
              {item.badge}
            </span>
          {/if}
          <!-- A document leads with its title, so the title is what gives up
               width; an issue leads with its key and lets the summary truncate. -->
          <span
            class="{item.badge ? 'min-w-0 flex-1 truncate' : 'flex-none'} {item.mono
              ? 'font-mono text-accent-text'
              : ''}"
          >
            {item.label}
          </span>
          {#if item.sub}
            <span
              class="truncate text-text-muted {item.badge
                ? 'max-w-[35%] flex-none'
                : 'min-w-0 flex-1'}">{item.sub}</span
            >
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
