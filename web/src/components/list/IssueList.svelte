<script lang="ts">
  /*
   * Virtual issue list ([explore]) — in-house.
   *  - Fixed 42px row height (header/issue same) → uniform virtualization
   *    (DOM nodes = viewport rows).
   *  - Group headers float sticky + next header push effect.
   *  - Keys live in App.svelte (one window handler for the whole triage flow);
   *    this file owns the cursor's scroll follow and paints the cursor row.
   *  Perf: scroll only computes top(translate); no full recompute.
   */
  import { onMount, untrack } from 'svelte'
  import { startupViewTick } from '../../lib/startup-view'
  import { filters, type IssueGroup } from '../../stores/filters.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { triage } from '../../stores/triage.svelte'
  import { browse } from '../../lib/browse.svelte'
  import type { IssueLite } from '../../lib/types'
  import IssueRow from './IssueRow.svelte'
  import GroupHeader from './GroupHeader.svelte'

  const ROW_H = 42
  const OVERSCAN = 8

  type RowItem =
    | { type: 'header'; group: IssueGroup }
    | { type: 'issue'; issue: IssueLite }

  const grouped = $derived(filters.display.group_by !== 'none')

  // Flatten visual order: (header) + group items. group_by=none → one group, no header.
  const rows = $derived.by(() => {
    const out: RowItem[] = []
    for (const g of filters.groups) {
      if (grouped) out.push({ type: 'header', group: g })
      for (const it of g.items) out.push({ type: 'issue', issue: it })
    }
    return out
  })

  // Row-index map for the cursor's scroll follow (visual order incl. headers).
  const issueRowIndex = $derived.by(() => {
    const m = new Map<string, number>()
    rows.forEach((r, i) => {
      if (r.type === 'issue') m.set(r.issue.issue_key, i)
    })
    return m
  })

  let scrollTop = $state(0)
  let viewportH = $state(0)
  let scroller = $state<HTMLDivElement | null>(null)
  /** Last viewKey we already handled. A re-run from scroller bind must not reset. */
  let lastHandledViewKey = ''
  const cursorKey = $derived(triage.cursorKey)

  const total = $derived(rows.length * ROW_H)
  const start = $derived(Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN))
  const end = $derived(
    Math.min(rows.length, Math.ceil((scrollTop + viewportH) / ROW_H) + OVERSCAN),
  )
  const slice = $derived(rows.slice(start, end))

  // ── Floating group header (top group + push effect) ──
  const firstRow = $derived(Math.floor(scrollTop / ROW_H))
  const floatingGroup = $derived.by(() => {
    if (!grouped || rows.length === 0) return null
    for (let i = Math.min(firstRow, rows.length - 1); i >= 0; i--) {
      const r = rows[i]
      if (r.type === 'header') return { group: r.group, headerIndex: i }
    }
    return null
  })
  // Push up as the next header approaches.
  const floatingOffset = $derived.by(() => {
    if (!floatingGroup) return 0
    for (let i = floatingGroup.headerIndex + 1; i < rows.length; i++) {
      if (rows[i].type === 'header') {
        const nextTop = i * ROW_H
        const delta = nextTop - scrollTop
        return delta < ROW_H ? delta - ROW_H : 0
      }
    }
    return 0
  })

  function onScroll(e: Event) {
    scrollTop = (e.currentTarget as HTMLDivElement).scrollTop
  }

  function scrollToRow(rowIndex: number) {
    if (!scroller) return
    const top = rowIndex * ROW_H
    const bottom = top + ROW_H
    // In group mode, leave room for the sticky header (1 row)
    const pad = grouped ? ROW_H : 0
    if (top - pad < scroller.scrollTop) scroller.scrollTop = top - pad
    else if (bottom > scroller.scrollTop + viewportH) scroller.scrollTop = bottom - viewportH
  }

  // Follow the cursor: keep the row the keys moved onto inside the viewport.
  //  untrack the row map so a data delta cannot yank the scroll back on its own.
  $effect(() => {
    const key = cursorKey
    if (!key) return
    untrack(() => {
      const rowIdx = issueRowIndex.get(key)
      if (rowIdx != null) scrollToRow(rowIdx)
    })
  })

  // A breakdown chip asked for a group: put that section header at the top.
  //  Only the request itself is tracked — the row lookup runs untracked so a
  //  data delta cannot re-scroll on its own (same shape as the follow above).
  $effect(() => {
    const req = filters.revealRequest
    if (!req) return
    void req.nonce
    untrack(() => {
      const idx = rows.findIndex((r) => r.type === 'header' && r.group.key === req.key)
      if (idx >= 0 && scroller) scroller.scrollTop = idx * ROW_H
    })
  })

  // Real view (filter/group/sort) changes scroll to top (ignore data deltas).
  // The boot-time first commit is not a user view change: wait until App has
  // applied the startup view, then mark keys ready (replay held j/k/x) instead
  // of wiping a cursor that never belonged to this view.
  $effect(() => {
    const vk = filters.viewKey
    const applied = triage.startupViewApplied
    // scroller bind is not a view change — do not subscribe.
    untrack(() => {
      if (scroller) scroller.scrollTop = 0
    })
    scrollTop = 0
    // untrack keysReady: markKeysReady flips it; tracking would re-run and
    // resetCursor the keys we just replayed.
    const tick = startupViewTick(applied, untrack(() => triage.keysReady), vk, lastHandledViewKey)
    if (tick === 'wait' || tick === 'same-view') return
    if (tick === 'mark-ready') {
      triage.markKeysReady()
      lastHandledViewKey = vk
      return
    }
    lastHandledViewKey = vk
    triage.resetCursor()
  })

  // Tell the shell the triage keys have a list to act on. Feed / onboarding /
  // empty states render instead of this component, and there they do nothing.
  onMount(() => {
    triage.listActive = true
    return () => {
      triage.listActive = false
      triage.resetCursor()
    }
  })
</script>

<div class="relative h-full">
  <!-- Sticky group header (floating) -->
  {#if floatingGroup}
    <div
      class="pointer-events-none absolute inset-x-0 top-0 z-10"
      style:transform="translateY({floatingOffset}px)"
    >
      <div class="pointer-events-auto border-b border-border-subtle shadow-sm shadow-black/20">
        <GroupHeader
          group={floatingGroup.group}
          floating
          showCategoryCounts={filters.display.group_by !== 'status_category'}
        />
      </div>
    </div>
  {/if}

  <div
    bind:this={scroller}
    bind:clientHeight={viewportH}
    onscroll={onScroll}
    data-testid="issue-list-scroller"
    class="h-full overflow-y-auto"
  >
    <!-- The margin only exists while the browse re-entry pill floats over this
         corner: without it the pill sits on the last row's checkbox. Off
         desktop, tabs never exist and the margin never appears. -->
    <div
      class="relative"
      style:height="{total}px"
      style:margin-bottom={browse.pillVisible ? '56px' : ''}
    >
      {#each slice as row, i (start + i + (row.type === 'issue' ? row.issue.issue_key : 'h' + row.group.key))}
        <div class="absolute inset-x-0" style:top="{(start + i) * ROW_H}px" style:height="{ROW_H}px">
          {#if row.type === 'header'}
            <GroupHeader
              group={row.group}
              showCategoryCounts={filters.display.group_by !== 'status_category'}
            />
          {:else}
            <IssueRow
              issue={row.issue}
              active={selection.selectedKey === row.issue.issue_key}
              cursor={cursorKey === row.issue.issue_key}
            />
          {/if}
        </div>
      {/each}
    </div>
  </div>
</div>
