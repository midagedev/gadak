<script lang="ts">
  /*
   * Detail panel header ([detail]).
   * Renders immediately from local-pool IssueLite alone (latency hide); independent
   * of detail load. Key/title/status/priority/severity/assignee/labels/versions/
   * group/reopen badge + close.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import {
    config,
    isDesktop,
    originTrackerName,
    profileName,
    workspaceName,
  } from '../../lib/config'
  import { copyText } from '../../lib/copy-text'
  import { selection } from '../../stores/selection.svelte'
  import { reading } from '../../stores/reading.svelte'
  import { favorites } from '../../stores/favorites.svelte'
  import { write } from '../../stores/write.svelte'
  import { openIssueOrigin } from '../../lib/desktop-links'
  import { issueOriginUrl } from '../../lib/issue-origin'
  import { formatSpan } from '../../lib/format'
  import { shellForIssue, shouldMarkUnattended } from '../../lib/issue-shells'
  import { shells } from '../../lib/issue-shells.svelte'
  import { enterShell, terminalSessions } from '../../lib/terminal/sessions.svelte'
  import { isHostedDemo } from '../../lib/config'
  import IssueBreadcrumb from './IssueBreadcrumb.svelte'
  import WatchButton from '../personal/WatchButton.svelte'
  import StatusTransition from '../write/StatusTransition.svelte'
  import PriorityPicker from '../write/PriorityPicker.svelte'
  import TitleEditor from '../write/TitleEditor.svelte'
  import Icon from '../ui/Icon.svelte'

  let {
    issue,
    overlay = false,
    waitMs = null,
    progressMs = null,
  }: {
    issue: IssueLite
    overlay?: boolean
    /** Lifecycle spans from the detail response (server's Durations). Absent → no chip. */
    waitMs?: number | null
    progressMs?: number | null
  } = $props()

  const isFavorite = $derived(favorites.keys.has(issue.issue_key))

  // GDK-1164-A. In progress, and no shell this serve knows is on it. Read
  // only — nothing here or below it unclaims anything; see lib/issue-shells.ts
  // for why detecting death and recording it are different layers.
  const unattended = $derived(shouldMarkUnattended(issue, shells.sessions))

  // The header's shell verb (GDK-1388): the shell already on this issue, or
  // a new one bound to it from its first prompt — the same binding `gadak
  // claim` would make later, without the claim. Hidden on the hosted demo,
  // which has no serve to open a shell on.
  const boundShell = $derived(shellForIssue(shells.sessions, issue.issue_key))
  const canShell = $derived(!isHostedDemo())
  function openShell(): void {
    if (boundShell) enterShell(boundShell.id)
    else terminalSessions.openNewFor(issue.issue_key)
  }

  // "Waited 3d · In progress 5h" — the CLI durations line's numbers. Parts a
  // span cannot answer drop out; with none, the chip does not render.
  const durationsLabel = $derived(
    [
      formatSpan(waitMs) && t('detail.waitSpan', { span: formatSpan(waitMs) }),
      formatSpan(progressMs) && t('detail.progressSpan', { span: formatSpan(progressMs) }),
    ]
      .filter(Boolean)
      .join(' · '),
  )

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
    // The origin's page for this key — the address that survives a paste into
    // chat. Same resolution as the key anchor above (issueOriginUrl: the
    // row's stored url, else the site's /browse/KEY); null on the built-in
    // tracker, where the app links below are the only shareable address.
    const originUrl = issueOriginUrl(key)
    // Desktop has no shareable http origin (in-process webview). Serve/hosted
    // copy both lines so a paste into Slack still works without the app.
    const appText = isDesktop() ? gadak : `${gadak}\n${httpIssueLink(key)}`
    const text = originUrl ? `${originUrl}\n${appText}` : appText
    if (await copyText(text)) {
      write.toast(
        originUrl
          ? t('detail.originLinkCopied', { tracker: originTrackerName() })
          : t('detail.linkCopied'),
        'success',
      )
    } else {
      write.toast(t('clipboard.copyFailed'), 'error')
    }
  }
</script>

<header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
  <!-- Top row: issue key (origin link) + close -->
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
      {#if issueOriginUrl(issue.issue_key)}
        <a
          href={issueOriginUrl(issue.issue_key)}
          target="_blank"
          rel="noopener noreferrer"
          class="flex-none font-mono text-micro font-medium text-accent-text hover:underline"
          title={t('detail.openJira', { tracker: originTrackerName() })}
          onclick={(e) => {
            if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
            e.preventDefault()
            openIssueOrigin(issue.issue_key)
          }}
        >
          {issue.issue_key}
        </a>
      {:else}
        <!-- No origin page (built-in tracker, or a Linear row without its url yet):
             the key is a label — a link-styled <a> with no href promised an action
             that did nothing (GDK-1313). -->
        <span class="flex-none font-mono text-micro font-medium text-text-secondary" data-testid="issue-key-label">
          {issue.issue_key}
        </span>
      {/if}
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
      <!-- Copy the origin URL, then gadak:// (+ http, off desktop) — same 24px icon-button cluster -->
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
      <!-- Reading width (GDK-1311): the list keeps its minimum, the body gets the rest. -->
      <button
        type="button"
        onclick={() => reading.toggle()}
        data-testid="issue-detail-wide"
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary aria-pressed:text-text-primary"
        aria-pressed={reading.wide}
        aria-label={reading.wide ? t('detail.wideOff') : t('detail.wideOn')}
        title={reading.wide ? t('detail.wideOff') : t('detail.wideOn')}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          {#if reading.wide}
            <path d="M9 4l-6 8 6 8M15 4l6 8-6 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          {:else}
            <path d="M8 4l-5 8 5 8M16 4l5 8-5 8M3 12h18" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          {/if}
        </svg>
      </button>
      <!-- Shell (GDK-1388): enter the bound one, or open one bound to this issue. -->
      {#if canShell}
        <button
          type="button"
          onclick={openShell}
          data-testid="issue-open-shell"
          data-bound={boundShell ? 'true' : 'false'}
          class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          aria-label={boundShell
            ? t('detail.enterShell', { key: issue.issue_key })
            : t('detail.openShell', { key: issue.issue_key })}
          title={boundShell
            ? t('detail.enterShell', { key: issue.issue_key })
            : t('detail.openShell', { key: issue.issue_key })}
        >
          <Icon name="terminal" size={14} />
        </button>
      {/if}
      <!-- Watch -->
      <WatchButton issueKey={issue.issue_key} />
      <!-- Close -->
      <button
        type="button"
        onclick={() => selection.clear()}
        data-testid="issue-detail-close"
        class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        aria-label={t('common.closeEsc')}
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
    <!-- Type is a property, not a state: it lives in the list below (GDK-1337).
         The row keeps what changes or alarms — status, priority, reopen — plus
         the quiet derived chips. -->
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

    <!-- No shell here (GDK-1164-A). Last in the row, beside the duration chip
         and in its grammar: both are quiet things derived about the issue
         rather than fields of it, and this one is information, not an alarm.
         A colored badge would read as an accusation, and what is actually
         known is only "this serve does not see a shell on it" — which a
         sleeping laptop makes true of live work on another machine. The title
         says so in full.

         After the reopen badge, not before: the reopen badge is the loud,
         colored one, and pushing it onto a second line to make room for a
         muted note inverts which of the two the eye finds first (measured on
         the capture — NMS-3 at 1600px wrapped exactly that way). -->
    {#if unattended}
      <span
        class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-muted"
        data-testid="unattended-chip"
        title={t('detail.unattendedHint')}
      >
        {t('detail.unattended')}
      </span>
    {/if}

    <!-- Lifecycle spans (GDK-590): waited / in-progress, from the changelog -->
    {#if durationsLabel}
      <span
        class="rounded-md bg-bg-elevated px-2 py-0.5 text-text-secondary"
        data-testid="duration-chip"
      >
        {durationsLabel}
      </span>
    {/if}
  </div>

  <!-- Assignee, type, labels, versions: one property list in IssueFields,
       directly under this header (GDK-1337). Two groups with two label widths
       — and Version here, Fix versions there, the same value — were the
       measured defect. -->
</header>
