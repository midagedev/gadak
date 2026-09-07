<script lang="ts">
  import type { IssueLite } from '../lib/types'
  import { folioDate, rowAgeBand, rowAgeDays, rowAgeTitle, rowIsStale, spineToken } from '../lib/domain'
  import { t } from '../lib/i18n'
  import { openIssue } from '../lib/store.svelte'

  // One ledger row (DESIGN.md §3.4): the ink spine carries the status, the
  // summary is the sentence, the meta line is one truncating breath.
  let { issue, showAssignee = false }: { issue: IssueLite; showAssignee?: boolean } = $props()

  /*
   * Work-item age (GDK-1495 ②) — the desk's clock, on the desk's rules
   * (lib/domain rowAge → web view-config workAge): counted from the moment
   * work started when the mirror knows it, from the current status otherwise.
   * Present only past the threshold in force, so on a healthy queue the
   * column is empty; weight follows magnitude, because a maximum-emphasis
   * mark on every row warns about nothing.
   *
   * Data, not a control (GDK-906): a span with the rule in its title, never
   * a button that quietly re-filters the list under the reader — and it
   * rides the meta line beside the key, not the title baseline, so the
   * summary keeps its width (vision FIX 2026-09-07).
   */
  const stale = $derived(rowIsStale(issue))
  const ageDays = $derived(rowAgeDays(issue))
  const band = $derived(rowAgeBand(issue))

  const meta = $derived(
    [
      showAssignee && issue.assignee ? issue.assignee : null,
      issue.comment_count > 0
        ? `${issue.comment_count} comment${issue.comment_count === 1 ? '' : 's'}`
        : null,
    ].filter(Boolean) as string[],
  )
</script>

<button class="row" onclick={() => openIssue(issue.issue_key)}>
  <span class="spine spine-{spineToken(issue)}" aria-hidden="true"></span>
  <span class="text">
    <span class="line1">
      <span class="summary">{issue.summary}</span>
    </span>
    <span class="line2">
      <span class="key">{issue.issue_key}</span>
      {#each meta as m (m)}
        <span class="sep" aria-hidden="true">·</span>
        <span class="m">{m}</span>
      {/each}
      {#if stale}
        <span class="sep" aria-hidden="true">·</span>
        <span class="age" data-age-band={band} title={rowAgeTitle(issue)}
          >{t('list.staleDaysShort', { n: ageDays })}</span
        >
      {/if}
      <span class="when">{folioDate(issue.updated_at)}</span>
    </span>
  </span>
</button>

<style>
  .row {
    position: relative;
    display: flex;
    width: 100%;
    min-height: var(--spacing-row);
    align-items: center;
    text-align: left;
    padding: 8px 16px;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .row:active {
    background: var(--color-bg-hover);
  }
  .spine {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
  }
  .spine-new {
    background: var(--color-spine-new);
  }
  .spine-inprogress {
    background: var(--color-status-inprogress);
  }
  .spine-done {
    background: var(--color-status-done);
  }
  .spine-reopen {
    background: var(--color-status-reopen);
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1 1 auto;
  }
  .line1 {
    display: flex;
    align-items: baseline;
    min-width: 0;
  }
  /* Two lines, then stop (GDK-1543 step 3). Reclaiming the date's 41px took
     the first screen from 11 cut titles to 10 — the fixture's summaries are
     simply longer than one 370px line, so width alone could not pay for the
     readability. A second line pays for it: 0/9 cut on the first screen,
     0/42 in the list. The row keeps `min-height: var(--spacing-row)`, so a
     one-line row does not shrink and the list's rhythm survives. */
  .summary {
    flex: 1 1 auto;
    min-width: 0;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: normal;
    color: var(--color-text-primary);
  }
  /* GDK-1543, 2026-09-07: the date left line 1. The A4 round measured what
     the age chip cost the title (23px) and named what actually held the rest
     — the date column, which took a fixed slice of the title's line and cut
     11 of the 12 first-screen summaries. The date is meta like the key and
     the count are: it belongs on the line that truncates by design, and the
     summary gets the row's full width back (329px → 370px measured on the
     demo fixture at 402px; the first screen went 11/12 cut → 10/12 on width
     alone). Right-aligned so the dates still read as a column, `flex: none`
     so the meta items yield first. */
  .when {
    flex: none;
    margin-left: auto;
    padding-left: 6px;
    font-variant-numeric: tabular-nums;
  }
  /* GDK-1336's rule, kept: band weight is text weight and amber, no box.
     A bordered chip on every stale row reads as a column of badges rather
     than a signal — and on a phone that column is a third of the line.

     Placed on the meta line, not the title baseline (vision FIX 2026-09-07).
     The age *is* meta — it belongs where the key and the comment count
     already are, on the line that truncates by design. Measured on the demo
     fixture at 402px (a4-captures logs it every run): the title went 306px →
     329px and the list's truncated summaries 39/42 → 37/42. The 23px is the
     chip and its gap, not the ~150px the verdict estimated — what actually
     held the rest of that width was the date, and GDK-1543 moved it here
     too (see `.when`).

     Three bands, drawn as three (same FIX): mid was the stale amber at 0.8
     and photographed as the same dark brown as loud, so the ladder read as
     two. Only loud carries colour now; mid is one step darker than the meta
     grey it sits in, which is weight without hue. The thresholds are
     untouched — rowAgeBand still reads the desk's own workAge ratios. */
  .age {
    flex: none;
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }
  .age[data-age-band='mid'] {
    color: var(--color-text-secondary);
  }
  .age[data-age-band='loud'] {
    color: var(--color-status-stale);
    font-weight: 500;
  }
  .line2 {
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    overflow: hidden;
    white-space: nowrap;
  }
  .key {
    font-family: var(--font-mono);
    flex: none;
  }
  /* The meta line's one truncating breath: the key never shortens, the date
     and the age hold their width, and these yield first (GDK-1543). */
  .m {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .sep {
    flex: none;
  }
</style>
