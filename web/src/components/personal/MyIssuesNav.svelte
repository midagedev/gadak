<script lang="ts">
  /*
   * Personal sidebar rows ([personal]): the feed doors. Feed (all focus) and
   * Reported by me (reporter focus) open the feed screen, which no built-in
   * view replaces — they stand here as two plain rows. The block lost its
   * "My Issues" heading and its "Assigned to me" row in the 2026-09-07
   * sidebar subtraction: the built-in My issues view is that question's one
   * owner (same filter, with a sort).
   * Counts are $derived from the local pool (unread = API result count).
   * Without identity: local-origin and the demo say why the rows are absent
   * (GDK-1122 — neither can configure one); a connected workspace prompts to
   * set credentials.
   */
  import { t } from '../../lib/i18n'
  import { issueMatchesPerson } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { effectiveCategory } from '../../lib/view-config'
  import { feature } from '../../lib/config'
  import { isHostedDemo, isLocalOriginWorkspace } from '../../lib/config'
  import type { IssueLite } from '../../lib/types'
  import Icon from '../ui/Icon.svelte'

  const myIdentity = $derived(me.accountId ?? me.email)
  const isMe = (issue: IssueLite, role: 'reporter') =>
    issueMatchesPerson(issue, role, me.accountId) || issueMatchesPerson(issue, role, me.email)
  // Without feed, hide both rows — no panel to open.
  const feedOn = feature('feed')

  // Counts use active (non-done) issues.
  const reportedCount = $derived(
    myIdentity
      ? issues.allIssues.filter(
          (i) => isMe(i, 'reporter') && effectiveCategory(i) !== 'done',
        ).length
      : 0,
  )
  const feedUnreadCount = $derived(me.feedUnread.all)
</script>

{#if me.identified && feedOn}
  <!-- The feed is the door, the reporter focus its refinement — that order. -->
  <div class="mb-2">
    <!-- The two feed rows carry aria-current while the feed holds the main
         column — same condition as their paint, and the semantic axis e2e
         reads instead of the bg token (GDK-613). -->
    <button
      type="button"
      class="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {me.feedOpen &&
      me.feedFocus !== 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      aria-current={me.feedOpen && me.feedFocus !== 'reporter' ? 'true' : undefined}
      onclick={() => me.openFeed('all')}
      title={t('personal.feedHint')}
    >
      <Icon name="megaphone" size={14} class="flex-none text-text-muted" />
      <span class="min-w-0 flex-1 truncate">{t('common.feed')}</span>
      {#if feedUnreadCount}
        <span
          class="flex h-4 min-w-4 flex-none items-center justify-center rounded-full bg-accent-subtle px-1.5 font-mono text-micro font-semibold tabular-nums text-accent-text"
          >{feedUnreadCount > 99 ? '99+' : feedUnreadCount}</span
        >
      {/if}
    </button>

    <button
      type="button"
      class="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {me.feedOpen &&
      me.feedFocus === 'reporter'
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      aria-current={me.feedOpen && me.feedFocus === 'reporter' ? 'true' : undefined}
      onclick={() => me.openFeed('reporter')}
    >
      <Icon name="pen" size={14} class="flex-none text-text-muted" />
      <span class="min-w-0 flex-1 truncate">{t('personal.myReporter')}</span>
      <span class="flex-none font-mono text-micro tabular-nums text-text-muted">{reportedCount}</span>
    </button>
  </div>
{:else if !me.identified}
  <div class="mb-2">
    {#if isLocalOriginWorkspace()}
      <!-- GDK-1122: a local-origin workspace has no credential and no identity to
           configure — the seeded origin is the machine's own tracker (see
           lib/workspace.ts), and writes are attributed to the process actor, not
           to a reader. A credentials CTA here would send this workspace's only
           audience to a dialog that cannot help, so the anonymous branch says
           why the rows are absent instead. -->
      <p class="px-3 py-1.5 text-micro text-text-muted" data-testid="my-issues-local-origin-note">
        {t('personal.localOriginNoIdentity')}
      </p>
    {:else if isHostedDemo()}
      <!-- The feed rows need a Jira identity, which the demo has no way to
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
{/if}
