<script lang="ts">
  /*
   * My Issues sidebar section ([personal]).
   *  Rows: assigned to me N / reported by me N / mentioned me (feed) N.
   *   - Assigned: filters.applyConfig(assignee + active statuses) → main list.
   *   - Reported: reporter isn't in explore filter schema (assignee_email only) —
   *     open personal feed on the "reported" focus tab (feed computes reporter).
   *   - Mentions/feed: open personal feed (all focus).
   *  Counts are $derived from the local pool (mentions = API result count).
   *  Without identity: prompt to set credentials.
   */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { effectiveCategory, emptyConfig, type ViewConfig } from '../../lib/view-config'
  import { feature } from '../../lib/config'
  import { isHostedDemo } from '../../lib/config'

  const myEmail = $derived(me.email)
  // Without feed, hide "reported by me" / "feed" — no panel to open.
  const feedOn = feature('feed')

  // Counts use active (non-done) issues.
  const assignedCount = $derived(
    myEmail
      ? issues.allIssues.filter(
          (i) => i.assignee_email === myEmail && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const reportedCount = $derived(
    myEmail
      ? issues.allIssues.filter(
          (i) => i.reporter_email === myEmail && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const feedUnreadCount = $derived(me.feedUnread.all)

  function assigneeConfig(): ViewConfig {
    const c = emptyConfig()
    c.filters.assignee_email = [myEmail!]
    c.filters.status_category = ['new', 'inprogress']
    return c
  }

  function applyAssignee() {
    me.closeFeed()
    filters.applyConfig(assigneeConfig())
  }
</script>

<div class="mb-3">
  <div class="px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-text-muted">
    {t('personal.myIssues')}
  </div>

  {#if me.identified}
    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={applyAssignee}
    >
      <span class="flex-none">🙋</span>
      <span class="min-w-0 flex-1 truncate">{t('personal.myAssignee')}</span>
      <span class="flex-none text-[11px] text-text-muted">{assignedCount}</span>
    </button>

    {#if feedOn}
    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {me.feedOpen &&
      me.feedFocus === 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('reporter')}
    >
      <span class="flex-none">✍️</span>
      <span class="min-w-0 flex-1 truncate">{t('personal.myReporter')}</span>
      <span class="flex-none text-[11px] text-text-muted">{reportedCount}</span>
    </button>

    <button
      type="button"
      class="flex min-h-7 w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {me.feedOpen &&
      me.feedFocus !== 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('all')}
      title={t('personal.feedHint')}
    >
      <span class="flex-none">📣</span>
      <span class="min-w-0 flex-1 truncate">{t('common.feed')}</span>
      {#if feedUnreadCount}
        <span
          class="min-w-5 flex-none rounded-full bg-accent px-1.5 py-0.5 text-center text-micro font-semibold text-white"
        >{feedUnreadCount > 99 ? '99+' : feedUnreadCount}</span>
      {/if}
    </button>
    {/if}
  {:else if isHostedDemo()}
    <!-- The personal views need a Jira identity, which the demo has no way to
         obtain. Say so rather than pointing at a credential dialog. -->
    <p class="px-3 py-1.5 text-[12px] text-text-muted">{t('personal.demoNoIdentity')}</p>
  {:else}
    <button
      type="button"
      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-secondary"
      onclick={() => write.openSettings()}
    >
      {t('personal.needCredentials')}
    </button>
  {/if}
</div>
