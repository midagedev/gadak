<script lang="ts">
  /*
   * Issue breadcrumb ([detail]) — where this issue sits in the hierarchy.
   *  `epic › (parent ›) current`, resolved from the local pool only: the server
   *  already sent epic_key/parent_key on every IssueLite, so no request and no
   *  wait for the detail body. The parent segment is skipped when it *is* the
   *  epic (the common case) — repeating it would say nothing.
   *
   *  Same rhythm as DocumentPanel's doc-breadcrumb: one line, ancestors give up
   *  width first, the current key stays whole.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'

  let { issue }: { issue: IssueLite } = $props()

  type Crumb = { key: string; summary: string | null }

  // Ancestors, outermost first. At most two: epic, then the direct parent when
  // that is a different issue (a sub-task under a story under an epic).
  const trail = $derived.by(() => {
    const out: Crumb[] = []
    const seen = new Set<string>([issue.issue_key])
    for (const key of [issue.epic_key, issue.parent_key]) {
      if (!key || seen.has(key)) continue
      seen.add(key)
      out.push({ key, summary: issues.get(key)?.summary ?? null })
    }
    return out
  })
</script>

{#if trail.length > 0}
  <nav
    class="mb-2 flex items-center gap-1 overflow-hidden whitespace-nowrap text-[11px] text-text-muted"
    aria-label={t('detail.breadcrumb')}
    data-testid="issue-breadcrumb"
  >
    {#each trail as a (a.key)}
      <button
        type="button"
        class="flex min-w-0 shrink items-baseline gap-1 text-text-secondary transition-colors hover:text-text-primary hover:underline"
        title={a.summary ?? a.key}
        data-testid="issue-breadcrumb-ancestor"
        onclick={() => selection.select(a.key)}
      >
        <span class="flex-none font-mono">{a.key}</span>
        {#if a.summary}
          <span class="min-w-0 truncate">{a.summary}</span>
        {/if}
      </button>
      <span class="flex-none" aria-hidden="true">›</span>
    {/each}
    <span class="flex-none font-mono text-text-secondary">{issue.issue_key}</span>
  </nav>
{/if}
