<script lang="ts">
  /*
   * Virtual issue list ([explore]) — in-house.
   *  - Row heights come from the CSS tokens via rowMetrics() (headers and
   *    plain rows --spacing-row, an issue row with a match/excerpt line
   *    --spacing-row-excerpt); the window is offset-based, so a token change
   *    or a taller row mode repaints true without a second height constant
   *    to drift (GDK-842). DOM nodes = viewport rows.
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
  import {
    type RowMode,
    onRowMetricsInvalidated,
    rowMetrics,
    rowOffsets,
    rowIndexAt,
    rowWindow,
  } from '../../lib/row-metrics'
  import IssueRow, { HEADER_ROW_ISSUE } from './IssueRow.svelte'
  import GroupHeader from './GroupHeader.svelte'
  import { defaultColumns } from '../../lib/view-config'
  import { TRAIL_BREAK_PRIORITY, trailBreakCss, syncTrailBreakStyle } from './row-column-thresholds'

  const OVERSCAN = 8

  type RowItem =
    | { type: 'header'; group: IssueGroup }
    // gk: the group the row sits in — only set when grouped. The actor axis is
    // multi-membership (one issue in several buckets), so the each-key needs
    // group+issue to stay unique; every other axis simply carries it unused.
    // mode: the paint mode IssueRow will use for this row — 'row' is the
    // dense 42px paint, 'row-excerpt' adds the match line at 59px. This list
    // never passes a match line today, so every row is 'row'; the field keeps
    // the window honest the day one does (GDK-842 closes the 42/59 drift).
    | { type: 'issue'; issue: IssueLite; gk?: string; mode: RowMode }

  const grouped = $derived(filters.display.group_by !== 'none')

  // Token-sourced heights, re-read when a user dims override lands
  // (applyUserTokens → invalidateRowMetrics fires the subscription below).
  let metrics = $state(rowMetrics())

  // Flatten visual order: (header) + group items. group_by=none → one group, no header.
  const rows = $derived.by(() => {
    const out: RowItem[] = []
    for (const g of filters.groups) {
      if (grouped) out.push({ type: 'header', group: g })
      for (const it of g.items)
        out.push(
          grouped
            ? { type: 'issue', issue: it, gk: g.key, mode: 'row' }
            : { type: 'issue', issue: it, mode: 'row' },
        )
    }
    return out
  })

  // Per-row height: headers and dense issue rows share --spacing-row; an
  // issue row carrying a match/excerpt line is --spacing-row-excerpt
  // (DocsView's rowHeight rule, mirrored).
  function rowHeightOf(r: RowItem): number {
    if (r.type === 'issue' && r.mode === 'row-excerpt') return metrics.rowExcerpt
    return metrics.row
  }

  // Row-index map for the cursor's scroll follow (visual order incl. headers).
  // First occurrence wins: under multi-membership grouping the same issue is
  // on screen more than once, and the cursor should land on the first.
  const issueRowIndex = $derived.by(() => {
    const m = new Map<string, number>()
    rows.forEach((r, i) => {
      if (r.type === 'issue' && !m.has(r.issue.issue_key)) m.set(r.issue.issue_key, i)
    })
    return m
  })

  // ── GDK-1087: the column header row ──
  // It appears only once the view has left the default column set. The seven
  // defaults are an avatar, a date and label chips — self-evident, and a
  // header over them would change the look of every list nobody customised.
  // The audit's unreadable columns ("⧖ 2", "1w", "NMS-157") are all on a
  // customised list, and when the header comes it labels every column on the
  // row, defaults included: a strip that labels half its columns is worse
  // than one that labels none.
  const DEFAULT_COLS = $derived(new Set<string>(defaultColumns()))
  const showColumnHeader = $derived(filters.display.columns.some((c) => !DEFAULT_COLS.has(c)))
  // The header sits outside the scroller, so it does not compete with the
  // floating group header for the top of the list. That costs one number: the
  // scrollbar gutter the scroller takes out of the rows' width and the header
  // would otherwise keep. Measured on the scroller itself (0 under macOS
  // overlay scrollbars, ~15px on Linux/Windows), never assumed.
  let gutter = $state(0)

  let scrollTop = $state(0)
  let viewportH = $state(0)
  let scroller = $state<HTMLDivElement | null>(null)
  /** Last viewKey we already handled. A re-run from scroller bind must not reset. */
  let lastHandledViewKey = ''
  const cursorKey = $derived(triage.cursorKey)

  // Offset sheet: offsets[i] is row i's top, offsets[n] the total height.
  // The window/anchor math mirrors VirtualRows.svelte (row-metrics owns it).
  const heights = $derived(rows.map(rowHeightOf))
  const offsets = $derived(rowOffsets(heights))
  const total = $derived(offsets[rows.length])
  const win = $derived(rowWindow(offsets, scrollTop, viewportH, OVERSCAN))
  const slice = $derived(rows.slice(win.start, win.end))

  // ── Floating group header (top group + push effect) ──
  const firstRow = $derived(rowIndexAt(offsets, scrollTop))
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
        const nextTop = offsets[i]
        const delta = nextTop - scrollTop
        return delta < metrics.row ? delta - metrics.row : 0
      }
    }
    return 0
  })

  function onScroll(e: Event) {
    scrollTop = (e.currentTarget as HTMLDivElement).scrollTop
  }

  function scrollToRow(rowIndex: number) {
    if (!scroller) return
    const top = offsets[rowIndex]
    const bottom = top + heights[rowIndex]
    // In group mode, leave room for the sticky header (1 row)
    const pad = grouped ? metrics.row : 0
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
      if (idx >= 0 && scroller) scroller.scrollTop = offsets[idx]
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

  // ── GDK-1077: the option-column break ladder, regenerated for the enabled
  // set ── A row-width hide threshold is a function of WHICH option columns
  // are on; the static full-catalog table GDK-1049 left in app.css made the
  // six rungs above the 1360 row cap unpaintable even enabled alone. Pure
  // constant math (row-column-thresholds.ts — no layout reads here), synced
  // into one <style> in <head>; the browser's container query does the
  // firing. This list is the only renderer of rows that wear the
  // trail-break classes, so the element rides its lifetime: gone on unmount.
  $effect(() => {
    const cols = filters.columnSet
    const epicPaints = filters.display.group_by !== 'epic'
    syncTrailBreakStyle(
      trailBreakCss(
        TRAIL_BREAK_PRIORITY.filter((col) => (col === 'epic' ? epicPaints : cols.has(col))),
      ),
    )
  })

  // Scrollbar gutter, measured. A content-box observer fires when a scrollbar
  // appears or leaves as well as on resize, which is exactly when the header's
  // right edge would stop agreeing with the rows'.
  $effect(() => {
    const el = scroller
    if (!el) return
    const measure = () => {
      gutter = el.offsetWidth - el.clientWidth
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  })

  // Tell the shell the triage keys have a list to act on. Feed / onboarding /
  // empty states render instead of this component, and there they do nothing.
  // The metrics subscription re-snapshots the token-sourced heights when a
  // user dims override lands at runtime (GDK-842) — the offsets deriveds
  // cascade and the window re-renders at the new geometry.
  onMount(() => {
    triage.listActive = true
    const offMetrics = onRowMetricsInvalidated(() => {
      metrics = rowMetrics()
    })
    return () => {
      offMetrics()
      triage.listActive = false
      triage.resetCursor()
      syncTrailBreakStyle('')
    }
  })
</script>

<div class="flex h-full flex-col">
  <!-- Column header. Same component as a row, in header mode, so the slots it
       labels cannot drift from the slots below it (GDK-1087). -->
  {#if showColumnHeader}
    <div class="flex-none" style:padding-right="{gutter}px" data-testid="issue-column-header-host">
      <IssueRow issue={HEADER_ROW_ISSUE} header />
    </div>
  {/if}

  <div class="relative min-h-0 flex-1">
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
      {#each slice as row, i (row.type === 'issue' ? (row.gk !== undefined ? row.gk + '::' + row.issue.issue_key : row.issue.issue_key) : 'h' + row.group.key)}
        {@const idx = win.start + i}
        <div class="absolute inset-x-0" style:top="{offsets[idx]}px" style:height="{heights[idx]}px">
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
</div>
