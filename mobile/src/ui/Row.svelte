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
   * a button that quietly re-filters the list under the reader.
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
      {#if stale}
        <span class="age" data-age-band={band} title={rowAgeTitle(issue)}
          >{t('list.staleDaysShort', { n: ageDays })}</span
        >
      {/if}
      <span class="when">{folioDate(issue.updated_at)}</span>
    </span>
    <span class="line2">
      <span class="key">{issue.issue_key}</span>
      {#each meta as m (m)}
        <span class="sep" aria-hidden="true">·</span>
        <span class="m">{m}</span>
      {/each}
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
    gap: 8px;
    min-width: 0;
  }
  .summary {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }
  .when {
    flex: none;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    font-variant-numeric: tabular-nums;
  }
  /* GDK-1336's rule, kept: band weight is text weight and amber, no box.
     A bordered chip on every stale row reads as a column of badges rather
     than a signal — and on a phone that column is a third of the line. */
  .age {
    flex: none;
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }
  .age[data-age-band='mid'] {
    color: var(--color-status-stale);
    opacity: 0.8;
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
  .m {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sep {
    flex: none;
  }
</style>
