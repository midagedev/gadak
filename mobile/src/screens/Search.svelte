<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import Row from '../ui/Row.svelte'
  import DocRow from '../ui/DocRow.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import { app, recentSearches, rememberSearch } from '../lib/store.svelte'
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

  // Local-first: the snapshot answers instantly while typing; the server
  // adds body/comment matches the lite rows cannot see (DESIGN.md §5).
  // Page hits arrive only with the server reply — they are not in the issue snapshot.
  const local = $derived(matchLocal(app.issues, query))
  const results = $derived<IssueLite[]>(mergeSearch(local, serverKeys, app.issues))

  function onInput() {
    serverKeys = []
    serverPages = []
    serverMatches = {}
    if (debounce) clearTimeout(debounce)
    const q = query.trim()
    if (q.length < 2) return
    debounce = setTimeout(async () => {
      try {
        const res = await request<SearchResponse>(
          `issues/search/?q=${encodeURIComponent(q)}&limit=50`,
        )
        // Ignore a reply that raced a newer query.
        if (q === query.trim() && res.body) {
          serverKeys = res.body.keys
          serverPages = res.body.pages ?? []
          serverMatches = res.body.matches ?? {}
          rememberSearch(q)
          recents = recentSearches()
        }
      } catch {
        // Search degrades to local-only offline; the snapshot already answered.
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
        <button class="clear" onclick={clear} aria-label="Clear search">×</button>
      {/if}
    </div>
  {/snippet}

  {#if query.trim() === ''}
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
        <EmptyState
          title="Search the mirror"
          body={`Keys and summaries answer instantly from the ${app.issues.length}-issue snapshot; the server adds comment matches.`}
        />
      {/if}
    </div>
  {:else if results.length === 0 && serverPages.length === 0}
    <EmptyState title="No matches" body="Nothing on this mirror matches that. Comment text needs at least 2 characters." />
  {:else}
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
