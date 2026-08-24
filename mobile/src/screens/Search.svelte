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
</script>

<script lang="ts">
  import {
    ApiError,
    categoryInk,
    search as searchIssues,
    type ApiContext,
    type SearchResult,
  } from '../lib/api'
  import { readPairing, readToken } from '../lib/settings'
  import { t, type MessageKey } from '../lib/i18n'
  import { readRecentSearches, recordRecentSearch } from '../lib/recent-searches'

  let {
    rows = [],
    onopen,
  }: { rows?: QueueRow[]; onopen?: (issue_key: string) => void } = $props()

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

  $effect(() => {
    inputEl?.focus()
  })
</script>

<section class="m-main scroll-region" data-testid="search-screen">
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
            <button
              class="s-row"
              type="button"
              onclick={() => onopen?.(row.issue_key)}
              data-testid="search-local"
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
</style>
