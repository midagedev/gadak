<script lang="ts">
  /*
   * My Issues sidebar section ([personal]).
   *  Rows: assigned to me N / reported by me N / mentioned me (feed) N.
   *   - Assigned: showIssueList(assignee + active statuses) → main list.
   *   - Reported: open personal feed on the "reported" focus tab.
   *   - Mentions/feed: open personal feed (all focus).
   *  Counts are $derived from the local pool (mentions = API result count).
   *  Without identity: prompt to set credentials.
   */
  import { t } from '../../lib/i18n'
  import { issueMatchesPerson } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { showIssueList } from '../../lib/show-issue-list'
  import { effectiveCategory, emptyConfig, type ViewConfig } from '../../lib/view-config'
  import { feature } from '../../lib/config'
  import { isHostedDemo } from '../../lib/config'
  import type { IssueLite } from '../../lib/types'
  import Icon from '../ui/Icon.svelte'

  const myIdentity = $derived(me.accountId ?? me.email)
  const isMe = (issue: IssueLite, role: 'assignee' | 'reporter') =>
    issueMatchesPerson(issue, role, me.accountId) || issueMatchesPerson(issue, role, me.email)
  // Without feed, hide "reported by me" / "feed" — no panel to open.
  const feedOn = feature('feed')

  // Counts use active (non-done) issues.
  const assignedCount = $derived(
    myIdentity
      ? issues.allIssues.filter(
          (i) => isMe(i, 'assignee') && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const reportedCount = $derived(
    myIdentity
      ? issues.allIssues.filter(
          (i) => isMe(i, 'reporter') && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const feedUnreadCount = $derived(me.feedUnread.all)

  function assigneeConfig(): ViewConfig {
    const c = emptyConfig()
    c.filters.assignee_email = [myIdentity!]
    c.filters.status_category = ['new', 'inprogress']
    return c
  }

  function applyAssignee() {
    showIssueList(assigneeConfig(), true)
  }
</script>

<div class="mb-3">
  <div class="px-3 py-1 text-micro font-medium uppercase tracking-wide text-text-muted">
    {t('personal.myIssues')}
  </div>

  {#if me.identified}
    <button
      type="button"
      class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={applyAssignee}
    >
      <Icon name="user" size={15} class="text-text-muted" />
      <span class="min-w-0 flex-1 truncate">{t('personal.myAssignee')}</span>
      <span class="flex-none text-micro text-text-muted">{assignedCount}</span>
    </button>

    {#if feedOn}
    <button
      type="button"
      class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {me.feedOpen &&
      me.feedFocus === 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('reporter')}
    >
      <Icon name="pen" size={15} class="text-text-muted" />
      <span class="min-w-0 flex-1 truncate">{t('personal.myReporter')}</span>
      <span class="flex-none text-micro text-text-muted">{reportedCount}</span>
    </button>

    <button
      type="button"
      class="flex h-control w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {me.feedOpen &&
      me.feedFocus !== 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      onclick={() => me.openFeed('all')}
      title={t('personal.feedHint')}
    >
      <Icon name="megaphone" size={15} class="text-text-muted" />
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
    <p class="px-3 py-1.5 text-micro text-text-muted">{t('personal.demoNoIdentity')}</p>
  {:else}
    <button
      type="button"
      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-body text-text-muted transition-colors hover:bg-bg-hover hover:text-text-secondary"
      onclick={() => write.openSettings()}
    >
      {t('personal.needCredentials')}
    </button>
  {/if}
</div>
