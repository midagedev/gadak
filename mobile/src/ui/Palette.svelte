<script lang="ts">
  import Row from './Row.svelte'
  import DocRow from './DocRow.svelte'
  import DeskRow from './DeskRow.svelte'
  import EmptyState from './EmptyState.svelte'
  import { t } from '../lib/i18n'
  import {
    app,
    closePalette,
    recentSearches,
    rememberSearch,
    searchPaint,
    setOwner,
  } from '../lib/store.svelte'
  import { docSnippet, matchLocal, mergeSearch, type Scope, type ScopeSection } from '../lib/domain'
  import {
    matchScopes,
    offersTerminal,
    paletteMode,
    recentIssueRows,
    scopeGroups,
    STANCE,
  } from '../lib/palette'
  import { request } from '../lib/api'
  import type { IssueLite, PageLite, SearchMatch, SearchResponse } from '../lib/types'

  /*
   * The palette (GDK-902, DESIGN.md §2) — the heading's body, not a screen.
   *
   * It is what Search.svelte was and what ScopeSheet.svelte was, in one
   * body: the same field, the same debounce, the same merge, the same
   * Row/DocRow, and the owner list the sheet used to draw. Search stopped
   * being a screen because it never was one — it is this body with a query
   * in the field.
   *
   * The ordering rules live in lib/palette.ts (vitest); what is here is the
   * I/O: the debounced request, its generation counter, and the focus.
   */
  let {
    scopes,
    counts,
    current,
    onpickScope,
  }: {
    scopes: Scope[]
    /** Match count per scope id; null = the phone refuses this view. */
    counts: Map<string, number | null>
    current: string
    onpickScope: (id: string) => void
  } = $props()

  let query = $state('')
  let serverKeys = $state<string[]>([])
  let serverPages = $state<PageLite[]>([])
  let serverMatches = $state<Record<string, SearchMatch>>({})
  let recents = $state(recentSearches())
  let debounce: ReturnType<typeof setTimeout> | null = null
  let inputEl = $state<HTMLInputElement | null>(null)
  let searching = $state(false)
  let searchFailed = $state(false)
  let searchGen = 0
  let expanded = $state(new Set<ScopeSection>())

  /*
   * Focus happens *here*, on mount, and nowhere else — and only for the
   * door that means a query. The component only exists while the palette
   * is open, so "never on boot" stays true whichever door opened it
   * (DESIGN.md §2); the heading's open is the scope door, which must not
   * put a keyboard over the owner list (GDK-1974), so it is the store's
   * `paletteFocus` — set by whichever control opened the palette — that
   * decides. An $effect body runs after the DOM is attached, inside the
   * tap's own task, which is what WKWebView requires of a programmatic
   * focus.
   */
  $effect(() => {
    if (app.paletteFocus) inputEl?.focus()
  })

  const mode = $derived(paletteMode(query))

  // Local-first: the snapshot answers instantly while typing; the server
  // adds body/comment matches the lite rows cannot see (DESIGN.md §5).
  // Page hits arrive only with the server reply — not in the issue snapshot.
  const local = $derived(matchLocal(app.issues, query))
  const results = $derived<IssueLite[]>(mergeSearch(local, serverKeys, app.issues))
  const scopeHits = $derived(matchScopes(scopes, query))
  const recentIssues = $derived(recentIssueRows(app.recentVisits, app.issues))
  const groups = $derived(scopeGroups(scopes, expanded))
  const plate = $derived(
    searchPaint({
      query,
      resultCount: results.length,
      pageCount: serverPages.length,
      searching,
      failed: searchFailed,
    }),
  )

  function onInput() {
    serverKeys = []
    serverPages = []
    serverMatches = {}
    if (debounce) clearTimeout(debounce)
    const q = query.trim()
    searchGen += 1
    const gen = searchGen
    if (q.length < 2) {
      searching = false
      searchFailed = false
      return
    }
    searching = true
    searchFailed = false
    debounce = setTimeout(async () => {
      if (gen !== searchGen) return
      try {
        const res = await request<SearchResponse>(
          `issues/search/?q=${encodeURIComponent(q)}&limit=50`,
        )
        if (gen !== searchGen) return
        if (res.body) {
          serverKeys = res.body.keys
          serverPages = res.body.pages ?? []
          serverMatches = res.body.matches ?? {}
          rememberSearch(q)
          recents = recentSearches()
        }
      } catch {
        if (gen !== searchGen) return
        searchFailed = true
      } finally {
        if (gen === searchGen) searching = false
      }
    }, 250)
  }

  function runRecent(q: string) {
    query = q
    onInput()
    inputEl?.focus()
  }

  function clear() {
    query = ''
    serverKeys = []
    serverPages = []
    serverMatches = {}
    searching = false
    searchFailed = false
    searchGen += 1
    if (debounce) {
      clearTimeout(debounce)
      debounce = null
    }
    inputEl?.focus()
  }

  function expand(section: ScopeSection): void {
    const next = new Set(expanded)
    next.add(section)
    expanded = next
  }
</script>

<!-- The field is the head of the body, not a header band: the body is what
     the heading swapped, and the field rides its top so the ranking below
     can scroll under it. -->
<div class="palette-field field">
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
    <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
  </svg>
  <input
    bind:this={inputEl}
    bind:value={query}
    oninput={onInput}
    type="search"
    aria-label={t('app.searchTitle')}
    placeholder={t('app.searchPlaceholder')}
    autocapitalize="off"
    autocorrect="off"
    spellcheck="false"
    enterkeyhint="search"
  />
  {#if query}
    <button class="clear" onclick={clear} aria-label={t('app.searchClear')}>×</button>
  {/if}
  <button class="palette-cancel" onclick={closePalette}>{t('common.cancel')}</button>
</div>

{#if mode === 'empty'}
  {#if recentIssues.length > 0}
    <!-- Issues first (GDK-875): a row opens the issue, where a scope row
         only changes what the list holds. -->
    <p class="palette-section">{t('palette.recent')}</p>
    {#each recentIssues as issue (issue.issue_key)}
      <Row {issue} showAssignee={true} />
    {/each}
  {/if}

  {#each groups as group (group.section)}
    <p class="palette-section">{t(group.headingKey)}</p>
    {#each group.rows as scope, i (scope.id)}
      {@const blocked = scope.unsupported.length > 0}
      {@const n = counts.get(scope.id) ?? null}
      {#if scope.stance && scope.stance !== group.rows[i - 1]?.stance}
        <div class="stance">{t(STANCE[scope.stance])}</div>
      {/if}
      {#if blocked}
        <!-- A blocked scope can never be the current one — the row that
             would select it is disabled — so it carries no `on` branch.
             Lifted into this list, the row wears the list's inset: the
             `.desk` wrapper supplies the other 8px of the sheet's 16
             (GDK-1948; SprintLine's idiom). -->
        <div class="desk"><DeskRow label={scope.name} /></div>
      {:else}
        <button
          class="palette-row"
          class:on={scope.id === current}
          aria-current={scope.id === current ? 'true' : undefined}
          onclick={() => onpickScope(scope.id)}
        >
          <span class="name">{scope.name}</span>
          {#if n !== null}
            <span class="n">{n}</span>
          {/if}
        </button>
      {/if}
    {/each}
    {#if group.total > group.rows.length}
      <button class="more" onclick={() => expand(group.section)}>
        {t('sidebar.scopeShowAll', { n: group.total })}
      </button>
    {/if}
    {#if group.section === 'views'}
      <!-- Making and editing a view is the desk's form: filters, JQL,
           columns, sort, save. The catalog's own word for that surface, at
           the end of the views it would produce. -->
      <div class="desk"><DeskRow label={t('view.settings')} testid="desk-row-views" /></div>
    {/if}
  {/each}

  {#if app.sprints.length > 0}
    <!-- The sprint list is an owner of the column too (GDK-1827), and like
         the Terminal row it is absent — not disabled — until there is
         something to show: the row exists only while the snapshot holds
         sprint rows, so a kanban workspace never offers it. Under its own
         section label for the same reason the Terminal row wears one. -->
    <p class="palette-section">{t('sprints.title')}</p>
    <button class="palette-row" onclick={() => setOwner('sprints')}>
      <span class="name">{t('sprints.title')}</span>
    </button>
  {/if}

  {#if offersTerminal(app.terminal)}
    <!-- The shell is an owner of the column (DESIGN.md §10), so it is a row
         in the owner list — and it is absent, not disabled, until a
         terminal pairing is stored. -->
    <p class="palette-section">{t('sidebar.terminal')}</p>
    <button class="palette-row" onclick={() => setOwner('shell')}>
      <span class="name">{t('sidebar.terminal')}</span>
    </button>
  {/if}

  <!-- Dashboards, last: a layout of panels is the one scope-shaped thing
       the phone has no plate for at all. Under its own section label, as
       the Terminal row is — without one the desk row reads as a second
       item of whichever section came before it (vision verdict, GDK-902
       2026-09-15). -->
  <p class="palette-section">{t('sidebar.dashboards')}</p>
  <div class="desk"><DeskRow label={t('sidebar.dashboards')} testid="desk-row-dashboards" /></div>

  {#if recents.length > 0}
    <p class="palette-section">{t('personal.recent')}</p>
    {#each recents as r (r)}
      <button class="recent" onclick={() => runRecent(r)}>
        <span class="r-q">{r}</span>
        <span class="r-go" aria-hidden="true">↑</span>
      </button>
    {/each}
  {/if}
  <div class="foot" aria-hidden="true"></div>
{:else if mode === 'short'}
  <p class="idle-hint">{t('app.searchIdleHint', { n: app.issues.length })}</p>
{:else}
  {#if scopeHits.length > 0}
    <!-- Matching owners lead under the desk's own word for them: the
         command palette calls this set Views
         (web/src/components/palette/CommandPalette.svelte:899), so the
         phone borrows it rather than authoring a second one (§3.6). -->
    <p class="palette-section">{t('palette.sectionViews')}</p>
    {#each scopeHits as scope (scope.id)}
      {#if scope.unsupported.length > 0}
        <div class="desk"><DeskRow label={scope.name} /></div>
      {:else}
        <button
          class="palette-row"
          class:on={scope.id === current}
          onclick={() => onpickScope(scope.id)}
        >
          <span class="name">{scope.name}</span>
        </button>
      {/if}
    {/each}
  {/if}
  {#if plate === 'results'}
    {#if results.length > 0}
      <div class="section">
        <span class="label">{t('doc.issues')}</span>
        <span class="n">{results.length}</span>
      </div>
    {/if}
    {#each results as issue (issue.issue_key)}
      <Row {issue} showAssignee={true} />
    {/each}
    {#if serverPages.length > 0}
      <div class="section">
        <span class="label">{t('sidebar.docs')}</span>
        <span class="n">{serverPages.length}</span>
      </div>
      {#each serverPages as page (page.key)}
        <!-- The snippet line is earned, not repeated: a title hit's FTS
             snippet is the title itself, so the row would say it twice
             (GDK-890). -->
        <DocRow
          {page}
          showSpace={true}
          snippet={docSnippet(serverMatches[page.key], page.title, query)}
        />
      {/each}
    {/if}
    <div class="foot" aria-hidden="true"></div>
  {:else if plate === 'searching'}
    <p class="idle-hint">{t('common.searching')}</p>
  {:else if plate === 'failed'}
    <EmptyState title={t('list.searchFailed')}>
      <button class="link" onclick={onInput}>{t('list.searchRetry')}</button>
    </EmptyState>
  {:else if scopeHits.length === 0}
    <EmptyState title={t('common.noResults')} />
  {/if}
{/if}

<style>
  /* The field block, verbatim from the Search screen it replaces — same
     border, same 44pt floor, same focus ring. The only addition is the
     Cancel control beside it (DESIGN.md §2's exit for this surface). */
  .field {
    position: sticky;
    top: 0;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: var(--spacing-control);
    margin: 0 16px 10px;
    padding: 0 4px 0 12px;
    background: var(--color-bg-panel);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    color: var(--color-text-muted);
  }
  .field:focus-within {
    border-color: var(--color-border-strong);
  }
  .field svg {
    flex: none;
    width: 17px;
    height: 17px;
  }
  input {
    flex: 1 1 auto;
    min-width: 0;
    border: none;
    outline: none;
    background: none;
    color: var(--color-text-primary);
  }
  input::-webkit-search-cancel-button {
    display: none;
  }
  input::placeholder {
    color: var(--color-text-muted);
  }
  .clear {
    flex: none;
    min-width: var(--spacing-control);
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 9999px;
    color: var(--color-text-muted);
    font-size: var(--text-title);
  }
  .palette-cancel {
    flex: none;
    padding: 0 8px;
    color: var(--color-accent-text);
    font-size: var(--text-body);
  }
  .palette-section {
    margin: 0;
    padding: 8px 16px 4px;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .idle-hint {
    margin: 0;
    padding: 8px 16px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .stance {
    padding: 8px 16px 2px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .palette-row {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 10px;
    padding: 0 16px;
    text-align: left;
    min-width: 0;
  }
  .palette-row:active {
    background: var(--color-bg-hover);
  }
  /* A DeskRow lifted into this list wears the list's inset (GDK-1948): the
     sheet's rows carry 16px of side padding where the row's own sheet values
     carry 8 (DeskRow.svelte, GDK-1704), so the wrapper makes up the other
     8 — the same wrapper SprintLine and PageDetail give the row. */
  .desk {
    padding: 0 8px;
  }
  .palette-row.on .name {
    font-weight: 600;
    color: var(--color-accent-text);
  }
  .name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }
  .n {
    flex: none;
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .more {
    display: flex;
    align-items: center;
    padding: 0 16px;
    color: var(--color-accent-text);
    font-size: var(--text-micro);
  }
  .recent {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: space-between;
    min-height: var(--spacing-control);
    padding: 0 16px;
    text-align: left;
    color: var(--color-text-secondary);
  }
  .recent:active {
    background: var(--color-bg-hover);
  }
  .r-go {
    color: var(--color-text-muted);
    transform: rotate(45deg);
  }
  .section {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 10px 16px 4px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .label {
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .link {
    color: var(--color-accent-text);
    font-size: var(--text-body);
    min-height: var(--spacing-control);
    padding: 0 16px;
  }
  .foot {
    height: 24px;
  }
</style>
