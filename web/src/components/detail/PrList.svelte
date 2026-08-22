<script lang="ts">
  /*
   * Linked PRs ([detail]). State chip (open/merged/closed) + repo#number + title + new-tab.
   * A dev link also names who attached it (linked_by) — a different person from
   * the PR's own author, and a bot's claim trail (GDK-590).
   */
  import { t } from '../../lib/i18n'
  import type { LinkedPr } from '../../lib/types'
  import BotBadge from '../list/BotBadge.svelte'

  let { prs }: { prs: LinkedPr[] } = $props()

  /** PR state → chip color class. merged=purple, open=green, closed=red. */
  function stateClass(state: string): string {
    const s = state.toLowerCase()
    if (s === 'merged') return 'bg-accent/15 text-accent-text'
    if (s === 'open') return 'bg-status-done/15 text-status-done'
    if (s === 'closed' || s === 'declined') return 'bg-status-reopen/15 text-status-reopen'
    return 'bg-bg-active text-text-secondary'
  }

  function repoLabel(pr: LinkedPr): string {
    const repo = pr.repo ? pr.repo.split('/').pop() : null
    return `${repo ?? t('common.unknown')}#${pr.number}`
  }
</script>

<ul class="flex flex-col gap-1">
  {#each prs as pr (pr.url)}
    <li>
      <a
        href={pr.url}
        target="_blank"
        rel="noopener noreferrer"
        class="group flex items-start gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-bg-hover"
      >
        {#if pr.state}
          <span
            class="mt-px flex-none rounded px-1.5 py-0.5 text-micro font-semibold uppercase {stateClass(pr.state)}"
          >
            {pr.state}
          </span>
        {/if}
        <span class="min-w-0 flex-1">
          <span class="font-mono text-micro text-text-muted">{repoLabel(pr)}</span>
          <span class="block truncate text-body text-text-secondary group-hover:text-text-primary">
            {pr.title}
          </span>
          {#if pr.linked_by}
            <span class="block text-micro text-text-muted">
              {t('detail.prLinkedBy', { name: pr.linked_by })}
              <BotBadge accountId={pr.linked_by_id} />
            </span>
          {/if}
        </span>
      </a>
    </li>
  {/each}
</ul>
