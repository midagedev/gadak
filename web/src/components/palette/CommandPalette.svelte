<script lang="ts">
  /*
   * Command palette (⌘K/Ctrl+K) — issue jump / apply view / run action / all-search.
   *  - Local sections stay on the memory pool (zero network). Issues reuse list
   *    filterIssues + relevance sort, so chosung search and key short-forms work.
   *  - After a debounce, GET search/?q= fills an All-search section under them
   *    (titles, bodies, comments; list filter chips are not sent).
   *  - Items are a flat array in section order → ↑↓ moves a single index.
   *  - Open/close and key bindings live in App.svelte (must open even while focused).
   */
  import { onMount } from 'svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import {
    formatNumber,
    formatTimeOfDay,
    locale,
    setLocale,
    t,
  } from '../../lib/i18n'
  import { extractChosung, isChosungQuery } from '../../lib/korean'
  import { rankPages } from '../../lib/doc-search'
  import { highlightSegments } from '../../lib/format'
  import { search } from '../../lib/api'
  import { matchEvidence } from '../../lib/search-match'
  import {
    UNIFIED_FETCH_LIMIT,
    createUnifiedSearch,
    emptyUnifiedView,
    projectUnifiedHits,
    type UnifiedView,
  } from '../../lib/unified-search'
  import { builtinViews } from '../../lib/builtin-views'
  import { showIssueList } from '../../lib/show-issue-list'
  import { applyServerSearchOutcome } from '../../lib/server-search'
  import { emptyFilters, type ViewConfig } from '../../lib/view-config'
  import {
    filterIssues,
    filters,
    sortIssues,
    type RelevanceContext,
  } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { person } from '../../stores/person.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import { triage } from '../../stores/triage.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { runSyncNow } from '../../lib/sync-now'
  import { THEME_MODES, setThemePreference } from '../../lib/theme'
  import type { IssueLite, Member, PageLite, SearchMatch } from '../../lib/types'
  import Icon, { type IconName } from '../ui/Icon.svelte'
  import Marks from '../ui/Marks.svelte'

  let { onclose, onOpenSettings }: { onclose: () => void; onOpenSettings: () => void } = $props()

  // 'recent' is the empty-query list, which is issues and documents together in
  // visit order — one group, so neither kind claims the other's section.
  type Section = 'person' | 'recent' | 'doc' | 'issue' | 'view' | 'action' | 'unified'

  interface Item {
    id: string
    section: Section
    /** Body used for match + display. */
    label: string
    icon?: IconName
    /** Label split into matched/unmatched runs. Rows that were found by a query
     *  carry it so the row says why it is here. */
    segs?: { text: string; hit: boolean }[]
    /** Same for the sub line. An issue is found by its summary as often as by
     *  its key, and a doc row right above it marking the word while the issue
     *  row does not reads as two surfaces (vision verdict 2026-08-07). */
    subSegs?: { text: string; hit: boolean }[]
    /** Leading chip — what kind of thing the row opens (documents only; an
     *  issue says so with its monospace key). */
    badge?: string
    /** Right-side secondary text (issue title / view source / shortcut). */
    sub?: string
    kbd?: string
    mono?: boolean
    testid?: string
    /** Server-search evidence (body/comment). Title hits stay on the first line. */
    match?: SearchMatch
    run: () => void
  }

  let query = $state('')
  let idx = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)
  let listEl = $state<HTMLElement | null>(null)
  let serverView = $state<UnifiedView>(emptyUnifiedView())

  const unifiedSession = createUnifiedSearch({
    fetch: (q) => search(q, UNIFIED_FETCH_LIMIT),
    onView: (view) => {
      serverView = view
    },
  })

  onMount(() => {
    inputEl?.focus()
    return () => unifiedSession.cancel()
  })

  const raw = $derived(query.trim())
  const needle = $derived(raw.toLowerCase())
  const chosungQuery = $derived(raw ? isChosungQuery(raw) : false)

  /** Match view/action names — substring + chosung query. */
  function matches(text: string): boolean {
    if (!needle) return true
    if (text.toLowerCase().includes(needle)) return true
    return chosungQuery && extractChosung(text).includes(needle)
  }

  function issueItem(issue: IssueLite, section: Section = 'issue'): Item {
    return {
      id: `i:${issue.issue_key}`,
      section,
      label: issue.issue_key,
      sub: issue.summary,
      subSegs: needle ? highlightSegments(issue.summary, raw) : undefined,
      mono: true,
      run: () => selection.select(issue.issue_key),
    }
  }

  /** A mirrored page reads as its title; the badge carries the kind, since a
   *  page key is an opaque id nobody recognizes. */
  function docItem(page: PageLite, section: Section = 'doc'): Item {
    return {
      id: `d:${page.key}`,
      section,
      label: page.title,
      segs: needle ? highlightSegments(page.title, raw) : undefined,
      badge: t('doc.badge'),
      sub: pages.spaceLabel(page.space_key),
      testid: 'palette-doc-row',
      run: () => pages.select(page.key),
    }
  }

  /**
   * Documents matched by the query — title first, then the space it lives in,
   * with chosung covering the Korean titles a Latin keyboard cannot type in
   * full. Four rows: the section sits above the issues and is a way in, not the
   * place to read a result set (that is the list's search section, which Enter
   * in the document filter goes to).
   */
  const docMatches = $derived.by<PageLite[]>(() =>
    needle ? rankPages(pages.index, needle, chosungQuery, (key) => pages.spaceLabel(key)) : [],
  )

  const DOC_LIMIT = 4
  const docItems = $derived.by<Item[]>(() =>
    docMatches.slice(0, DOC_LIMIT).map((page) => docItem(page)),
  )

  /** What the header says next to "Documents": how many are shown out of how
   *  many matched, so four rows never read as the whole answer. Spelled out
   *  ("4 of 7"), never "4 / 7" — the slash form is the document screens'
   *  filter fraction (shown / total in the mirror), and one glyph carrying two
   *  meanings across surfaces is how it stops carrying either. */
  const docCount = $derived(
    docMatches.length > DOC_LIMIT
      ? t('palette.docCount', {
          shown: formatNumber(docItems.length),
          total: formatNumber(docMatches.length),
        })
      : formatNumber(docMatches.length),
  )

  const issueItems = $derived.by<Item[]>(() => {
    // Empty input = recently opened issues and documents, in visit order.
    if (!needle) {
      const out: Item[] = []
      for (const visit of me.recent) {
        if (visit.kind === 'doc') {
          const page = pages.byKey.get(visit.key)
          if (page) out.push(docItem(page, 'recent'))
        } else {
          const issue = issues.pool.get(visit.key)
          if (issue) out.push(issueItem(issue, 'recent'))
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
      myAccountId: me.accountId,
      recentKeys: new Set(me.recentIssues.map((v) => v.key)),
    }
    return sortIssues(filterIssues(issues.allIssues, f), 'relevance', 'desc', ctx)
      .slice(0, 8)
      .map((issue) => issueItem(issue))
  })

  /**
   * People come from the member directory bootstrap already loaded, so this is
   * a pool read like every other section — no request on a keystroke.
   *
   * A name is matched by the same forgiving rule as views and actions, plus the
   * email, because half the time the thing you remember about a colleague is
   * their handle. Exact and prefix hits sort first: typing "sam" should reach
   * Sam before it reaches Samantha, and the list is capped at five so a large
   * directory cannot push the issues section off the screen.
   */
  const peopleItems = $derived.by<Item[]>(() => {
    if (!needle) return []
    const scored: { member: Member; label: string; rank: number }[] = []
    for (const member of issues.members.values()) {
      const label = member.display_name || member.name || member.email
      if (!matches(label) && !matches(member.email)) continue
      const lower = label.toLowerCase()
      const local = member.email.toLowerCase().split('@')[0]
      const rank = lower === needle || local === needle ? 0 : lower.startsWith(needle) || local.startsWith(needle) ? 1 : 2
      scored.push({ member, label, rank })
    }
    return scored
      .sort((a, b) => a.rank - b.rank || a.label.localeCompare(b.label))
      .slice(0, 5)
      .map(({ member, label }) => {
        const identity = member.jira_account_id || member.email
        return {
          id: `p:${identity}`,
          section: 'person' as const,
          label,
          icon: 'user' as const,
          sub: member.email || member.jira_account_id || '',
          testid: 'palette-person-row',
          run: () => person.select(identity),
        }
      })
  })

  function applyView(config: ViewConfig) {
    showIssueList(config)
  }

  function searchResultKeys(): string[] {
    if (serverView.status !== 'ready') return []
    const res = serverView.response
    return [...(res.keys ?? []), ...(res.pages ?? []).map((p) => p.key)]
  }

  /** Record the committed unified query once. Local typing is not a search. */
  function flushPaletteSearch(opened?: { kind: 'issue' | 'page'; key: string }): void {
    if (serverView.status !== 'ready') return
    const q = serverView.query
    if (!q) return
    const res = serverView.response
    me.recordSearch(q, res.total ?? 0, searchResultKeys(), opened)
  }

  function closePalette(): void {
    flushPaletteSearch()
    onclose()
  }

  const viewItems = $derived.by<Item[]>(() => {
    const out: Item[] = []
    const push = (id: string, name: string, sub: string, config: ViewConfig, icon?: IconName) => {
      if (matches(name)) out.push({ id, section: 'view', label: name, sub, icon, run: () => applyView(config) })
    }
    for (const v of builtinViews()) push(`vb:${v.id}`, v.name, t('palette.viewBuiltin'), v.config, v.icon)
    for (const v of views.personal) push(`vp:${v.id}`, v.name, t('palette.viewPersonal'), v.config)
    for (const v of views.team) push(`vt:${v.id}`, v.name, t('palette.viewTeam'), v.config)
    for (const v of views.source) {
      if (!matches(v.name)) continue
      out.push({
        id: `vs:${v.id}`,
        section: 'view',
        label: v.name,
        sub: t('palette.viewSource'),
        run: () => {
          applyView(v.config)
          if (v.unsupported?.length) {
            write.toast(t('filter.jqlPartial', { clauses: v.unsupported.join('; ') }), 'info')
          }
        },
      })
    }
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
      {
        id: 'a:triage-labels',
        label: t('palette.actionTriageLabels', { target }),
        kbd: 'l',
        run: () => triage.requestMenu('labels'),
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
      {
        id: 'a:history',
        label: t('palette.actionHistory'),
        run: () => {
          me.closeFeed()
          pages.openHistory()
        },
      },
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
      ...THEME_MODES.map((mode) => ({
        id: `a:theme-${mode.name}`,
        label: t('palette.actionTheme', { mode: t(mode.labelKey) }),
        run: () => setThemePreference(mode.name),
      })),
      { id: 'a:sync', label: t('palette.actionSyncStatus'), run: syncStatusToast },
      {
        id: 'a:sync-now',
        label: t('palette.actionSyncNow'),
        run: () => void runSyncNow('incremental'),
      },
    ]
    return defs.filter((d) => matches(d.label)).map((d) => ({ ...d, section: 'action' as const }))
  })

  const localIssueKeys = $derived.by(() => {
    const keys = new Set<string>()
    for (const item of issueItems) {
      if (item.id.startsWith('i:')) keys.add(item.id.slice(2))
    }
    return keys
  })

  const localPageKeys = $derived.by(() => {
    const keys = new Set<string>()
    for (const item of docItems) {
      if (item.id.startsWith('d:')) keys.add(item.id.slice(2))
    }
    for (const item of issueItems) {
      if (item.id.startsWith('d:')) keys.add(item.id.slice(2))
    }
    return keys
  })

  function matchFieldLabel(field: SearchMatch['field']): string {
    if (field === 'comment') return t('palette.matchComment')
    if (field === 'body') return t('palette.matchBody')
    return t('palette.matchTitle')
  }

  function attachMatch(item: Item, match: SearchMatch | undefined, title: string): Item {
    const evidence = matchEvidence(match, title, raw)
    if (evidence && evidence !== 'title') item.match = evidence
    return item
  }

  const unifiedItems = $derived.by<Item[]>(() => {
    if (!needle || serverView.status !== 'ready') return []
    const proj = projectUnifiedHits(serverView.response, localIssueKeys, localPageKeys)
    const out: Item[] = []
    for (const hit of proj.pages) {
      if (!hit.page) continue
      const item = docItem(hit.page, 'unified')
      item.id = `u:d:${hit.page.key}`
      item.testid = 'palette-unified-doc'
      const open = item.run
      item.run = () => {
        flushPaletteSearch({ kind: 'page', key: hit.page!.key })
        open()
      }
      out.push(attachMatch(item, hit.match, hit.page.title))
    }
    for (const hit of proj.issues) {
      const issue = issues.pool.get(hit.key)
      const item = issue
        ? issueItem(issue, 'unified')
        : {
            id: `u:i:${hit.key}`,
            section: 'unified' as const,
            label: hit.key,
            mono: true,
            run: () => selection.select(hit.key),
          }
      item.id = `u:i:${hit.key}`
      item.testid = 'palette-unified-issue'
      const open = item.run
      item.run = () => {
        flushPaletteSearch({ kind: 'issue', key: hit.key })
        open()
      }
      out.push(attachMatch(item, hit.match, issue?.summary ?? ''))
    }
    if (proj.truncated) {
      out.push({
        id: 'u:more',
        section: 'unified',
        label: t('palette.seeMore'),
        icon: 'arrow-up-right',
        testid: 'palette-unified-more',
        run: () => {
          flushPaletteSearch()
          const c = filters.currentConfig()
          c.filters.q = raw
          showIssueList(c)
          void filters.runServerSearch().then(applyServerSearchOutcome)
        },
      })
    }
    return out
  })

  $effect(() => {
    unifiedSession.request(raw)
  })

  const unifiedBusy = $derived(
    serverView.status === 'pending' ||
      serverView.status === 'loading' ||
      (Boolean(needle) && serverView.status === 'idle'),
  )
  const showUnifiedStatus = $derived(
    Boolean(needle) &&
      unifiedItems.length === 0 &&
      (unifiedBusy || serverView.status === 'error' || serverView.status === 'ready'),
  )

  // People lead when they match at all. The group only appears for a query that
  // names someone, which is a strong statement of intent — and putting it under
  // eight issue rows would leave the axis undiscoverable, which is the whole
  // reason it exists. Key and title queries never match a member, so the hot
  // path (jump to an issue) keeps its top slot.
  //
  // Documents sit above the issues for the same reason they do in the list's
  // search section: when someone types words rather than a key, the page is
  // often the thing they came for, and eight issue rows would bury it.
  const items = $derived([
    ...peopleItems,
    ...docItems,
    ...issueItems,
    ...viewItems,
    ...actionItems,
    ...unifiedItems,
  ])

  const SECTION_LABEL: Record<Section, string> = {
    person: t('palette.sectionPeople'),
    recent: t('palette.recent'),
    doc: t('palette.sectionDocs'),
    issue: t('palette.sectionIssues'),
    view: t('palette.sectionViews'),
    action: t('palette.sectionActions'),
    unified: t('palette.sectionUnified'),
  }

  // Keep the highlight inside the viewport.
  $effect(() => {
    if (idx >= items.length) idx = Math.max(0, items.length - 1)
    listEl?.querySelector(`[data-idx="${idx}"]`)?.scrollIntoView({ block: 'nearest' })
  })

  function run(item: Item) {
    item.run()
    closePalette()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      closePalette()
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
  class="fixed inset-0 z-50 flex items-start justify-center bg-[#1c1812]/28 p-4 pt-[12vh] backdrop-blur-[2px]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) closePalette()
  }}
>
  <div
    use:trapFocus
    class="anim-pop flex max-h-[70vh] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-border-strong bg-bg-panel shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('palette.title')}
  >
    <div class="flex h-11 flex-none items-center gap-2 border-b border-border-subtle px-3">
      <Icon name="search" size={15} class="text-text-muted" />
      <input
        bind:this={inputEl}
        bind:value={query}
        oninput={() => (idx = 0)}
        onkeydown={onKeydown}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls="palette-list"
        aria-autocomplete="list"
        aria-activedescendant={items.length ? `palette-opt-${idx}` : undefined}
        placeholder={t('palette.placeholder')}
        class="min-w-0 flex-1 bg-transparent text-body text-text-primary placeholder:text-text-muted focus:outline-none"
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
      {#if !needle}
        <p class="px-2 pb-1 pt-2 text-micro text-text-muted" data-testid="palette-empty-hint">
          {t('palette.emptyHint')}
        </p>
      {/if}
      {#if items.length === 0 && !showUnifiedStatus}
        <p class="px-2 py-6 text-center text-[12px] text-text-muted">{t('palette.empty')}</p>
      {/if}
      {#each items as item, i (item.id)}
        {#if i === 0 || items[i - 1].section !== item.section}
          <div
            role="presentation"
            class="flex items-center gap-1.5 px-2 pb-1 pt-2 text-micro font-medium uppercase tracking-wide text-text-muted"
            data-testid="palette-section"
            data-section={item.section}
          >
            <span>{SECTION_LABEL[item.section]}</span>
            {#if item.section === 'doc'}
              <!-- Four rows out of however many matched. Without the total, a
                   capped section reads as the whole answer. -->
              <span class="tabular-nums" data-testid="palette-doc-count">{docCount}</span>
            {/if}
          </div>
        {/if}
        <button
          type="button"
          role="option"
          id="palette-opt-{i}"
          data-idx={i}
          data-testid={item.testid}
          aria-selected={i === idx}
          class="flex w-full flex-col gap-0.5 rounded px-2 py-1.5 text-left text-[12px] {i === idx
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover'}"
          onmousemove={() => (idx = i)}
          onmousedown={(e) => {
            e.preventDefault()
            run(item)
          }}
        >
          <span class="flex w-full min-w-0 items-center gap-2">
            {#if item.icon}
              <Icon name={item.icon} size={14} class={i === idx ? 'text-text-secondary' : 'text-text-muted'} />
            {/if}
            {#if item.badge}
              <span
                class="flex-none rounded bg-bg-active px-1.5 py-0.5 text-micro font-medium uppercase tracking-wide text-text-muted"
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
              <!-- Same mark as the list's search hits: the row shows the part of
                   the title the query found. A chosung query marks nothing, and
                   falls back to the plain title. -->
              {#if item.segs}<Marks segs={item.segs} />{:else}{item.label}{/if}
            </span>
            {#if item.sub}
              <span
                class="truncate text-text-muted {item.badge
                  ? 'max-w-[35%] flex-none'
                  : 'min-w-0 flex-1'}"
                >{#if item.subSegs}<Marks segs={item.subSegs} />{:else}{item.sub}{/if}</span
              >
            {/if}
            {#if item.kbd}
              <span class="ml-auto flex-none">
                <kbd class="rounded border border-border-subtle px-1 text-micro text-text-muted">
                  {item.kbd}
                </kbd>
              </span>
            {/if}
          </span>
          {#if item.match?.snippet}
            <span
              class="flex w-full min-w-0 items-center gap-1 text-micro text-text-muted"
              data-testid="palette-unified-snippet"
              data-match-field={item.match.field}
            >
              <span class="flex-none">{matchFieldLabel(item.match.field)}</span>
              <span class="min-w-0 flex-1 truncate"
                ><Marks segs={highlightSegments(item.match.snippet, raw)} /></span
              >
            </span>
          {/if}
        </button>
      {/each}
      {#if showUnifiedStatus}
        <div
          role="presentation"
          class="flex items-center gap-1.5 px-2 pb-1 pt-2 text-micro font-medium uppercase tracking-wide text-text-muted"
          data-testid="palette-section"
          data-section="unified"
        >
          <span>{t('palette.sectionUnified')}</span>
        </div>
        {#if unifiedBusy}
          <p class="px-2 py-2 text-[12px] text-text-muted" data-testid="palette-unified-loading">
            {t('list.searching')}
          </p>
        {:else if serverView.status === 'error'}
          <p class="px-2 py-2 text-[12px] text-text-muted" data-testid="palette-unified-error">
            {t('list.searchFailed')}
          </p>
        {:else}
          <p class="px-2 py-2 text-[12px] text-text-muted" data-testid="palette-unified-empty">
            {t('palette.empty')}
          </p>
        {/if}
      {/if}
    </div>

    <div
      class="flex-none border-t border-border-subtle px-3 py-1.5 text-micro text-text-muted"
    >
      {t('palette.hintNav')} · <kbd class="font-mono">?</kbd> {t('palette.hintHelp')}
    </div>
  </div>
</div>
