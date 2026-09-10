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
  import { workspaceHref } from '../../lib/api'
  import { workspaces } from '../../stores/workspaces.svelte'
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

  /*
   * GDK-1326: a workspace/key row was styled exactly like the link beside it
   * (accent, medium, mono) while going nowhere. Mirrored targets become real
   * links — the switcher's own href (workspaceHref, so the primary resolves
   * to `/` instead of the /w/<name>/ 404 of GitHub #85) with the issue
   * deep-linked the way App restores it. Unmirrored targets demote to plain
   * text: link-coloured mono that cannot navigate is a promise the pointer
   * does not keep. Reads the workspace list, so it is reactive on the boot
   * fetch — a detail opened before the list lands demotes for a moment and
   * links once it arrives.
   */
  function refHref(ref: IssueRef): string | null {
    if (!ref.workspace || !ref.key) return null
    const w = workspaces.list.find((x) => x.name === ref.workspace)
    if (!w) return null
    return `${workspaceHref(w)}#/?issue=${encodeURIComponent(ref.key)}`
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
            {#if refHref(ref)}
              <a
                href={refHref(ref)}
                class="font-mono text-micro font-medium text-accent-text hover:underline">{target(ref)}</a
              >
            {:else}
              <span class="font-mono text-micro text-text-secondary">{target(ref)}</span>
            {/if}
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
