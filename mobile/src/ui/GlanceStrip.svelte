<script lang="ts">
  import { errorMessage } from '../lib/api'
  import { t } from '../lib/i18n'
  import { feedDetail, feedKindLabel, glanceRows, relTime, type FeedItem } from '../lib/domain'
  import { app, markGlanceAllRead, markGlanceIssueRead, openIssue } from '../lib/store.svelte'

  /*
   * The glance strip (GDK-871): what moved while the phone was away, as a
   * band above the Issues queue — not a fourth tab (DESIGN.md §1 non-jobs).
   * It led the bands when it was written; the session strip (GDK-1495) and
   * the sprint line (GDK-1867) now sit above it, both of which say what is
   * true of the workspace rather than what is unread by this person.
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
      {@const detail = feedDetail(item)}
      <button class="g-item" onclick={() => open(item)}>
        <span class="key">{item.issue_key}</span>
        <span class="what">{feedKindLabel(item.event_type)}</span>
        {#if detail}
          <!-- What moved, in place of who moved it: at 402pt the line holds
               one of the two, and "Priority, Labels" answers the strip's
               question where a name does not. The actor waits in Detail. -->
          <span class="who">{detail}</span>
        {:else if item.actor_name}
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
  /* The kind label never gives way (GDK-1963). It and .who were both
     `flex: 0 1 auto`, so an overflowing row shrank them in proportion and a
     long comment body clipped the label beside it — measured on the phone
     clip, `New comment` rendered as `New …` in 31px while its own text
     wanted 81. The label comes from a closed catalogue of seven short
     strings; the detail is whatever a person wrote, and the ellipsis is
     that one's job. The row's own `overflow: hidden` is the backstop at
     widths where even the label cannot fit. */
  .what {
    flex: none;
    color: var(--color-text-primary);
  }
  .who {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .who::before {
    content: '· ';
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
