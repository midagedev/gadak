<script lang="ts">
  import DeskRow from './DeskRow.svelte'
  import { t } from '../lib/i18n'
  import { sprintCounts, sprintDaysLeft } from '../lib/sprint'
  import type { IssueLite, SprintRow } from '../lib/types'

  /*
   * The current sprint, as one line (GDK-1867).
   *
   * The desk has said these facts since 0.22 on the board's sprint strip
   * (web/src/components/board/SprintStrip.svelte). This is the same reading
   * on a 402px screen: which sprint, how much of it is done, how long is
   * left, and what it is for. Not a new design — no bar, no burn-up, no
   * points. The bar is what the desk gives its spare width to and this line
   * has none; the count beside it was always the reading, and the bar was
   * the picture of the count.
   *
   * Read-only, and the only thing it does is change scope. Eighteen of the
   * collected phone complaints are board/sprint *reads* — "the board exists
   * but there is no way to get to it" — and none of them asked for
   * drag-and-drop on a phone.
   *
   * Absent, never empty: no active sprint (kanban, or between sprints), a
   * serve older than `issues/sprints/`, an origin with no cycles — the
   * parent passes null and nothing renders. A stale snapshot shows the
   * counts of the snapshot it is showing; the offline banner above already
   * says the data is cached.
   */
  let {
    sprint,
    issues,
    now,
    current,
    onpick,
  }: {
    /** The one active sprint, or null — then this draws nothing. */
    sprint: SprintRow | null
    /** The held snapshot: the counts are taken from it, not from the wire's
     *  server-side totals, so the line and the list under it are the same
     *  arithmetic over the same rows (lib/sprint.ts). */
    issues: IssueLite[]
    /** The app clock, so days-left ticks with every other relative time. */
    now: Date
    /** Whether the list is already scoped to this sprint. */
    current: boolean
    onpick: () => void
  } = $props()

  const counts = $derived(sprint ? sprintCounts(issues, sprint.id) : null)
  const daysLeft = $derived(sprint ? sprintDaysLeft(sprint.end_at, now) : null)

  /* A sentence, not a "D-6" — the desk's own five keys and the desk's own
   * reason: the abbreviation is a Korean office idiom that neither English
   * nor Japanese reads. */
  const days = $derived.by(() => {
    const n = daysLeft
    if (n == null) return ''
    if (n === 0) return t('board.sprintEndsToday')
    if (n === 1) return t('board.sprintOneDayLeft')
    if (n > 1) return t('board.sprintDaysLeft', { n })
    if (n === -1) return t('board.sprintEndedYesterday')
    return t('board.sprintEndedAgo', { n: -n })
  })
</script>

{#if sprint && counts}
  <button
    class="sprint"
    data-testid="sprint-line"
    aria-current={current ? 'true' : undefined}
    onclick={onpick}
  >
    <span class="top">
      <span class="name">{sprint.name}</span>
      {#if counts.total > 0}
        <span class="count" aria-label={t('board.sprintProgress', { done: counts.done, total: counts.total })}
          >{counts.done} / {counts.total} · {counts.pct}%</span
        >
      {/if}
      {#if days}
        <span class="days">{days}</span>
      {/if}
    </span>
    {#if sprint.goal}
      <!-- The goal has been in the mirror since sprints became rows and no
           phone surface read it. Up to two lines (GDK-1977); the sprint
           scope is one tap away for the rows it describes. -->
      <span class="goal">{sprint.goal}</span>
    {/if}
  </button>
  <!-- GDK-1874: the board itself. Eighteen of the collected phone
       complaints are board/sprint reads and none asked for drag-and-drop on
       a phone — but "there is no board here" is something a person finds out
       by looking for one. The line above is the read; this row is where the
       moving happens, said next to it. Its own band, under the line's
       border, because it is a different claim. -->
  <div class="desk">
    <DeskRow label={t('board.label')} testid="desk-row-board" />
  </div>
{/if}

<style>
  /* Three rows at most on a 402px phone: the facts line, and the goal under
     it in up to two. The facts line still clamps to one — it is a row of
     short fields and a second line of it would be a layout accident, not a
     sentence. The goal gets two because it IS a sentence, and one line was
     not enough to say it in Japanese (GDK-1977).

     The rule this replaces said both clamp to one, because chrome that
     grows pushes the rows it describes off the screen. That is still the
     reason the ceiling is two and not "as many as it takes" — but a goal cut
     mid-word is chrome that costs its space and returns nothing, which is
     the worse end of the same trade. */
  .sprint {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 1px;
    width: 100%;
    /* A sprint without a goal is one row (~30pt); the tap target still owes
       44pt — the same token KeyBar and the sheet rows read. */
    min-height: var(--spacing-control);
    padding: 4px 16px;
    text-align: left;
    border-bottom: 1px solid var(--color-border-subtle);
    min-width: 0;
  }
  .sprint:active {
    background: var(--color-bg-hover);
  }
  /* The desk row's own 8px plus 8 here puts its label on the 16px column
     every band above the queue shares — the sheet reaches the same number
     the same way (.list 8 + .row 8). The border is the band grammar the
     line above already wears, so the two read as two rows and not as one
     row with a tail. */
  .desk {
    padding: 0 8px;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .top {
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .name {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-body);
    color: var(--color-text-primary);
  }
  .sprint[aria-current='true'] .name {
    font-weight: 600;
    color: var(--color-accent-text);
  }
  .count {
    flex: none;
    font-variant-numeric: tabular-nums;
  }
  .days {
    flex: none;
    margin-left: auto;
    white-space: nowrap;
  }
  /* Two lines, then ellipsis (GDK-1977). One line here is 370px at 402px of
     phone, which is 30 full-width characters. The recording fixture's goal
     is the same sentence in three languages — 48 Latin characters in
     English, 32 in Korean of which 22 are full-width, 34 in Japanese of
     which 29 are — so the first two fit on one line and the Japanese one
     measured 377px against that 370px box and lost its last two characters
     to the ellipsis, which is what the ja clip's frame showed.

     The width is the font's, not the language's, and that is the part worth
     remembering: `web/src/app.css` hangs the Japanese and Korean stacks off
     `:lang(…)`, which is set from the chosen locale, and those stacks draw
     every full-width glyph at a full em. The same 34 characters under the
     Latin stack's CJK fallback measure 333px and fit — so a measurement of
     this taken with the UI in English says there is nothing wrong.

     Japanese and Korean break between characters on their own, so the second
     line needs no break hint; `overflow-wrap` is deliberately absent so an
     English goal still breaks at its spaces.

     -webkit-line-clamp is the form that works in WKWebView and in iOS
     Safari, which is every surface this file is painted on; the unprefixed
     `line-clamp` rides along for the browsers that have moved. */
  .goal {
    min-width: 0;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
</style>
