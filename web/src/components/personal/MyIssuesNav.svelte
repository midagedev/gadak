<script lang="ts">
  /*
   * Personal sidebar row ([personal]): the feed door. Feed opens the feed
   * screen, which no built-in view replaces — one plain row, the inbox. The
   * block lost its "My Issues" heading and "Assigned to me" row in the first
   * 2026-09-07 subtraction (the built-in My issues view owns that question),
   * and its "Reported by me" row in the second (GDK-1493): the open half of
   * that question is the Handed off view, and the feed screen keeps its
   * reporter focus as a toggle. Unread count = API result count.
   * Without identity: local-origin and the demo say why the rows are absent
   * (GDK-1122 — neither can configure one); a connected workspace prompts to
   * set credentials.
   */
  import { t } from '../../lib/i18n'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { feature } from '../../lib/config'
  import { isHostedDemo, isLocalOriginWorkspace } from '../../lib/config'
  import { cappedCount } from '../../lib/format'
  import Icon from '../ui/Icon.svelte'

  // Without feed, hide the row — no panel to open.
  const feedOn = feature('feed')
  const feedUnreadCount = $derived(me.feedUnread.all)
</script>

{#if me.identified && feedOn}
  <div class="mb-2">
    <!-- The feed row carries aria-current while the feed holds the main
         column — same condition as its paint, and the semantic axis e2e
         reads instead of the bg token (GDK-613). -->
    <button
      type="button"
      class="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-body transition-colors {me.feedOpen
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      aria-current={me.feedOpen ? 'true' : undefined}
      onclick={() => me.openFeed('all')}
      title={t('personal.feedHint')}
    >
      <Icon name="megaphone" size={14} class="flex-none text-text-muted" />
      <span class="min-w-0 flex-1 truncate">{t('common.feed')}</span>
      {#if feedUnreadCount}
        <span
          class="flex h-4 min-w-4 flex-none items-center justify-center rounded-full bg-accent-subtle px-1.5 font-mono text-micro font-semibold tabular-nums text-accent-text"
          >{cappedCount(feedUnreadCount)}</span
        >
      {/if}
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
