<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import DocRow from '../ui/DocRow.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import { app, recentSearches, rememberSearch, searchPaint } from '../lib/store.svelte'
  import { matchLocal, mergeSearch } from '../lib/domain'
  import { request } from '../lib/api'
  import { t } from '../lib/i18n'
  import type { SearchResponse, IssueLite, PageLite, SearchMatch } from '../lib/types'

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

  // Local-first: the snapshot answers instantly while typing; the server
  // adds body/comment matches the lite rows cannot see (DESIGN.md §5).
  // Page hits arrive only with the server reply — they are not in the issue snapshot.
  const local = $derived(matchLocal(app.issues, query))
  const results = $derived<IssueLite[]>(mergeSearch(local, serverKeys, app.issues))
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
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <h1 class="type-subject">Search</h1>
    </div>
    <div class="field">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
        <circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" />
      </svg>
      <input
        bind:this={inputEl}
        bind:value={query}
        oninput={onInput}
        type="search"
        placeholder="Key, summary, comment…"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        enterkeyhint="search"
      />
      {#if query}
        <button class="clear" onclick={clear} aria-label={t('list.clearSearch')}>×</button>
      {/if}
    </div>
  {/snippet}

  {#if plate === 'idle'}
    <div class="idle">
      {#if recents.length > 0}
        <p class="idle-label">Recent</p>
        {#each recents as r (r)}
          <button class="recent" onclick={() => runRecent(r)}>
            <span class="r-q">{r}</span>
            <span class="r-go" aria-hidden="true">↑</span>
          </button>
        {/each}
      {:else}
        <p class="idle-hint">Keys and summaries answer instantly from the {app.issues.length}-issue snapshot; the server adds comment matches.</p>
      {/if}
    </div>
  {:else if plate === 'results'}
    {#each results as issue (issue.issue_key)}
      <Row {issue} showAssignee={true} />
    {/each}
    {#if serverPages.length > 0}
      <div class="section">
        <span class="label">{t('sidebar.docs')}</span>
        <span class="n">{serverPages.length}</span>
      </div>
      {#each serverPages as page (page.key)}
        <DocRow {page} showSpace={true} showExcerpt={false} snippet={serverMatches[page.key]?.snippet ?? ''} />
      {/each}
    {/if}
  {:else if plate === 'searching'}
    <p class="idle-hint">{t('common.searching')}</p>
  {:else if plate === 'failed'}
    <EmptyState title={t('list.searchFailed')}>
      <button class="link" onclick={onInput}>{t('list.searchRetry')}</button>
    </EmptyState>
  {:else}
    <EmptyState title={t('common.noResults')} />
  {/if}
</Screen>

<style>
  .head {
    display: flex;
    align-items: baseline;
    padding: 12px 0 10px;
  }
  h1 {
    margin: 0;
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
  }
  .field {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: var(--spacing-control);
    margin-bottom: 10px;
    padding: 0 12px;
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
  .idle {
    padding: 8px 0;
  }
  .idle-label {
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
  .link {
    color: var(--color-accent-text);
    font-size: var(--text-body);
    min-height: var(--spacing-control);
    padding: 0 16px;
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
  .n {
    font-family: var(--font-mono);
  }
</style>
