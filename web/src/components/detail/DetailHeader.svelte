<script lang="ts">
  /*
   * Detail panel header ([detail]).
   * Renders immediately from local-pool IssueLite alone (latency hide); independent
   * of detail load. Key/title/status/priority/severity/assignee/labels/versions/
   * group/reopen badge + close.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { selection } from '../../stores/selection.svelte'
  import { me } from '../../stores/me.svelte'
  import { jiraUrl } from './format'
  import WatchButton from '../personal/WatchButton.svelte'
  import StatusTransition from '../write/StatusTransition.svelte'
  import AssigneePicker from '../write/AssigneePicker.svelte'

  let { issue }: { issue: IssueLite } = $props()

  const isFavorite = $derived(me.favorites.has(issue.issue_key))
</script>

<header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
  <!-- Top row: issue key (Jira link) + close -->
  <div class="mb-2 flex items-center justify-between gap-2">
    <div class="flex min-w-0 items-center gap-2">
      <a
        href={jiraUrl(issue.issue_key)}
        target="_blank"
        rel="noopener noreferrer"
        class="flex-none font-mono text-[12px] font-medium text-accent-text hover:underline"
        title={t('detail.openJira')}
      >
        {issue.issue_key}
      </a>
    </div>
    <div class="flex flex-none items-center gap-1">
      <!-- Favorite toggle -->
      <button
        type="button"
        onclick={() => void me.toggleFavorite(issue.issue_key)}
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-[13px] transition-colors hover:bg-bg-hover {isFavorite
          ? 'text-status-stale'
          : 'text-text-muted hover:text-text-primary'}"
        aria-pressed={isFavorite}
        aria-label={isFavorite ? t('common.unfavorite') : t('common.favorite')}
        title={isFavorite ? t('common.unfavorite') : t('common.favorite')}
      >
        {isFavorite ? '★' : '☆'}
      </button>
      <!-- Watch -->
      <WatchButton issueKey={issue.issue_key} />
      <!-- Close -->
      <button
        type="button"
        onclick={() => selection.clear()}
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        aria-label={t('common.close')}
        title={t('common.closeEsc')}
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
        </svg>
      </button>
    </div>
  </div>

  <!-- Title -->
  <h2 class="mb-3 text-[16px] leading-snug font-semibold text-text-primary">
    {issue.summary}
  </h2>

  <!-- Meta chip row -->
  <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
    <!-- Status (click → transition dropdown) -->
    <StatusTransition {issue} />

    {#if issue.issue_type}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.issue_type}</span>
    {/if}
    {#if issue.priority}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{t('detail.priorityShort', { p: issue.priority })}</span>
    {/if}
    {#if issue.severity}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{t('detail.severityShort', { s: issue.severity })}</span>
    {/if}
    {#if issue.d1_group}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.d1_group}</span>
    {/if}

    <!-- Reopen badge -->
    {#if issue.reopen_count > 0}
      <span
        class="inline-flex items-center gap-1 rounded-md bg-status-reopen/15 px-2 py-0.5 font-semibold text-status-reopen"
        title={issue.reopen_reason ?? t('detail.reopened')}
      >
        {t('detail.reopenTimes', { n: issue.reopen_count })}
      </span>
    {/if}
  </div>

  <!-- Assignee + labels/versions -->
  <div class="mt-3 flex flex-col gap-2 text-[12px] text-text-muted">
    <!-- Assignee (click → assign popover; works when unassigned too) -->
    <AssigneePicker {issue} />
    {#if issue.fix_versions.length > 0}
      <div class="flex items-start gap-1.5">
        <span class="w-12 flex-none pt-0.5 text-text-muted">{t('common.version')}</span>
        <span class="flex flex-wrap gap-1">
          {#each issue.fix_versions as v (v)}
            <span class="rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary">{v}</span>
          {/each}
        </span>
      </div>
    {/if}
    {#if issue.labels.length > 0}
      <div class="flex items-start gap-1.5">
        <span class="w-12 flex-none pt-0.5 text-text-muted">{t('common.labels')}</span>
        <span class="flex flex-wrap gap-1">
          {#each issue.labels as l (l)}
            <span class="rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary">{l}</span>
          {/each}
        </span>
      </div>
    {/if}
  </div>
</header>
