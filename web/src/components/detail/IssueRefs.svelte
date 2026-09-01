<script lang="ts">
  /*
   * Cross-workspace references ([detail], GDK-1032). Each row is a pointer
   * this issue carries at something outside its own tracker. When the
   * machine mirrors the named workspace the server hydrates the target's
   * live status and assignee — that pair (a personal issue beside a team
   * issue's current state, with no network call) is the whole point, so the
   * row leads with it. An unhydrated row is a normal state, not an error:
   * the pointer is fine, this machine just does not mirror that workspace.
   */
  import { t } from '../../lib/i18n'
  import type { IssueRef } from '../../lib/types'

  let { refs }: { refs: IssueRef[] } = $props()

  /** Dot colour follows the target's status category, the same vocabulary
   *  the rest of the app keys on (never the localized display name). */
  function dotClass(ref: IssueRef): string {
    switch (ref.status_category) {
      case 'done':
        return 'bg-status-done'
      case 'inprogress':
        return 'bg-status-inprogress'
      default:
        return 'bg-border-strong'
    }
  }

  function target(ref: IssueRef): string {
    if (ref.workspace && ref.key) return `${ref.workspace}/${ref.key}`
    return ref.title || ref.url
  }
</script>

<ul class="flex flex-col gap-1" data-testid="issue-refs">
  {#each refs as ref (ref.id)}
    <li
      class="flex w-full items-start gap-2 rounded-md px-2 py-1.5"
      data-testid="issue-ref"
      data-hydrated={ref.hydrated ? 'true' : 'false'}
    >
      <span class="mt-px flex-none text-micro text-text-muted"
        >{ref.relationship || t('detail.refRelates')}</span
      >
      <span class="min-w-0 flex-1">
        <span class="flex items-center gap-1.5">
          {#if ref.workspace && ref.key}
            <span class="font-mono text-micro font-medium text-accent-text">{target(ref)}</span>
          {:else}
            <a
              href={ref.url}
              target="_blank"
              rel="noreferrer noopener"
              class="truncate text-micro text-accent-text hover:underline">{target(ref)}</a
            >
          {/if}
          {#if ref.hydrated}
            <span class="h-1.5 w-1.5 flex-none rounded-full {dotClass(ref)}" aria-hidden="true"
            ></span>
            <span class="flex-none text-micro text-text-muted">{ref.status}</span>
            {#if ref.assignee}
              <span class="flex-none text-micro text-text-muted">· {ref.assignee}</span>
            {/if}
          {/if}
        </span>
        {#if ref.summary}
          <span class="block truncate text-body text-text-secondary">{ref.summary}</span>
        {/if}
        {#if !ref.hydrated && ref.workspace}
          <span class="block text-micro text-text-muted" data-testid="issue-ref-unmirrored"
            >{t('detail.refNotMirrored', { workspace: ref.workspace })}</span
          >
        {/if}
      </span>
    </li>
  {/each}
</ul>
