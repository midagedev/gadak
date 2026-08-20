<script lang="ts">
  /*
   * Detail panel header ([detail]).
   * Renders immediately from local-pool IssueLite alone (latency hide); independent
   * of detail load. Key/title/status/priority/severity/assignee/labels/versions/
   * group/reopen badge + close.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { config, isDesktop, profileName, workspaceName } from '../../lib/config'
  import { copyText } from '../../lib/copy-text'
  import { selection } from '../../stores/selection.svelte'
  import { favorites } from '../../stores/favorites.svelte'
  import { write } from '../../stores/write.svelte'
  import { jiraUrl } from './format'
  import IssueBreadcrumb from './IssueBreadcrumb.svelte'
  import WatchButton from '../personal/WatchButton.svelte'
  import StatusTransition from '../write/StatusTransition.svelte'
  import PriorityPicker from '../write/PriorityPicker.svelte'
  import AssigneePicker from '../write/AssigneePicker.svelte'
  import LabelEditor from '../write/LabelEditor.svelte'
  import TitleEditor from '../write/TitleEditor.svelte'
  import Icon from '../ui/Icon.svelte'

  let { issue, overlay = false }: { issue: IssueLite; overlay?: boolean } = $props()

  const isFavorite = $derived(favorites.keys.has(issue.issue_key))

  // Same hash the CLI's deepLinkURL / composeServeURL pass through:
  // "issue=KEY" with no leading ? or #. /w/<profile> only for a named
  // non-default profile (config().profile is the server's document).
  function gadakIssueLink(key: string): string {
    const p = profileName(config().profile)
    const prefix = p !== 'default' ? `/w/${p}` : ''
    return `gadak://view${prefix}?issue=${key}`
  }

  function httpIssueLink(key: string): string {
    const ws = workspaceName()
    const prefix = ws ? `/w/${ws}` : ''
    return `${location.origin}${prefix}/#/?issue=${key}`
  }

  async function copyLink(): Promise<void> {
    const key = issue.issue_key
    const gadak = gadakIssueLink(key)
    // Desktop has no shareable http origin (in-process webview). Serve/hosted
    // copy both lines so a paste into Slack still works without the app.
    const text = isDesktop() ? gadak : `${gadak}\n${httpIssueLink(key)}`
    if (await copyText(text)) {
      write.toast(t('detail.linkCopied'), 'success')
    } else {
      write.toast(t('clipboard.copyFailed'), 'error')
    }
  }
</script>

<header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
  <!-- Top row: issue key (Jira link) + close -->
  <div class="mb-2 flex items-center justify-between gap-2">
    <div class="flex min-w-0 items-center gap-2">
      {#if overlay}
        <!-- GDK-463: overlay covers the list. Same arrow-left control as the
             feed header (PersonalFeed); X and the scrim still close. -->
        <button
          type="button"
          onclick={() => selection.clear()}
          data-testid="issue-detail-back"
          class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          aria-label={t('feed.backToList')}
          title={t('feed.backToList')}
        >
          <Icon name="arrow-left" size={14} />
        </button>
      {/if}
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
        onclick={() => void favorites.toggle(issue.issue_key)}
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-body transition-colors hover:bg-bg-hover {isFavorite
          ? 'text-status-stale'
          : 'text-text-muted hover:text-text-primary'}"
        aria-pressed={isFavorite}
        aria-label={isFavorite ? t('common.unfavorite') : t('common.favorite')}
        title={isFavorite ? t('common.unfavorite') : t('common.favorite')}
      >
        <Icon name="star" size={14} filled={isFavorite} />
      </button>
      <!-- Copy gadak:// (and http, off desktop) — same 24px icon-button cluster -->
      <button
        type="button"
        onclick={() => void copyLink()}
        data-testid="issue-copy-link"
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        aria-label={t('detail.copyLink')}
        title={t('detail.copyLink')}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <path
            d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
      <!-- Watch -->
      <WatchButton issueKey={issue.issue_key} />
      <!-- Close -->
      <button
        type="button"
        onclick={() => selection.clear()}
        data-testid="issue-detail-close"
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

  <!-- Where the issue sits in its epic. Renders nothing when it has no ancestors. -->
  <IssueBreadcrumb {issue} />

  <!-- Title. Same type-subject line; click to rename. -->
  <TitleEditor {issue} />

  <!-- Meta chip row -->
  <div class="flex flex-wrap items-center gap-2 text-micro">
    <!-- Status (click → transition dropdown) -->
    <StatusTransition {issue} />

    {#if issue.issue_type}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.issue_type}</span>
    {/if}
    <PriorityPicker {issue} />
    {#if issue.severity}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{t('detail.severityShort', { s: issue.severity })}</span>
    {/if}
    {#if issue.team_group}
      <span class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary">{issue.team_group}</span>
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
      <div class="flex items-start gap-2">
        <span class="w-12 flex-none pt-0.5 text-text-muted">{t('common.version')}</span>
        <span class="flex flex-wrap gap-1">
          {#each issue.fix_versions as v (v)}
            <span class="rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary">{v}</span>
          {/each}
        </span>
      </div>
    {/if}
    <LabelEditor {issue} />
  </div>
</header>
