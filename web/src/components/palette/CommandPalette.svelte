<script lang="ts">
  /*
   * Command palette (⌘K/Ctrl+K) — issue jump / apply view / run action / all-search.
   *  - Local sections stay on the memory pool (zero network). Issues reuse list
   *    filterIssues + relevance sort, so key short-forms work.
   *  - After a debounce, GET search/?q= fills an All-search section under them
   *    (titles, bodies, comments; list filter chips are not sent).
   *  - Items are a flat array in section order → ↑↓ moves a single index.
   *  - Open/close and key bindings live in App.svelte (must open even while focused).
   */
  import { onMount } from 'svelte'
  import { createCompositionCommit } from '../../lib/composition-commit'
  import { trapFocus } from '../../lib/focus-trap'
  import {
    formatNumber,
    formatTimeOfDay,
    t,
  } from '../../lib/i18n'
  import { rankPages } from '../../lib/doc-search'
  import { highlightSegments } from '../../lib/format'
  import { search, workspaceHost, workspaceHref } from '../../lib/api'
  import { workspaces } from '../../stores/workspaces.svelte'
  import { matchEvidence } from '../../lib/search-match'
  import {
    UNIFIED_FETCH_LIMIT,
    createUnifiedSearch,
    emptyUnifiedView,
    projectUnifiedHits,
    requestOpenShortcuts,
    type UnifiedView,
  } from '../../lib/unified-search'
  import { builtinViews } from '../../lib/builtin-views'
  import { showIssueList } from '../../lib/show-issue-list'
  import { applyServerSearchOutcome } from '../../lib/server-search'
  import { MULTI_FIELDS, emptyFilters, type ViewConfig, type ViewFilters } from '../../lib/view-config'
  import {
    filterIssues,
    filters,
    sortIssues,
    type RelevanceContext,
  } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { column } from '../../stores/column.svelte'
  import { me } from '../../stores/me.svelte'
  import { person } from '../../stores/person.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import { triage } from '../../stores/triage.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { terminalChrome } from '../../lib/terminal/pane.svelte'
  import { enterShell } from '../../lib/terminal/sessions.svelte'
  import { shellForIssue } from '../../lib/issue-shells'
  import { shells } from '../../lib/issue-shells.svelte'
  import { views } from '../../stores/views.svelte'
  import { write } from '../../stores/write.svelte'
  import { favorites } from '../../stores/favorites.svelte'
  import { watches } from '../../stores/watches.svelte'
  import { feature, hasServerVerb, isHostedDemo } from '../../lib/config'
  import { runSyncNow } from '../../lib/sync-now'
  import { openIssueOrigin, openOriginUrl } from '../../lib/desktop-links'
  import { paletteActionItems } from '../../lib/command-palette'
  import { copyViewLink } from '../list/copy-view-link'
  import type { IssueLite, Member, PageLite, SearchMatch } from '../../lib/types'
import type { SettingsTab } from '../../lib/settings-tabs'
  import Icon, { type IconName } from '../ui/Icon.svelte'
  import Marks from '../ui/Marks.svelte'

  let { onclose, onOpenSettings }: { onclose: () => void; onOpenSettings: (tab?: SettingsTab) => void } =
    $props()

  // Empty-query home: 'recent' is visits (issues + documents, visit order —
  // one group so neither kind claims the other's section); 'updated' is the
  // in-memory recently-updated issue slice. A query replaces both with the
  // usual match sections.
  type Section = 'person' | 'recent' | 'updated' | 'doc' | 'issue' | 'view' | 'action' | 'unified'

  const HOME_LIMIT = 5

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
    /** Instant-create stays open until the request settles (failure must keep the query). */
    stayOpen?: boolean
    run: () => void
  }

  // `draft` is the live input (browser-owned during IME). `query` is the
  // committed needle — GDK-169: mid-composition states are never committed
  // as queries.
  let draft = $state('')
  let query = $state('')
  let idxRaw = $state(0)
  // GDK-461: a write must never be the default Enter target. Arrow/hover
  // can still land on create-now; query changes reset this so a shrinking
  // match set cannot leave create selected by accident.
  let idxUserMoved = $state(false)

  const ime = createCompositionCommit((q) => {
    query = q
    idxUserMoved = false
  })
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
    // The session table is polled only while a surface that draws it is
    // mounted (issue-shells.svelte); the shell rows below are one.
    const untrack = shells.track()
    return () => {
      untrack()
      unifiedSession.cancel()
    }
  })

  const raw = $derived(query.trim())
  const needle = $derived(raw.toLowerCase())

  /** Match view/action names — substring. */
  function matches(text: string): boolean {
    if (!needle) return true
    return text.toLowerCase().includes(needle)
  }

  /**
   * GDK-300: the user named this action. Full label equality, or a
   * distinctive token of the label ("settings" in "Open settings", "설정"
   * in "설정 열기"). Short Latin tokens ("issue", "new", "sync") stay in
   * section order so they do not steal Enter from a key/title query.
   * People already rank exact/prefix inside their section; actions had no
   * equivalent, so a doc/issue title that merely contains the word won.
   */
  function isDistinctiveActionToken(token: string): boolean {
    if (/[^\u0000-\u007f]/.test(token)) return true
    return token.length >= 6
  }

  function isExactActionMatch(label: string): boolean {
    if (!needle) return false
    const lower = label.toLowerCase()
    if (lower === needle) return true
    return lower.split(/[\s·]+/).some((tok) => tok === needle && isDistinctiveActionToken(tok))
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
   * Documents matched by the query — title first, then the space it lives in.
   * Four rows: the section sits above the issues and is a way in, not the
   * place to read a result set (that is the list's search section, which Enter
   * in the document filter goes to).
   */
  const docMatches = $derived.by<PageLite[]>(() =>
    needle ? rankPages(pages.index, needle, (key) => pages.spaceLabel(key)) : [],
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

  /**
   * GDK-1196 — the way into an issue's shell, from the palette.
   *
   * A row of its own rather than a hidden ⌘Enter on the issue row: this is
   * the product's claim (a shell belongs to an issue) and a shortcut nobody
   * can see does not teach it. It sits *under* the issue it belongs to and
   * inside the same section, so the plain jump keeps the pre-selected Enter
   * it has always had — pressing Enter on a key still opens the issue.
   *
   * Only a live binding produces one; `shellForIssue` is the same lookup the
   * body's ▶ uses, so the two affordances can never disagree.
   */
  function shellRow(key: string, section: Section): Item | null {
    const session = shellForIssue(shells.sessions, key)
    if (!session) return null
    return {
      id: `s:${key}`,
      section,
      label: key,
      mono: true,
      icon: 'terminal',
      sub: t('palette.openShell'),
      testid: 'palette-shell-row',
      run: () => enterShell(session.id),
    }
  }

  function withShellRows(list: Item[]): Item[] {
    return list.flatMap((item) => {
      const row = item.id.startsWith('i:') ? shellRow(item.id.slice(2), item.section) : null
      return row ? [item, row] : [item]
    })
  }

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
        if (out.length >= HOME_LIMIT) break
      }
      return withShellRows(out)
    }
    const f = emptyFilters()
    f.q = raw
    // Relevance context matches the list (match strength + recency + personalization).
    const ctx: RelevanceContext = {
      needle,
      now: Date.now(),
      myEmail: me.email,
      myAccountId: me.accountId,
      recentKeys: new Set(me.recentIssues.map((v) => v.key)),
    }
    return withShellRows(
      sortIssues(filterIssues(issues.allIssues, f), 'relevance', 'desc', ctx)
        .slice(0, 8)
        .map((issue) => issueItem(issue)),
    )
  })

  /**
   * Empty-query home: issues that moved recently. Reads the already-loaded
   * pool (issues.allIssues) — same zero-network rule as people. Sort on
   * IssueLite.updated_at; keys already in Recent are skipped so the two
   * sections do not repeat a row.
   */
  const updatedItems = $derived.by<Item[]>(() => {
    if (needle) return []
    const recentKeys = new Set<string>()
    for (const visit of me.recent) {
      if (visit.kind !== 'doc') recentKeys.add(visit.key)
    }
    for (const item of issueItems) {
      if (item.id.startsWith('i:')) recentKeys.add(item.id.slice(2))
    }
    const ranked = [...issues.allIssues].sort((a, b) =>
      (b.updated_at ?? '').localeCompare(a.updated_at ?? ''),
    )
    const out: Item[] = []
    for (const issue of ranked) {
      if (recentKeys.has(issue.issue_key)) continue
      const item = issueItem(issue, 'updated')
      item.testid = 'palette-updated-row'
      out.push(item)
      if (out.length >= HOME_LIMIT) break
    }
    return out
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
    showIssueList(config, true)
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

  /*
   * A view row has to answer three things at a glance: what it is called, which
   * kind of view it is, and what it opens. Built-ins came with a glyph and the
   * three saved kinds came with none, and every sub line was a single kind word
   * — so the section read as two lists, and no row answered the third question.
   * (GDK-191)
   *
   * The kind glyphs are picked for what they already mean elsewhere, and around
   * the ones this screen has already spent: `user` marks a person in the People
   * section above, and inbox/layers/clock/zap and the rest belong to the
   * built-in views in this very section.
   */
  const VIEW_KIND_ICON = {
    /**
     * Saved by me, like a starred issue is kept by me. Both stores get this
     * glyph: GDK-437 made a saved view one kind, so whether the row came from
     * the server or from this browser is not a distinction to draw here.
     */
    saved: 'star',
    /** Mirrored from the site — the same glyph the browse pane uses for "out there". */
    source: 'globe',
  } as const satisfies Record<'saved' | 'source', IconName>

  /**
   * Active filter axes, minus the two the clue names outright. Counting axes and
   * not values is what makes "2 filters" mean "two things narrowed" rather than
   * "two labels picked on one axis".
   */
  function filterAxisCount(f: Partial<ViewFilters>): number {
    let n = 0
    for (const field of MULTI_FIELDS) {
      if (field === 'jira_project' || field === 'keys') continue
      if ((f[field] ?? []).length) n++
    }
    const dyn = f.fields ?? {}
    for (const alias in dyn) if ((dyn[alias] ?? []).length) n++
    if (f.reopened) n++
    if (f.unassigned) n++
    if (f.stale) n++
    if (f.created_from || f.created_to) n++
    if (f.updated_from || f.updated_to) n++
    if (f.due_from || f.due_to) n++
    if (f.resolved_from || f.resolved_to) n++
    if ((f.q ?? '').trim()) n++
    return n
  }

  /**
   * What a saved view opens, read off the config the row is already holding —
   * never a request, because every section of this palette runs on the memory
   * pool (file header) and a sub line is not worth breaking that for.
   *
   * Projects lead: it is the axis people name a view by. Three keys stop being a
   * clue and become a list, so they collapse to a count. Configs arriving from
   * Jira are partial (only the clauses that translated), so every axis is read
   * defensively.
   */
  function viewClue(config: ViewConfig | undefined): string {
    const f = config?.filters as Partial<ViewFilters> | undefined
    if (!f) return ''
    const parts: string[] = []
    const projects = f.jira_project ?? []
    if (projects.length && projects.length <= 2) {
      parts.push(projects.join(', '))
    } else if (projects.length) {
      parts.push(t('palette.viewProjects', { n: formatNumber(projects.length) }))
    }
    const keys = (f.keys ?? []).length
    if (keys) {
      parts.push(keys === 1 ? t('palette.viewKeyOne') : t('palette.viewKeys', { n: formatNumber(keys) }))
    }
    const axes = filterAxisCount(f)
    if (axes) {
      parts.push(axes === 1 ? t('palette.viewFilterOne') : t('palette.viewFilters', { n: formatNumber(axes) }))
    }
    return parts.join(' · ')
  }

  /** Kind first, then the clue — one shape for all four kinds of view row. */
  function viewSub(kind: string, clue: string): string {
    return clue ? `${kind} · ${clue}` : kind
  }

  const viewItems = $derived.by<Item[]>(() => {
    const out: Item[] = []
    const push = (id: string, name: string, sub: string, config: ViewConfig, icon: IconName) => {
      if (!matches(name)) return
      out.push({
        id,
        section: 'view',
        label: name,
        sub,
        icon,
        testid: 'palette-view-row',
        run: () => applyView(config),
      })
    }
    // A built-in carries a written hint, which says what it opens better than a
    // count of its filters ever could. Identity views are absent for an
    // anonymous reader — same rule as the sidebar (my-work pack).
    for (const v of builtinViews()) {
      if (v.needsIdentity && !me.identified) continue
      push(`vb:${v.id}`, v.name, viewSub(t('palette.viewBuiltin'), v.hint ?? ''), v.config, v.icon)
    }
    for (const v of views.personal) {
      push(
        `vp:${v.id}`,
        v.name,
        viewSub(t('palette.viewSaved'), viewClue(v.config)),
        v.config,
        VIEW_KIND_ICON.saved,
      )
    }
    for (const v of views.team) {
      push(
        `vt:${v.id}`,
        v.name,
        viewSub(t('palette.viewSaved'), viewClue(v.config)),
        v.config,
        VIEW_KIND_ICON.saved,
      )
    }
    for (const v of views.source) {
      if (!matches(v.name)) continue
      out.push({
        id: `vs:${v.id}`,
        section: 'view',
        label: v.name,
        sub: viewSub(t('palette.viewSource'), viewClue(v.config)),
        icon: VIEW_KIND_ICON.source,
        testid: 'palette-view-row',
        run: () => {
          applyView(v.config)
          if (v.unsupported?.length) {
            write.toast(t('filter.jqlPartial', { clauses: v.unsupported.join('; ') }), 'info')
          }
        },
      })
    }
    if (!needle) {
      const saved = out.filter((item) => !item.id.startsWith('vb:'))
      return (saved.length > 0 ? saved : out).slice(0, HOME_LIMIT)
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

  let createBusy = $state(false)

  async function createFromPalette(summary: string) {
    if (createBusy) return
    createBusy = true
    try {
      const res = await write.createFromSummary(summary)
      if (res.ok) {
        closePalette()
        return
      }
      if (!res.error) {
        // Write gate opened settings — drop the palette so that dialog is visible.
        closePalette()
        return
      }
      write.toast(res.error, 'error')
    } finally {
      createBusy = false
    }
  }

  /**
   * GDK-1340: Switch to <workspace> for every mirror but the one on screen,
   * then the door into Settings → Workspaces — the rows the sidebar
   * switcher's menu holds (GDK-1335), reachable without the mouse. Read from
   * the store the sidebar filled at boot: the palette makes no request of
   * its own. A full navigation, not a store flip — a workspace is a
   * different mount with its own mirror.
   */
  const workspaceItems = $derived.by<Item[]>(() => {
    const out: Item[] = []
    for (const w of workspaces.list) {
      if (w.active || w.error) continue
      const label = t('palette.actionSwitchWorkspace', { name: w.name })
      if (!matches(label)) continue
      out.push({
        id: `w:${w.name}`,
        section: 'action',
        label,
        icon: 'layers',
        sub: workspaceHost(w),
        testid: 'palette-workspace',
        run: () => window.location.assign(workspaceHref(w)),
      })
    }
    const newLabel = t('sidebar.workspaceNew')
    if (hasServerVerb('settings') && matches(newLabel)) {
      out.push({
        id: 'a:workspace-new',
        section: 'action',
        label: newLabel,
        icon: 'plus',
        testid: 'palette-workspace-new',
        run: () => onOpenSettings('workspaces'),
      })
    }
    return out
  })

  const actionItems = $derived.by<Item[]>(() => {
    const cursor = triage.listActive ? triage.cursorKey : null
    const pageKey = pages.selectedKey
    const defs = paletteActionItems({
      bulkCount: bulk.count,
      bulkHasCursor: Boolean(cursor && bulk.has(cursor)),
      cursor,
      pageKey,
      issueKey: cursor ?? selection.selectedKey,
      identified: me.identified,
      hostedDemo: isHostedDemo(),
      feedEnabled: feature('feed'),
      query: raw,
      favoriteHas: (key) => favorites.has(key),
      watchHas: (key) => watches.has(key),
      host: {
        requestMenu: (menu) => triage.requestMenu(menu),
        openComment: (key) => triage.openComment(key),
        toggleBulk: (key) => bulk.toggle(key),
        clearBulk: () => bulk.clear(),
        toggleFavorite: (key) => void favorites.toggle(key),
        toggleWatch: (key) => void watches.toggle(key),
        openPageOrigin: () => {
          if (!pageKey) return
          const row = pages.lite(pageKey) ?? pages.searchHits.find((p) => p.key === pageKey)
          openOriginUrl(row?.url)
        },
        openIssueOrigin: (key) => openIssueOrigin(key),
        openNewIssue: () => void write.openNewIssue(),
        openSettings: onOpenSettings,
        copyViewLink: () => void copyViewLink(),
        openHistory: () => {
          // One show onto the column union (GDK-821) — whatever held the
          // column, feed included, is released by the same move.
          pages.openHistory()
        },
        openDocs: () => {
          if (pages.docsView && pages.spaceView === null) return
          pages.openDocs()
        },
        openFeed: () => {
          me.openFeed()
        },
        // Not a column view: the terminal sits beside whatever is up, so it
        // toggles rather than taking the column the way Docs or History do.
        toggleTerminal: () => terminalChrome.toggle(),
        clearUserFilters: () => filters.clearUserFilters(),
        toggleFlag: (flag) => filters.toggleFlag(flag),
        syncStatus: syncStatusToast,
        syncNow: () => void runSyncNow('incremental'),
        createNow: (summary) => void createFromPalette(summary),
      },
    })
    const createNow = defs.find((d) => d.id === 'a:create-now')
    // Copy link builds the Jira line from the list's filters; off the list
    // (Documents, History, feed, a dashboard) the hash names another screen.
    const rest = defs.filter(
      (d) => d.id !== 'a:create-now' && matches(d.label) && (d.id !== 'a:copy-view-link' || column.is('list')),
    )
    const out: Item[] = rest.map((d) => ({ ...d, section: 'action' as const }))
    out.push(...workspaceItems)
    if (!raw) return out
    // Instant-create is the typed text — it is not filtered by the action name.
    // It goes AFTER every matched action: create writes to Jira, so it must
    // never be the pre-selected Enter target when the user typed an action's
    // name ("Sync now" + Enter must sync, not file an issue titled "Sync now").
    // GDK-461: a destructive action is never the default. With no other match
    // the row still appears, unselected, and Enter does nothing until the
    // user arrows or points at it. New issue is not force-appended beside it
    // — that was a second write entry on a zero-match list.
    if (createNow) out.push({ ...createNow, section: 'action' })
    return out
  })

  const localIssueKeys = $derived.by(() => {
    const keys = new Set<string>()
    for (const item of issueItems) {
      if (item.id.startsWith('i:')) keys.add(item.id.slice(2))
    }
    for (const item of updatedItems) {
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
          // Back to the list for the widened search: one show releases
          // whatever held the column (GDK-821), closeDocs drops the docs
          // screens' narrowing on the way out.
          column.show({ view: 'list' })
          pages.closeDocs()
          filters.applyConfig(c)
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
  //
  // GDK-300: naming an action (exact label or a distinctive token of it) is
  // the same kind of statement — hoist the whole action block so Enter is
  // locale-stable. Instant-create is never an exact match (its label wraps
  // the query) and stays last inside that block.
  const hoistActions = $derived(
    Boolean(needle) &&
      actionItems.some((item) => item.id !== 'a:create-now' && isExactActionMatch(item.label)),
  )

  const items = $derived(
    hoistActions
      ? [
          ...actionItems,
          ...peopleItems,
          ...docItems,
          ...issueItems,
          ...updatedItems,
          ...viewItems,
          ...unifiedItems,
        ]
      : [
          ...peopleItems,
          ...docItems,
          ...issueItems,
          ...updatedItems,
          ...viewItems,
          ...actionItems,
          ...unifiedItems,
        ],
  )

  const SECTION_LABEL: Record<Section, string> = {
    person: t('palette.sectionPeople'),
    recent: t('palette.recent'),
    updated: t('palette.updated'),
    doc: t('palette.sectionDocs'),
    issue: t('palette.sectionIssues'),
    view: t('palette.sectionViews'),
    action: t('palette.sectionActions'),
    unified: t('palette.sectionUnified'),
  }

  /** First row that is safe to pre-select. create-now writes; it is never idx 0 by default. */
  function firstSafeIndex(list: Item[]): number {
    return list.findIndex((item) => item.id !== 'a:create-now')
  }

  // SearchBox pattern: store the raw highlight, read it clamped. Unmoved
  // follows the first non-destructive row, or −1 when the list is only
  // create-now / empty. Overflow after a shrinking match set lands on the
  // last row. The scroll is the only remaining effect — it is a DOM write.
  const idx = $derived.by(() => {
    const list = items
    if (!idxUserMoved) return firstSafeIndex(list)
    if (idxRaw >= list.length) return list.length ? list.length - 1 : -1
    return idxRaw
  })

  $effect(() => {
    const i = idx
    void items
    if (i >= 0) {
      listEl?.querySelector(`[data-idx="${i}"]`)?.scrollIntoView({ block: 'nearest' })
    }
  })

  function run(item: Item) {
    item.run()
    if (!item.stayOpen) closePalette()
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.isComposing || ime.composing) && e.key === 'Enter') {
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      closePalette()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (!items.length) return
      idxUserMoved = true
      idxRaw = idx < 0 ? 0 : (idx + 1) % items.length
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (!items.length) return
      idxUserMoved = true
      idxRaw = idx < 0 ? items.length - 1 : (idx - 1 + items.length) % items.length
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const item = idx >= 0 ? items[idx] : undefined
      if (item) run(item)
    } else if (
      e.key === '?' &&
      !draft &&
      !e.isComposing &&
      !ime.composing &&
      !e.metaKey &&
      !e.ctrlKey &&
      !e.altKey
    ) {
      // GDK-618: the footer advertises `?` as the way to the shortcuts sheet,
      // but the global keymap ignores every key while a modal owns the screen
      // (keymap.svelte.ts) — so this input is the only place the advertised
      // key can live. It owns `?` only while the query is empty: mid-query
      // `?` is a search character, and a composing session keeps the Enter
      // guard's rule (never touched mid-IME, GDK-169).
      e.preventDefault()
      closePalette()
      requestOpenShortcuts()
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
        bind:value={draft}
        oninput={(e) => ime.oninput(e, draft)}
        oncompositionstart={ime.oncompositionstart}
        oncompositionend={(e) => ime.oncompositionend(e, draft)}
        onkeydown={onKeydown}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls="palette-list"
        aria-autocomplete="list"
        aria-activedescendant={idx >= 0 && items.length ? `palette-opt-${idx}` : undefined}
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
      class="scroll-region min-h-0 flex-1 px-1 pt-1"
    >
      {#if items.length === 0 && !showUnifiedStatus}
        <p class="px-2 py-6 text-center text-micro text-text-muted">{t('palette.empty')}</p>
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
          class="flex w-full flex-col gap-0.5 rounded px-2 py-1.5 text-left text-body {i === idx
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover'}"
          onmousemove={() => {
            idxUserMoved = true
            idxRaw = i
          }}
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
                class="flex-none rounded {i === idx
                  ? 'bg-bg-base'
                  : 'bg-bg-elevated'} px-1.5 py-0.5 text-micro font-medium uppercase tracking-wide text-text-muted"
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
                   the title the query found. A caller with nothing to mark falls
                   back to the plain title. -->
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
          <p class="px-2 py-2 text-micro text-text-muted" data-testid="palette-unified-loading">
            {t('common.searching')}
          </p>
        {:else if serverView.status === 'error'}
          <div class="flex flex-col gap-1 px-2 py-2" data-testid="palette-unified-error">
            <p class="text-micro text-text-muted">{t('list.searchFailed')}</p>
            <button
              type="button"
              class="self-start rounded-md border border-border-strong px-2 py-0.5 text-body text-text-secondary transition-colors hover:bg-bg-hover"
              data-testid="palette-unified-retry"
              onclick={() => unifiedSession.request(raw)}
            >
              {t('list.searchRetry')}
            </button>
          </div>
        {:else}
          <p class="px-2 py-2 text-micro text-text-muted" data-testid="palette-unified-empty">
            {t('palette.empty')}
          </p>
        {/if}
      {/if}
    </div>

    <div
      class="flex-none border-t border-border-subtle px-3 py-1.5 text-micro text-text-muted"
    >
      {t('palette.hintNav')} · <kbd class="rounded border border-border-strong bg-bg-elevated px-1.5 py-0.5 font-mono text-micro text-text-secondary">?</kbd> {t('palette.hintHelp')}
    </div>
  </div>
</div>
