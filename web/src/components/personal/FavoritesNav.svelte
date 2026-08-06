<script lang="ts">
  /* Favorites and recent issues shown separately. Favorite rows are drag-reorder targets. */
  import { t, relativeSeenLabel } from '../../lib/i18n'
  import { onMount } from 'svelte'
  import type { IssueLite } from '../../lib/types'
  import { absTime } from '../../lib/format'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { me, type RecentVisit } from '../../stores/me.svelte'
  import Icon from '../ui/Icon.svelte'

  interface NavItem {
    issue: IssueLite
    visit: RecentVisit | null
  }

  interface DragCandidate {
    key: string
    pointerId: number
    startX: number
    startY: number
    moved: boolean
  }

  let now = $state(Date.now())
  let dragCandidate = $state<DragCandidate | null>(null)
  let draggingKey = $state<string | null>(null)
  let dragOverKey = $state<string | null>(null)
  let suppressClickKey = $state<string | null>(null)

  onMount(() => {
    const timer = window.setInterval(() => (now = Date.now()), 30_000)
    return () => window.clearInterval(timer)
  })

  const favoriteItems = $derived.by(() => {
    const visitByKey = new Map(me.recentIssues.map((visit) => [visit.key, visit]))
    const items: NavItem[] = []
    for (const key of me.favorites) {
      const issue = issues.pool.get(key)
      if (issue) items.push({ issue, visit: visitByKey.get(key) ?? null })
    }
    return items
  })

  const recentItems = $derived.by(() => {
    const items: NavItem[] = []
    for (const visit of me.recentIssues) {
      if (me.favorites.has(visit.key)) continue
      const issue = issues.pool.get(visit.key)
      if (issue) items.push({ issue, visit })
      if (items.length === 12) break
    }
    return items
  })

  function viewedLabel(viewedAt: string | null | undefined): string {
    now
    if (!viewedAt) return t('personal.recentHistory')
    return relativeSeenLabel(viewedAt)
  }

  function selectIssue(event: MouseEvent, key: string): void {
    if (suppressClickKey === key) {
      event.preventDefault()
      event.stopPropagation()
      suppressClickKey = null
      return
    }
    selection.select(key)
  }

  function beginFavoriteDrag(event: PointerEvent, key: string): void {
    if (event.button !== 0) return
    if ((event.target as HTMLElement).closest('[data-favorite-action]')) return
    dragCandidate = {
      key,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      moved: false,
    }
    const row = event.currentTarget as HTMLElement
    row.setPointerCapture(event.pointerId)
  }

  function moveFavoriteDrag(event: PointerEvent): void {
    const candidate = dragCandidate
    if (!candidate || candidate.pointerId !== event.pointerId) return
    if (!candidate.moved) {
      const distance = Math.hypot(event.clientX - candidate.startX, event.clientY - candidate.startY)
      if (distance < 6) return
      candidate.moved = true
      draggingKey = candidate.key
    }

    event.preventDefault()
    const target = document
      .elementFromPoint(event.clientX, event.clientY)
      ?.closest<HTMLElement>('[data-favorite-key]')
    const targetKey = target?.dataset.favoriteKey
    if (!targetKey || targetKey === draggingKey || targetKey === dragOverKey) return
    dragOverKey = targetKey
    me.reorderFavorite(candidate.key, targetKey)
  }

  function endFavoriteDrag(event: PointerEvent): void {
    const candidate = dragCandidate
    const row = event.currentTarget as HTMLElement
    if (row.hasPointerCapture(event.pointerId)) row.releasePointerCapture(event.pointerId)
    if (candidate?.moved) {
      suppressClickKey = candidate.key
      window.setTimeout(() => {
        if (suppressClickKey === candidate.key) suppressClickKey = null
      }, 0)
    }
    dragCandidate = null
    draggingKey = null
    dragOverKey = null
  }
</script>

{#if favoriteItems.length}
  <div class="mb-3">
    <div class="px-3 py-1 text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('personal.favorites')}
    </div>
    {#each favoriteItems as item (item.issue.issue_key)}
      <div
        data-testid={`favorite-issue-${item.issue.issue_key}`}
        data-favorite-key={item.issue.issue_key}
        class="group relative flex min-h-[48px] touch-none cursor-grab items-center rounded-md px-2.5 py-1.5 transition-colors active:cursor-grabbing {selection.selectedKey ===
        item.issue.issue_key
          ? 'bg-bg-active'
          : 'hover:bg-bg-hover'} {dragOverKey === item.issue.issue_key
          ? 'shadow-[inset_0_2px_0_var(--color-accent)]'
          : ''} {draggingKey === item.issue.issue_key ? 'opacity-50' : ''}"
        onpointerdown={(event) => beginFavoriteDrag(event, item.issue.issue_key)}
        onpointermove={moveFavoriteDrag}
        onpointerup={endFavoriteDrag}
        onpointercancel={endFavoriteDrag}
        role="group"
      >
        <button
          type="button"
          class="w-full min-w-0 text-left"
          onclick={(event) => selectIssue(event, item.issue.issue_key)}
          title={`${item.issue.issue_key} · ${item.issue.summary}`}
        >
          <span class="flex min-w-0 items-center gap-2 pr-7">
            <span class="w-[70px] flex-none truncate font-mono text-micro text-text-muted">
              {item.issue.issue_key}
            </span>
            <span
              class="min-w-0 flex-1 truncate text-right text-micro text-text-muted"
              title={item.visit?.viewed_at ? absTime(item.visit.viewed_at) : undefined}
            >
              {viewedLabel(item.visit?.viewed_at)}
            </span>
          </span>
          <span
            class="mt-0.5 block truncate text-micro font-medium leading-[1.35] {selection.selectedKey ===
            item.issue.issue_key
              ? 'text-text-primary'
              : 'text-text-secondary group-hover:text-text-primary'}"
          >
            {item.issue.summary}
          </span>
        </button>
        <button
          type="button"
          data-favorite-action
          class="absolute right-2 top-1 flex h-control-sm w-control-sm items-center justify-center rounded-md text-status-stale transition-colors hover:bg-bg-hover"
          onclick={() => void me.toggleFavorite(item.issue.issue_key)}
          aria-pressed="true"
          aria-label={t('personal.unfavoriteAria', { key: item.issue.issue_key })}
          title={t('common.unfavorite')}
        >
          <Icon name="star" size={13} filled />
        </button>
      </div>
    {/each}
  </div>
{/if}

{#if recentItems.length}
  <div class="mb-3">
    <div class="px-3 py-1 text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('personal.recent')}
    </div>
    {#each recentItems as item (item.issue.issue_key)}
      <div
        data-testid={`recent-issue-${item.issue.issue_key}`}
        class="group relative flex min-h-[48px] items-center rounded-md px-2.5 py-1.5 transition-colors {selection.selectedKey ===
        item.issue.issue_key
          ? 'bg-bg-active'
          : 'hover:bg-bg-hover'}"
      >
        <button
          type="button"
          class="w-full min-w-0 text-left"
          onclick={(event) => selectIssue(event, item.issue.issue_key)}
          title={`${item.issue.issue_key} · ${item.issue.summary}`}
        >
          <span class="flex min-w-0 items-center gap-2">
            <span class="w-[70px] flex-none truncate font-mono text-micro text-text-muted">
              {item.issue.issue_key}
            </span>
            <span
              class="min-w-0 flex-1 truncate text-right text-micro text-text-muted"
              title={item.visit?.viewed_at ? absTime(item.visit.viewed_at) : undefined}
            >
              {viewedLabel(item.visit?.viewed_at)}
            </span>
          </span>
          <span
            class="mt-0.5 block truncate text-micro font-medium leading-[1.35] {selection.selectedKey ===
            item.issue.issue_key
              ? 'text-text-primary'
              : 'text-text-secondary group-hover:text-text-primary'}"
          >
            {item.issue.summary}
          </span>
        </button>
        <button
          type="button"
          class="pointer-events-none absolute right-2 top-1 flex h-control-sm w-control-sm items-center justify-center rounded-md bg-bg-elevated text-text-muted opacity-0 shadow-sm shadow-black/25 transition-opacity hover:bg-bg-hover hover:text-text-primary group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
          onclick={() => void me.toggleFavorite(item.issue.issue_key)}
          aria-pressed="false"
          aria-label={t('personal.favoriteAria', { key: item.issue.issue_key })}
          title={t('common.favorite')}
        >
          <Icon name="star" size={13} />
        </button>
      </div>
    {/each}
  </div>
{/if}
