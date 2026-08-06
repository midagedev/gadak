<script lang="ts">
  /*
   * Change history timeline ([detail]).
   * Compact status/assignee/priority changes (from→to, by, relative time).
   * Reopen (resolved → unresolved status transition) gets a red point.
   */
  import { t } from '../../lib/i18n'
  import type { HistoryEntry } from '../../lib/types'
  import { RESOLVED_STATUS_NAMES } from '../../lib/view-config'
  import { relativeTime, absoluteTime } from './format'

  let { history }: { history: HistoryEntry[] } = $props()

  function isResolved(s: string | null): boolean {
    return !!s && RESOLVED_STATUS_NAMES.has(s.trim().toLowerCase())
  }

  /**
   * Is this status transition a reopen (resolved→unresolved)? Prefer server
   * before/after categories; only fall back to status names (locale-dependent).
   */
  function isReopen(e: HistoryEntry): boolean {
    if (e.field !== 'status') return false
    if (e.from_category || e.to_category) return e.from_category === 'done' && e.to_category !== 'done'
    return isResolved(e.from) && !isResolved(e.to)
  }

  /** Field label (via i18n). */
  function fieldLabel(f: string): string {
    return f === 'status' ? t('common.status') : f === 'assignee' ? t('common.assignee') : f === 'priority' ? t('common.priority') : f
  }
</script>

{#if history.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noHistory')}</p>
{:else}
  <ol class="relative flex flex-col gap-2.5 pl-4">
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
        <div class="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 text-[12px]">
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
        <div class="text-[11px] text-text-muted">
          {#if e.by}{e.by} · {/if}<span title={absoluteTime(e.at)}>{relativeTime(e.at)}</span>
        </div>
      </li>
    {/each}
  </ol>
{/if}
