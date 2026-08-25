<script lang="ts">
  import type { IssueLite } from '../lib/types'
  import { relTime, spineToken } from '../lib/domain'
  import { app, openIssue } from '../lib/store.svelte'

  // One ledger row (DESIGN.md §3.4): the ink spine carries the status, the
  // summary is the sentence, the meta line is one truncating breath.
  let { issue, showAssignee = false }: { issue: IssueLite; showAssignee?: boolean } = $props()

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
      <span class="when">{relTime(issue.updated_at, app.now)}</span>
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
    background: var(--color-status-new);
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
