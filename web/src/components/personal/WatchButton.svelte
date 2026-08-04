<script lang="ts">
  /*
   * Watch toggle ([personal]). Eye icon + watching state.
   *  - Identified: optimistic toggle (me.toggleWatch, rollback on failure).
   *  - No credential: open credential settings.
   */
  import { t } from '../../lib/i18n'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'

  let { issueKey }: { issueKey: string } = $props()

  const watching = $derived(me.watches.has(issueKey))
  let busy = $state(false)

  async function onClick(e: MouseEvent) {
    e.stopPropagation()
    if (!me.identified) {
      write.openSettings()
      return
    }
    if (busy) return
    busy = true
    await me.toggleWatch(issueKey)
    busy = false
  }
</script>

<button
  type="button"
  onclick={onClick}
  class="flex h-6 items-center gap-1 rounded-md px-2 text-[11px] font-medium transition-colors {watching
    ? 'bg-accent-subtle/40 text-accent-text hover:bg-accent-subtle/60'
    : 'text-text-muted hover:bg-bg-hover hover:text-text-primary'}"
  title={me.identified
    ? watching
      ? t('personal.watchOn')
      : t('personal.watchOff')
    : t('personal.watchNeedCredentials')}
  aria-pressed={watching}
>
  {#if watching}
    <!-- Open eye -->
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8s-2.4 4.5-6.5 4.5S1.5 8 1.5 8Z"
        stroke="currentColor"
        stroke-width="1.3"
      />
      <circle cx="8" cy="8" r="2" fill="currentColor" />
    </svg>
    <span>{t('common.watching')}</span>
  {:else}
    <!-- Closed eye -->
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path
        d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8s-2.4 4.5-6.5 4.5S1.5 8 1.5 8Z"
        stroke="currentColor"
        stroke-width="1.3"
      />
      <circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3" />
    </svg>
    <span>{t('common.watch')}</span>
  {/if}
</button>
