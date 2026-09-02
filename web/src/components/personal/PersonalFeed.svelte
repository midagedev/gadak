<script lang="ts">
  import { t, fieldLabel } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import ColumnHeader from '../ui/ColumnHeader.svelte'
  import type { FeedFocus, FeedItem } from '../../lib/types'
  import { selection } from '../../stores/selection.svelte'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { relativeTime, absTime } from '../../lib/format'
  import EmptyState from '../list/EmptyState.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { groupFeedItems, groupKindCounts, type FeedGroup } from './feed-groups'

  const skeleton = createSkeletonGrace(
    () => me.feedLoading && !me.feedLoaded,
    () => me.feedFocus,
  )

  const TABS: { key: FeedFocus; label: string }[] = [
    { key: 'all', label: t('feed.filterAll') },
    { key: 'assignee', label: t('feed.filterAssignee') },
    { key: 'reporter', label: t('feed.filterReporter') },
    { key: 'mention', label: t('feed.filterMention') },
  ]

  const EVENT_LABELS: Record<FeedItem['event_type'], string> = {
    created: t('feed.kindCreated'),
    status_changed: t('feed.kindStatus'),
    reopened: t('feed.kindReopen'),
    assigned: t('feed.kindAssignee'),
    comment_added: t('feed.kindComment'),
    attachment_added: t('feed.kindAttachment'),
    fields_changed: t('feed.kindField'),
  }

  const REASON_LABELS: Record<string, string> = {
    assignee: t('feed.whyAssignee'),
    assigned: t('feed.whyNewAssignee'),
    reporter: t('feed.whyReporter'),
    // Server emits `watched`; keep `watch` for any older payload.
    watched: t('feed.whyWatch'),
    watch: t('feed.whyWatch'),
    mention: t('feed.whyMention'),
  }

  function payloadString(item: FeedItem, key: string): string {
    const value = item.payload[key]
    return typeof value === 'string' ? value : ''
  }

  function eventDetail(item: FeedItem): string {
    if (item.event_type === 'comment_added') {
      const excerpt = payloadString(item, 'excerpt')
      return item.actor_name ? `${item.actor_name}: ${excerpt}` : excerpt
    }
    if (item.event_type === 'attachment_added') {
      return payloadString(item, 'filename')
    }
    if (
      item.event_type === 'status_changed' ||
      item.event_type === 'reopened' ||
      item.event_type === 'assigned'
    ) {
      return `${payloadString(item, 'from')} → ${payloadString(item, 'to')}`
    }
    if (item.event_type === 'fields_changed') {
      // Server ships field names as payload.fields[]; older shape used changes[].label.
      // Raw ids render through fieldLabel so a person reads display names;
      // unmapped ids degrade visibly to the raw id, never blank (GDK-1055).
      const fields = Array.isArray(item.payload.fields) ? item.payload.fields : []
      const fromFields = fields
        .slice(0, 3)
        .map((f) => (typeof f === 'string' ? fieldLabel(f) : ''))
        .filter(Boolean)
      if (fromFields.length) return fromFields.join(', ')
      const changes = Array.isArray(item.payload.changes) ? item.payload.changes : []
      return changes
        .slice(0, 3)
        .map((change) => {
          if (!change || typeof change !== 'object') return ''
          const label = (change as Record<string, unknown>).label
          return typeof label === 'string' ? fieldLabel(label) : ''
        })
        .filter(Boolean)
        .join(', ')
    }
    return item.current_status
  }

  function selectFocus(focus: FeedFocus) {
    // Re-show with the new focus: same surface, payload swap — and openFeed
    // reloads with it.
    me.openFeed(focus)
  }

  function openItem(item: FeedItem) {
    void me.markEventRead(item.event_id)
    selection.select(item.issue_key)
  }

  // Collapse consecutive events of one issue (same day) into one row group
  // (GDK-1058) — pure function in feed-groups.ts, unit-pinned there.
  const groups = $derived(groupFeedItems(me.feedItems))

  // Expanded groups (local). Keyed by group representative id.
  let expanded = $state<Record<number, boolean>>({})

  function reasonsOf(items: FeedItem[]): string[] {
    return [...new Set(items.flatMap((item) => item.reasons))]
  }

  // Expand a collapsed group and mark all its events read.
  function openGroup(group: FeedGroup) {
    expanded = { ...expanded, [group.id]: true }
    void me.markEventsRead(group.items.map((item) => item.event_id))
  }
</script>

<div class="flex h-full flex-col" data-skeleton={skeleton.attr}>
  <ColumnHeader title={t('feed.title')} closeTestid="feed-close" onClose={() => me.closeFeed()}>
    {#if me.feedUnread.all > 0}
      <span
        class="min-w-5 rounded-full bg-accent px-1.5 py-0.5 text-center text-micro font-semibold text-white"
      >
        {me.feedUnread.all > 99 ? '99+' : me.feedUnread.all}
      </span>
    {/if}

    <!-- p-1 + h-control-sm: the 32px segmented control every column header
         uses (GDK-1339), so the feed's tabs stand as tall as Documents'. -->
    <div
      class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1 max-[760px]:order-last max-[760px]:ml-0 max-[760px]:w-full max-[760px]:overflow-x-auto"
    >
      {#each TABS as tab (tab.key)}
        {@const count = me.feedUnread[tab.key]}
        <button
          type="button"
          class="flex h-control-sm items-center gap-1 rounded px-2 text-micro font-medium {me.feedFocus ===
          tab.key
            ? 'bg-bg-active text-text-primary'
            : 'text-text-muted hover:text-text-secondary'}"
          onclick={() => selectFocus(tab.key)}
        >
          {tab.label}
          {#if count > 0}<span class="text-micro text-accent-text">{count}</span>{/if}
        </button>
      {/each}
    </div>

    {#snippet trailing()}
      {#if me.feedUnread.all > 0}
        <button
          type="button"
          onclick={() => me.markAllFeedRead()}
          class="flex h-control-sm flex-none items-center gap-1 rounded-md px-2 text-micro text-text-secondary hover:bg-bg-hover hover:text-text-primary max-[760px]:w-control-sm max-[760px]:justify-center max-[760px]:px-0"
          title={t('feed.markAllRead')}
        >
          <Icon name="check-check" size={14} />
          <span class="max-[760px]:hidden">{t('feed.markAllRead')}</span>
        </button>
      {/if}
    {/snippet}
  </ColumnHeader>

  <div class="min-h-0 flex-1 overflow-y-auto">
    {#if !me.identified}
      <EmptyState
        icon="settings"
        title={t('feed.needCredentials')}
        actionLabel={t('common.setCredentials')}
        onAction={() => write.openSettings()}
      />
    {:else if me.feedLoading && !me.feedLoaded}
      {#if skeleton.visible}
        <LoadingState label={t('common.loading')} />
      {/if}
    {:else if me.feedLoadFailed && me.feedItems.length === 0}
      <!-- Failure is not emptiness (GDK-1066): the feed request answered with
           an error, so the error line replaces whatever the empty copy would
           say. Same shape as DocsView/HistoryView's load-failure branches
           (GDK-1054); stale rows on a failed reload stay below in the list —
           the connection strip already reports that failure, no banner here. -->
      <div class="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <p class="text-body text-text-secondary" data-testid="feed-load-error">
          {t('feed.loadFailed')}
        </p>
        <button
          type="button"
          onclick={() => void me.loadFeed()}
          class="rounded-md border border-border-strong px-3 py-1.5 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover"
        >
          {t('common.retry')}
        </button>
      </div>
    {:else if me.feedItems.length === 0}
      <EmptyState icon="inbox" title={t('feed.empty')} hint={t('personal.feedHint')} />
    {:else}
      {#snippet feedRow(item: FeedItem)}
        {@const unread = !item.read_at}
        {@const detail = eventDetail(item)}
        <button
          type="button"
          class="group flex w-full items-start gap-2.5 border-b border-border-subtle/60 px-4 py-2.5 text-left transition-colors {selection.selectedKey ===
          item.issue_key
            ? 'bg-bg-active'
            : unread
              ? 'bg-accent/[0.045] hover:bg-bg-hover'
              : 'hover:bg-bg-hover'}"
          onclick={() => openItem(item)}
        >
          <span class="mt-2 flex h-2 w-2 flex-none items-center justify-center">
            {#if unread}<span class="h-1.5 w-1.5 rounded-full bg-accent"></span>{/if}
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="flex-none font-mono text-micro text-text-muted">{item.issue_key}</span>
              <span
                class="min-w-0 flex-1 truncate text-body {unread
                  ? 'font-medium text-text-primary'
                  : 'text-text-secondary'}"
              >{item.summary}</span>
              <span class="flex-none text-micro text-text-muted" title={absTime(item.occurred_at)}>
                {relativeTime(item.occurred_at)}
              </span>
            </div>
            <div class="mt-1 flex min-w-0 items-center gap-1.5 text-micro">
              <span class="flex-none text-text-secondary">{EVENT_LABELS[item.event_type]}</span>
              {#if detail}
                <span class="truncate text-text-muted">{detail}</span>
              {/if}
              <span class="min-w-2 flex-1"></span>
              {#each item.reasons as reason (reason)}
                {#if REASON_LABELS[reason]}
                  <span class="flex-none rounded bg-bg-elevated px-1.5 py-0.5 text-micro text-text-muted">
                    {REASON_LABELS[reason]}
                  </span>
                {/if}
              {/each}
            </div>
          </div>
        </button>
      {/snippet}

      {#each groups as group (group.id)}
        {#if group.items.length === 1 || expanded[group.id]}
          {#each group.items as item (item.id)}
            {@render feedRow(item)}
          {/each}
        {:else}
          {@const rep = group.items[0]}
          {@const kinds = groupKindCounts(group.items)}
          {@const anyUnread = group.items.some((entry) => !entry.read_at)}
          <button
            type="button"
            class="group flex w-full items-start gap-2.5 border-b border-border-subtle/60 px-4 py-2.5 text-left transition-colors {anyUnread
              ? 'bg-accent/[0.045] hover:bg-bg-hover'
              : 'hover:bg-bg-hover'}"
            onclick={() => openGroup(group)}
          >
            <span class="mt-2 flex h-2 w-2 flex-none items-center justify-center">
              {#if anyUnread}<span class="h-1.5 w-1.5 rounded-full bg-accent"></span>{/if}
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="flex-none font-mono text-micro text-text-muted">{rep.issue_key}</span>
                <span
                  class="min-w-0 flex-1 truncate text-body {anyUnread
                    ? 'font-medium text-text-primary'
                    : 'text-text-secondary'}"
                >{rep.summary}</span>
                <span class="flex-none text-micro text-text-muted" title={absTime(rep.occurred_at)}>
                  {relativeTime(rep.occurred_at)}
                </span>
              </div>
              <div class="mt-1 flex min-w-0 items-center gap-1.5 text-micro">
                <Icon name="chevron-right" size={12} class="text-text-muted" />
                {#if kinds.length === 1}
                  <span class="flex-none text-text-secondary">{EVENT_LABELS[rep.event_type]}</span>
                  <span class="flex-none rounded bg-bg-elevated px-1 py-0.5 text-micro font-medium text-text-secondary">
                    ×{group.items.length}
                  </span>
                {:else}
                  <!-- GDK-1058: a mixed run names each kind (×count when a
                       kind repeats), so the collapsed row says what happened,
                       not just how much. -->
                  {#each kinds as kind (kind.type)}
                    <span class="flex-none text-text-secondary">
                      {EVENT_LABELS[kind.type]}{kind.count > 1 ? ` ×${kind.count}` : ''}
                    </span>
                  {/each}
                {/if}
                <span class="min-w-2 flex-1"></span>
                {#each reasonsOf(group.items) as reason (reason)}
                  {#if REASON_LABELS[reason]}
                    <span class="flex-none rounded bg-bg-elevated px-1.5 py-0.5 text-micro text-text-muted">
                      {REASON_LABELS[reason]}
                    </span>
                  {/if}
                {/each}
              </div>
            </div>
          </button>
        {/if}
      {/each}
    {/if}
  </div>
</div>
