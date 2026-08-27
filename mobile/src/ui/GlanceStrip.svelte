<script lang="ts">
  import { errorMessage } from '../lib/api'
  import { t } from '../lib/i18n'
  import { feedKindLabel, glanceRows, relTime, type FeedItem } from '../lib/domain'
  import { app, markGlanceAllRead, markGlanceIssueRead, openIssue } from '../lib/store.svelte'

  /*
   * The glance strip (GDK-871): what moved while the phone was away, as the
   * Issues queue's first band — not a fourth tab (DESIGN.md §1 non-jobs).
   * Present only while something is unread (counts.all === 0 → this whole
   * section is absent, no empty box), scope-independent: the feed is a
   * person's, not a scope's. Rows never mark themselves read by being seen;
   * receipts land only when the POST answers, so the strip cannot vanish
   * before it was read.
   */
  let error = $state('')

  const unread = $derived(app.feed?.unread_counts.all ?? 0)
  const shown = $derived(unread > 0)
  const rows = $derived(glanceRows(app.feed?.items ?? []))

  function open(item: FeedItem): void {
    openIssue(item.issue_key)
    // Settles behind the trip to Detail. The row is not removed before the
    // reply: on refusal it is still here, with the reason under the strip.
    void markGlanceIssueRead(item.issue_key).catch((err: unknown) => {
      error = errorMessage(err)
    })
  }

  async function markAll(): Promise<void> {
    error = ''
    try {
      await markGlanceAllRead()
    } catch (err) {
      error = errorMessage(err)
    }
  }
</script>

{#if shown}
  <section class="glance" data-testid="glance-strip">
    <div class="head">
      <span class="label">{t('feed.unreadCount', { n: unread })}</span>
      <button class="allread" onclick={markAll}>{t('feed.markAllRead')}</button>
    </div>
    {#each rows as item (item.event_id)}
      <button class="g-item" onclick={() => open(item)}>
        <span class="key">{item.issue_key}</span>
        <span class="what">{feedKindLabel(item.event_type)}</span>
        {#if item.actor_name}
          <span class="who">{item.actor_name}</span>
        {/if}
        {#if item.occurred_at}
          <span class="when">{relTime(item.occurred_at, app.now)}</span>
        {/if}
      </button>
    {/each}
    {#if error}
      <p class="err">{error}</p>
    {/if}
  </section>
{/if}

<style>
  .glance {
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 16px;
    min-width: 0;
  }
  .label {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .allread {
    flex: none;
    padding: 0;
    color: var(--color-accent-text);
    font-size: var(--text-micro);
  }
  .g-item {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 6px;
    text-align: left;
    padding: 0 16px;
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    overflow: hidden;
    white-space: nowrap;
  }
  .g-item:active {
    background: var(--color-bg-hover);
  }
  .key {
    flex: none;
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }
  .what {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--color-text-primary);
  }
  .who {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .when {
    flex: none;
    margin-left: auto;
    font-variant-numeric: tabular-nums;
  }
  .err {
    margin: 0;
    padding: 4px 16px 8px;
    font-size: var(--text-micro);
    color: var(--color-status-stale);
  }
</style>
