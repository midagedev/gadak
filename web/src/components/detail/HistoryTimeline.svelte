<script lang="ts">
  /*
   * Change history timeline ([detail]).
   * Compact status/assignee/priority changes (from→to, by, relative time).
   * Reopen (done-category → non-done) gets a red point.
   */
  import { t } from '../../lib/i18n'
  import type { HistoryEntry } from '../../lib/types'
  import { isReopen } from '../../lib/view-config'
  import { relativeTime, absoluteTime } from './format'
  import BotBadge from '../list/BotBadge.svelte'

  let { history }: { history: HistoryEntry[] } = $props()

  /** Field label (via i18n). */
  function fieldLabel(f: string): string {
    return f === 'status' ? t('common.status') : f === 'assignee' ? t('common.assignee') : f === 'priority' ? t('common.priority') : f
  }
</script>

<ol class="relative flex flex-col gap-3 pl-4">
    <!-- Vertical guide line -->
    <span
      class="absolute top-1 bottom-1 left-[3px] w-px bg-border-subtle"
      aria-hidden="true"
    ></span>
    {#each history as e, i (i)}
      {@const reopen = isReopen(e)}
      <li class="relative">
        <!-- Timeline point -->
        <span
          class="absolute top-[5px] -left-4 h-[7px] w-[7px] rounded-full ring-2 ring-bg-panel"
          class:bg-status-reopen={reopen}
          class:bg-border-strong={!reopen}
          aria-hidden="true"
        ></span>
        <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-body">
          <span class="font-medium text-text-secondary">{fieldLabel(e.field)}</span>
          {#if reopen}
            <span class="rounded bg-status-reopen/15 px-1 text-micro font-semibold text-status-reopen">
              {t('feed.kindReopen')}
            </span>
          {/if}
          <span class="text-text-muted">
            {e.from ?? t('common.none')}
            <span class="mx-0.5 text-text-muted">→</span>
            <span class="text-text-primary">{e.to ?? t('common.none')}</span>
          </span>
        </div>
        <div class="text-micro text-text-muted">
          {#if e.by}{e.by} <BotBadge accountId={e.author_id} /> · {/if}<span title={absoluteTime(e.at)}
            >{relativeTime(e.at)}</span
          >
        </div>
      </li>
    {/each}
  </ol>
