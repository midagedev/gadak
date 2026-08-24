<script module lang="ts">
  /*
   * Search — screen chunk A-search (GDK-801, the Issues half). ux-report Q4
   * is the contract: the empty screen already holds answers (recent
   * searches), and input climbs the web ladder — key shape jumps, otherwise
   * local title matches appear live, and the search key runs the REST FTS.
   *
   * The pure ladder functions live in this module block (not a new lib/
   * file) so tests can import them without mounting anything. Mirrors, in
   * order of authority: web/src/lib/omnibox.ts + jql.ts (ladder, JQL
   * detection) and web/src/lib/search-match.ts matchEvidence (the snippet
   * omission rule). Keep the regexes in lockstep with those and with
   * internal/jql/extract.go.
   */
  import type { QueueRow } from '../lib/api'
  import type { QueueRowFull } from '../lib/queue-rows'

  /** Rung ① — the whole trimmed input is an issue key (`[A-Z]+-\d+`). */
  export function looksLikeKey(raw: string): string | null {
    const s = raw.trim()
    return /^[A-Z]+-\d+$/.test(s) ? s : null
  }

  /*
   * JQL paste detection, copied from web/src/lib/jql.ts (which is itself in
   * lockstep with internal/jql/extract.go). Decides FTS text vs an advanced
   * query — mobile has no parser endpoint, so the query still goes to FTS
   * search() and the results header carries the one-line summary instead.
   */
  const JQL_FIELD_OP =
    /\b(project|projectkey|status|statuscategory|statuscategoryid|assignee|reporter|labels?|priority|issuetype|type|components?|fixversions?|created|createddate|updated|updateddate|due|duedate|resolved|resolutiondate|resolveddate|resolution|text|summary|description|comment|key|issuekey|issue)\b\s*(=|!=|~|!~|>=|<=|>|<|\bin\b|\bis\b)/i
  const JQL_ORDER_BY = /\border\s+by\b/i
  const JQL_EQ_URL = /(?:\?|&|#|^)jql=/i

  export function looksLikeJql(raw: string): boolean {
    const s = raw.trim()
    if (s === '') return false
    return (
      JQL_EQ_URL.test(s) ||
      JQL_FIELD_OP.test(s) ||
      s.toLowerCase().includes('currentuser()') ||
      JQL_ORDER_BY.test(s)
    )
  }

  /*
   * The title carries the reason for a hit by itself when the query is
   * visible in it (web search-match.ts matchEvidence — the mobile mirror at
   * substring level: web highlights the query as one literal string, so
   * "contains" is the same judgment).
   */
  export function titleShowsQuery(title: string, q: string): boolean {
    const needle = q.trim()
    if (needle === '') return false
    return title.toLowerCase().includes(needle.toLowerCase())
  }

  /** Rung ② — live title substring matches over the queue pool, pool order kept. */
  export function localTitleMatches(q: string, rows: QueueRow[]): QueueRow[] {
    const needle = q.trim().toLowerCase()
    if (needle === '') return []
    return rows.filter((r) => r.summary.toLowerCase().includes(needle))
  }

  /**
   * Rung ③ join — search returns keys only (flow-report Q3 ③: it never
   * returns IssueLite), so rows come from the bootstrap pool. A key the pool
   * does not have (done issues are outside the 1차 queue, late deltas) keeps
   * `row: null` and renders as key-only.
   */
  export interface SearchHit {
    key: string
    row: QueueRow | null
  }

  export function joinSearchKeys(keys: string[], rows: QueueRow[]): SearchHit[] {
    const byKey = new Map(rows.map((r) => [r.issue_key, r]))
    return keys.map((key) => ({ key, row: byKey.get(key) ?? null }))
  }

  /* ── Saved views (A-nav): which filter axes the phone may evaluate ──
   *
   * A saved view's config is the web ViewConfig document (web/src/lib/
   * view-config.ts) — the server stores it opaque and so does the phone.
   * Chips apply a view to the LOCAL pool only when every non-empty filter
   * axis is one the pool can key on ids/categories (CLAUDE.md schema
   * rules): status_category (+_not), jira_project (the issue_key prefix,
   * +_not), assignee_email (account id / email identity, +_not — never
   * the assignee display name), keys, and the unassigned flag. Any other
   * non-empty axis — status/priority value lists, labels, dates, text,
   * custom fields — makes the chip web-only ("웹에서 보기"): a half-read
   * filter would answer a question the view never asked. Semantics are
   * copied from web/src/stores/filters.svelte.ts (matchesMulti fold,
   * effectiveCategory aliases, sameIdentity).
   */

  /** What tapping a saved-view chip does. */
  export type ViewChipPlan =
    | { kind: 'local'; rows: QueueRow[] }
    | { kind: 'web' }

  /** The interpretable axes; everything else in `filters` must be unset. */
  const INTERPRETED_AXES: ReadonlySet<string> = new Set([
    'status_category',
    'status_category_not',
    'jira_project',
    'jira_project_not',
    'assignee_email',
    'assignee_email_not',
    'keys',
    'unassigned',
  ])

  function readFilters(config: unknown): Record<string, unknown> | null {
    if (typeof config !== 'object' || config === null) return null
    const f = (config as { filters?: unknown }).filters
    if (typeof f !== 'object' || f === null) return null
    return f as Record<string, unknown>
  }

  /** "Set" per axis shape: arrays by length, flags by true, ranges/fields by content. */
  function axisIsSet(v: unknown): boolean {
    if (Array.isArray(v)) return v.length > 0
    if (typeof v === 'boolean') return v
    if (typeof v === 'string') return v !== ''
    if (typeof v === 'object' && v !== null) return Object.keys(v).length > 0
    return false
  }

  function strArr(v: unknown): string[] {
    return Array.isArray(v) ? v.filter((s): s is string => typeof s === 'string') : []
  }

  /** web effectiveCategory (string arm): alias folding, never a status name. */
  function categoryAlias(raw: string): string {
    const sc = raw.toLowerCase()
    if (sc === 'new' || sc === 'todo') return 'new'
    if (sc === 'inprogress' || sc === 'indeterminate') return 'inprogress'
    if (sc === 'done' || sc === 'complete' || sc === 'completed') return 'done'
    return 'inprogress'
  }

  function categoryOf(row: QueueRow): string {
    return categoryAlias(row.status_category ?? '')
  }

  /** web jiraProjectOf: the issue_key prefix before '-'. */
  function projectOf(row: QueueRow): string {
    const sep = row.issue_key.indexOf('-')
    return sep > 0 ? row.issue_key.slice(0, sep) : ''
  }

  /** web sameIdentity: exact, or case-insensitive when both sides are emails. */
  function sameIdentity(a: string, b: string): boolean {
    return a === b || (a.includes('@') && b.includes('@') && a.toLowerCase() === b.toLowerCase())
  }

  function assigneeIdentity(row: QueueRowFull): { id: string; email: string } {
    return {
      id: typeof row.assignee_id === 'string' ? row.assignee_id : '',
      email: typeof row.assignee_email === 'string' ? row.assignee_email : '',
    }
  }

  function personMatches(row: QueueRowFull, value: string): boolean {
    if (value === '') return false
    const { id, email } = assigneeIdentity(row)
    return value === id || sameIdentity(value, email)
  }

  /**
   * The chip plan: interpret the view against the pool, or declare it
   * web-only. `pool` is the queue's open-issue pool — a view that keeps
   * done rows simply matches fewer of them, the honest local answer.
   */
  export function viewChipPlan(config: unknown, pool: QueueRowFull[]): ViewChipPlan {
    const f = readFilters(config)
    if (f === null) return { kind: 'web' }
    for (const [axis, v] of Object.entries(f)) {
      if (INTERPRETED_AXES.has(axis)) continue
      if (axisIsSet(v)) return { kind: 'web' }
    }
    const sc = strArr(f.status_category).map(categoryAlias)
    const scn = strArr(f.status_category_not).map(categoryAlias)
    const pj = strArr(f.jira_project)
    const pjn = strArr(f.jira_project_not)
    const as = strArr(f.assignee_email)
    const asn = strArr(f.assignee_email_not)
    const keys = strArr(f.keys).map((k) => k.trim().toUpperCase())
    const unassigned = f.unassigned === true
    const rows = pool.filter((r) => {
      const cat = categoryOf(r)
      if (sc.length && !sc.includes(cat)) return false
      if (scn.length && scn.includes(cat)) return false
      const proj = projectOf(r)
      if (pj.length && !pj.includes(proj)) return false
      if (pjn.length && pjn.includes(proj)) return false
      if (unassigned) {
        const { id, email } = assigneeIdentity(r)
        if (id !== '' || email !== '') return false
      }
      if (as.length && !as.some((v) => personMatches(r, v))) return false
      if (asn.length && asn.some((v) => personMatches(r, v))) return false
      if (keys.length && !keys.includes(r.issue_key.toUpperCase())) return false
      return true
    })
    return { kind: 'local', rows }
  }
</script>

<script lang="ts">
  import {
    ApiError,
    categoryInk,
    search as searchIssues,
    views as fetchViews,
    type ApiContext,
    type SavedView,
    type SearchResult,
  } from '../lib/api'
  import { readPairing, readToken } from '../lib/settings'
  import { t, type MessageKey } from '../lib/i18n'
  import { readRecentSearches, recordRecentSearch } from '../lib/recent-searches'

  // rows is the Queue's open pool (App mirrors it down); QueueRowFull keeps
  // assignee_id/email so saved-view chips can key people on identity, not name.
  let {
    rows = [],
    onopen,
  }: { rows?: QueueRowFull[]; onopen?: (issue_key: string) => void } = $props()

  let q = $state('')
  let submitted = $state<string | null>(null)
  let results = $state<SearchResult | null>(null)
  let searching = $state(false)
  let error = $state<ApiError | null>(null)
  let recent = $state<string[]>(readRecentSearches())
  let inputEl = $state<HTMLInputElement | null>(null)
  // Discards a response that a newer Enter has superseded.
  let runId = 0

  const jumpKey = $derived(looksLikeKey(q))
  const jumpPoolRow = $derived(
    jumpKey === null ? null : (rows.find((r) => r.issue_key === jumpKey) ?? null),
  )
  const locals = $derived(q.trim() !== '' ? localTitleMatches(q, rows) : [])
  const hits = $derived(results === null ? [] : joinSearchKeys(results.keys, rows))
  // Results stay on screen while the input still reads the submitted query;
  // any edit drops back to the live ladder.
  const showResults = $derived(submitted !== null && q.trim() === submitted)

  function errorMessageKey(err: ApiError): MessageKey {
    if (err.code === 'pairing_rejected') return 'search.error.pairing'
    if (err.code === 'credential_required') return 'search.error.credentials'
    if (err.code === 'forbidden_host' || err.code === 'forbidden_origin') {
      return 'search.error.host'
    }
    if (err.status === undefined) return 'search.error.network'
    return 'search.error.generic'
  }

  async function runSearch(text: string): Promise<void> {
    const query = text.trim()
    if (query === '') return
    const id = ++runId
    submitted = query
    results = null
    error = null
    searching = true
    recent = recordRecentSearch(query)
    const pairing = readPairing()
    // Dev default mirrors Queue.svelte: the vite proxy targets the dev serve.
    const endpoint = pairing?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
    if (endpoint === '') {
      searching = false
      error = new ApiError('network')
      return
    }
    const ctx: ApiContext = { endpoint, token: await readToken() }
    try {
      const res = await searchIssues(ctx, query)
      if (id !== runId) return
      results = res
    } catch (err) {
      if (id !== runId) return
      error = err instanceof ApiError ? err : new ApiError('network')
    } finally {
      if (id === runId) searching = false
    }
  }

  function onsubmit(e: SubmitEvent): void {
    e.preventDefault()
    const key = looksLikeKey(q)
    if (key !== null) {
      onopen?.(key)
      return
    }
    void runSearch(q)
  }

  function runRecent(text: string): void {
    q = text
    void runSearch(text)
  }

  /* ── Saved views (A-nav) ──
   * Fetched once per mount; null (never loaded, unpaired, or failed) keeps
   * the section hidden — views are an addition to the empty screen, never
   * a banner-worthy failure (ux-report Q4's empty screen stays quiet). */
  let savedViews = $state<SavedView[] | null>(null)
  let activeViewId = $state<string | null>(null)

  const activeView = $derived.by(() => {
    const id = activeViewId
    if (id === null || savedViews === null) return null
    const v = savedViews.find((s) => s.id === id) ?? null
    if (v === null) return null
    return { name: v.name, plan: viewChipPlan(v.config, rows) }
  })

  $effect(() => {
    const pairing = readPairing()
    const endpoint = pairing?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
    if (endpoint === '') return
    let stale = false
    readToken().then((token) => {
      if (stale) return
      return fetchViews({ endpoint, token }).then(
        (res) => {
          if (!stale) savedViews = res.views
        },
        () => {
          // Section simply stays hidden — nothing here is worth a banner.
        },
      )
    })
    return () => {
      stale = true
    }
  })

  function runView(v: SavedView): void {
    activeViewId = activeViewId === v.id ? null : v.id
  }

  $effect(() => {
    inputEl?.focus()
  })
</script>

<section class="m-main scroll-region" data-testid="search-screen">
  <!-- One row-button shape for the pool-backed lists (locals, view rows) —
       the FTS hits list stays separate: it carries snippets and key-only
       rows the pool does not know. -->
  {#snippet rowButton(row: QueueRowFull, testid: string)}
    <button
      class="s-row"
      type="button"
      onclick={() => onopen?.(row.issue_key)}
      data-testid={testid}
      data-key={row.issue_key}
    >
      <span class="s-row-main">
        <span class="s-dot" style:background={categoryInk(row.status_category)} aria-hidden="true"></span>
        <span class="m-row-key">{row.issue_key}</span>
        <span class="s-summary">{row.summary}</span>
        <span class="s-row-status" style:color={categoryInk(row.status_category)}>
          {row.status}
        </span>
      </span>
    </button>
  {/snippet}

  <form class="s-form" onsubmit={onsubmit}>
    <input
      bind:this={inputEl}
      bind:value={q}
      class="s-input"
      type="search"
      enterkeyhint="search"
      autocapitalize="off"
      autocorrect="off"
      spellcheck="false"
      placeholder={t('search.placeholder')}
      aria-label={t('search.title')}
      data-testid="search-input"
    />
    <button class="s-submit" type="submit" data-testid="search-submit">
      {t('search.action')}
    </button>
  </form>

  {#if error !== null}
    <div class="m-banner" role="alert" data-testid="search-error">
      <p>{t(errorMessageKey(error))}</p>
    </div>
  {/if}

  {#if searching}
    <p class="s-status" data-testid="search-searching">{t('search.searching')}</p>
  {/if}

  {#if showResults && results !== null && submitted !== null}
    <div class="s-results-head">
      {#if looksLikeJql(submitted)}
        <p class="s-advanced" data-testid="search-advanced">
          {t('search.advanced', { q: submitted })}
        </p>
      {/if}
      <!-- keys.length, not total: total counts wiki page hits too and this
           screen is Issues-only (flow-report Q3 ③ measured demo total 20 =
           17 issue keys + 3 pages). -->
      <p class="s-count" data-testid="search-count">
        {t('search.results.count', { n: results.keys.length })}
      </p>
    </div>
    {#if hits.length > 0}
      <ul class="s-rows" data-testid="search-hits">
        {#each hits as h (h.key)}
          {@const m = results?.matches?.[h.key]}
          {@const snippet =
            m && m.snippet !== '' && !(h.row !== null && titleShowsQuery(h.row.summary, submitted))
              ? m.snippet
              : null}
          <li>
            <button
              class="s-row"
              type="button"
              onclick={() => onopen?.(h.key)}
              data-testid="search-hit"
              data-key={h.key}
              data-pool={h.row !== null ? '1' : '0'}
            >
              <span class="s-row-main">
                <span
                  class="s-dot"
                  style:background={h.row !== null
                    ? categoryInk(h.row.status_category)
                    : 'var(--color-border-strong)'}
                  aria-hidden="true"
                ></span>
                <span class="m-row-key">{h.key}</span>
                {#if h.row !== null}
                  <span class="s-summary">{h.row.summary}</span>
                  <span class="s-row-status" style:color={categoryInk(h.row.status_category)}>
                    {h.row.status}
                  </span>
                {/if}
              </span>
              {#if snippet !== null}
                <span class="s-snippet" data-testid="search-snippet">{snippet}</span>
              {/if}
            </button>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="m-empty" data-testid="search-empty">
        <p>{t('search.empty', { q: submitted })}</p>
      </div>
    {/if}
  {:else if q.trim() !== ''}
    {#if jumpKey !== null}
      <button
        class="s-jump"
        type="button"
        onclick={() => onopen?.(jumpKey)}
        data-testid="search-jump"
        data-key={jumpKey}
      >
        <span class="s-jump-label">{t('search.jump', { key: jumpKey })}</span>
        {#if jumpPoolRow !== null}
          <span class="s-summary">{jumpPoolRow.summary}</span>
        {/if}
      </button>
    {/if}
    {#if locals.length > 0}
      <p class="s-section">{t('search.local')}</p>
      <ul class="s-rows" data-testid="search-locals">
        {#each locals as row (row.issue_key)}
          <li>
            {@render rowButton(row, 'search-local')}
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
    <p class="s-section">{t('search.recent')}</p>
    {#if recent.length > 0}
      <div class="s-chips" data-testid="search-recent">
        {#each recent as text (text)}
          <button class="s-chip" type="button" onclick={() => runRecent(text)}>{text}</button>
        {/each}
      </div>
    {:else}
      <p class="s-none" data-testid="search-recent-empty">{t('search.recent.empty')}</p>
    {/if}

    <!-- Saved views (A-nav): chips over the local pool. Names only; the
         interpretation is viewChipPlan's — web-only axes say so instead of
         half-answering. -->
    {#if savedViews !== null && savedViews.length > 0}
      <p class="s-section">{t('search.views')}</p>
      <div class="s-chips" data-testid="search-views">
        {#each savedViews as v (v.id)}
          <button
            class="s-chip"
            class:s-chip-on={activeViewId === v.id}
            type="button"
            aria-pressed={activeViewId === v.id}
            onclick={() => runView(v)}
            data-testid="search-view-chip"
          >
            {v.name}
          </button>
        {/each}
      </div>
      {#if activeView !== null}
        {#if activeView.plan.kind === 'local'}
          <p class="s-count" data-testid="search-view-count">
            {t('search.view.count', { n: activeView.plan.rows.length })}
          </p>
          {#if activeView.plan.rows.length > 0}
            <ul class="s-rows" data-testid="search-view-rows">
              {#each activeView.plan.rows as row (row.issue_key)}
                <li>{@render rowButton(row, 'search-view-row')}</li>
              {/each}
            </ul>
          {:else}
            <p class="s-none" data-testid="search-view-empty">{t('search.view.empty')}</p>
          {/if}
        {:else}
          <p class="s-none" data-testid="search-view-web">{t('search.view.webOnly')}</p>
        {/if}
      {/if}
    {/if}
  {/if}
</section>

<style>
  /* Scoped search styles — app.css is frozen this round; tokens only, no new hex. */
  .s-form {
    display: flex;
    gap: 0.5rem;
    padding: 0.5rem 0.25rem;
  }

  .s-input {
    flex: 1 1 auto;
    min-height: 44px; /* iOS touch target — the web 42px control token is not reused */
    padding: 0 0.75rem;
    border: 1px solid var(--color-border-strong);
    border-radius: 6px;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
    font-size: var(--text-body);
    font-family: var(--font-sans);
  }

  .s-submit {
    min-height: 44px;
    padding: 0 1rem;
    border: 0;
    border-radius: 6px;
    background: var(--color-accent);
    color: var(--color-bg-base);
    font-size: var(--text-body);
    font-family: var(--font-sans);
  }

  .s-status,
  .s-count {
    margin: 0.5rem 0 0;
    padding: 0 0.25rem;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }

  .s-advanced {
    margin: 0.5rem 0 0.125rem;
    padding: 0 0.25rem;
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .s-section {
    margin: 0.75rem 0 0.25rem;
    padding: 0 0.25rem;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }

  .s-none {
    margin: 0.75rem 0 0;
    padding: 0 0.25rem;
    color: var(--color-text-muted);
    font-size: var(--text-body);
  }

  .s-jump {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    min-height: 44px;
    margin-top: 0.5rem;
    padding: 0 0.75rem;
    border: 1px solid var(--color-accent-subtle);
    border-radius: 6px;
    background: var(--color-bg-elevated);
    font-family: var(--font-sans);
    text-align: left;
  }

  .s-jump-label {
    flex: none;
    color: var(--color-accent-text);
    font-family: var(--font-mono);
    font-size: var(--text-body);
    font-weight: 600;
  }

  .s-rows {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .s-row {
    display: flex;
    flex-direction: column;
    width: 100%;
    border: 0;
    border-bottom: 1px solid var(--color-border-subtle);
    background: transparent;
    font-family: var(--font-sans);
    text-align: left;
  }

  .s-row-main {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 44px;
    padding: 0 0.25rem;
  }

  .s-dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  /* Titles wrap instead of truncating — Dynamic Type grows height, never cuts (chunk contract). */
  .s-summary {
    flex: 1 1 auto;
    min-width: 0;
    color: var(--color-text-primary);
    font-size: var(--text-body);
    overflow-wrap: anywhere;
    text-align: left;
  }

  .s-row-status {
    flex: none;
    font-size: var(--text-micro);
    font-weight: 500;
  }

  .s-snippet {
    margin: 0 0 0.375rem 1.25rem;
    padding: 0 0.25rem;
    color: var(--color-text-secondary);
    font-size: var(--text-micro);
  }

  .s-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    padding: 0.25rem 0.25rem 0.5rem;
  }

  .s-chip {
    min-height: 44px;
    padding: 0 0.875rem;
    border: 1px solid var(--color-border-strong);
    border-radius: 6px;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
    font-size: var(--text-body);
    font-family: var(--font-sans);
  }

  /* The tapped saved-view chip — tokens only (accent family, as s-jump). */
  .s-chip-on {
    border-color: var(--color-accent);
    color: var(--color-accent-text);
  }
</style>
