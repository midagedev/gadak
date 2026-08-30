<script lang="ts">
  /*
   * One column (GDK-1175) — a group of `filters.groups`, stood on its end.
   *
   * The header is the list's own `GroupHeader`, unchanged: the board is a
   * second reading of one view, not a second app, and a header drawn twice is
   * a header that drifts. It already carries the label, the total and the
   * category tallies, which is every aggregate a column wants.
   *
   * No virtualization. A column scrolls, and that is the whole overflow
   * story — `VirtualRows` exists for ten thousand rows down one page, and a
   * board that needs it is a filter problem the board should not paper over.
   */
  import { t } from '../../lib/i18n'
  import type { IssueGroup } from '../../stores/filters.svelte'
  import type { TerminalSessionState } from '../../lib/terminal/strip'
  import GroupHeader from '../list/GroupHeader.svelte'
  import BoardCard from './BoardCard.svelte'

  let {
    group,
    showCategoryCounts = true,
    shellOf,
  }: {
    group: IssueGroup
    showCategoryCounts?: boolean
    /** The shell state for one issue, decided by the board (one poll, one pass). */
    shellOf: (key: string) => TerminalSessionState | null
  } = $props()
</script>

<section
  data-testid="board-column"
  data-board-column={group.key}
  class="flex h-full w-[288px] flex-none flex-col border-r border-border-subtle/70 last:border-r-0"
>
  <div class="flex-none"><GroupHeader {group} {showCategoryCounts} /></div>
  <div class="scroll-region flex min-h-0 flex-1 flex-col gap-1.5 px-2 pt-1">
    {#each group.items as issue (issue.issue_key)}
      <BoardCard {issue} shell={shellOf(issue.issue_key)} />
    {:else}
      <!-- An empty column is still a place a card can land, so it says so
           rather than collapsing into a gap between two other columns. -->
      <p class="px-1 pt-2 text-micro text-text-muted">{t('board.columnEmpty')}</p>
    {/each}
  </div>
</section>
