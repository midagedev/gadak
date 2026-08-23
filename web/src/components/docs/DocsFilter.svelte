<script lang="ts">
  /*
   * The narrowing field on a document screen — the same axis `/` reaches on the
   * issue list, in the place a document list needs it.
   *
   * Two things at one keyboard's reach, kept apart (UX_PRINCIPLES §3): typing
   * narrows the rows in front of you, locally and on every keystroke, and never
   * asks the server; Enter leaves for the whole mirror, which is the issue
   * list's search section — the one home unified results have, where a page hit
   * sits above the issues. The placeholder says so, because a field that
   * silently changes screens on Enter is a trap.
   */
  import Icon from '../ui/Icon.svelte'
  import { t } from '../../lib/i18n'
  import { widenToServerSearch } from '../../lib/server-search'
  import { pages } from '../../stores/pages.svelte'

  let { value = $bindable('') }: { value?: string } = $props()

  let inputEl = $state<HTMLInputElement | null>(null)

  /** Leave this screen, then hand the query to the shared widen. */
  function searchEverything() {
    widenToServerSearch(value, () => pages.closeDocs())
  }

  // Esc is SearchBox's contract: clear what is typed, and give the keyboard back
  // only once there is nothing left to clear.
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      searchEverything()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (value) value = ''
      else inputEl?.blur()
    }
  }
</script>

<div
  class="flex h-control items-center gap-2 rounded-md border border-border-strong/70 bg-bg-elevated px-3 shadow-sm shadow-black/10 focus-within:border-accent/70"
  data-testid="docs-filter"
>
  <Icon name="search" size={14} class="text-text-muted" />
  <input
    bind:this={inputEl}
    bind:value
    onkeydown={onKeydown}
    type="text"
    data-testid="docs-filter-input"
    data-enter="widen"
    placeholder={t('docs.filterPlaceholder')}
    aria-label={t('docs.filterLabel')}
    class="min-w-0 flex-1 bg-transparent text-body text-text-primary placeholder:text-text-muted focus:outline-none"
    spellcheck="false"
    autocomplete="off"
  />
  {#if value}
    <button
      type="button"
      class="flex flex-none items-center text-text-muted hover:text-text-primary"
      onclick={() => {
        value = ''
        inputEl?.focus()
      }}
      title={t('list.searchClear')}
      aria-label={t('list.searchClear')}
      data-testid="docs-filter-clear"
    >
      <Icon name="x" size={13} />
    </button>
  {:else}
    <kbd class="flex-none rounded border border-border-subtle px-1 text-micro text-text-muted">/</kbd>
  {/if}
</div>
