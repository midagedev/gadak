<script lang="ts">
  /*
   * Deploy status timeline ([detail]).
   *
   * Stages: merge (N/M PR) → dev release → qa release → qa swap (QA ready) → prod.
   *  Reached stages: filled circle + emphasis; unreached: empty gray. QA swap is
   *  highlighted as the QA team's key stage.
   *
   * Defensive parsing: older servers may omit/partial-send deploy — optional chain.
   *  (If state is missing entirely, parent DetailPanel skips this section.)
   */
  import { t } from '../../lib/i18n'
  import type { DeployDetail, DeployState } from '../../lib/types'
  import { absoluteTime } from './format'

  let { deploy }: { deploy: DeployDetail } = $props()

  // Stage rank — for "reached?" checks.
  const RANK: Record<DeployState, number> = {
    none: 0,
    merged: 1,
    dev: 2,
    qa_preview: 3,
    qa: 4,
    prod: 5,
  }

  const state = $derived((deploy.state ?? 'none') as DeployState)
  const rank = $derived(RANK[state] ?? 0)

  interface Step {
    /** Minimum rank required to count as reached. */
    at: number
    label: string
    /** Secondary detail (tag/time etc.). */
    detail: string | null
    /** External link (release html_url etc.). */
    href: string | null
    /** Emphasize as "QA can verify". */
    highlight: boolean
  }

  /** Find html_url for a channel in inclusion evidence (if any). */
  function releaseUrl(channel: string): string | null {
    const found = (deploy.releases ?? []).find((r) => (r.channel ?? '') === channel)
    return found?.html_url ?? null
  }

  const steps = $derived.by<Step[]>(() => {
    const mergedText =
      deploy.total_prs != null
        ? t('deploy.prMergedFrac', { a: deploy.merged_prs ?? 0, b: deploy.total_prs })
        : deploy.merged_prs != null
          ? t('deploy.prMergedCount', { n: deploy.merged_prs })
          : null
    const devText = deploy.dev ? `${deploy.dev.tag} · ${absoluteTime(deploy.dev.at)}` : null
    const qaRelText = deploy.qa_release
      ? `${deploy.qa_release.tag} · ${absoluteTime(deploy.qa_release.at)}`
      : null
    const swapText = deploy.qa_swapped_at ? absoluteTime(deploy.qa_swapped_at) : null
    const prodText = deploy.prod_at ? absoluteTime(deploy.prod_at) : null

    return [
      { at: 1, label: t('deploy.merge'), detail: mergedText, href: null, highlight: false },
      { at: 2, label: t('deploy.dev'), detail: devText, href: releaseUrl('dev'), highlight: false },
      { at: 3, label: t('deploy.qaRelease'), detail: qaRelText, href: releaseUrl('qa'), highlight: false },
      {
        at: 4,
        label: t('deploy.qaSwapReady'),
        detail: swapText,
        href: null,
        highlight: true,
      },
      { at: 5, label: t('deploy.prod'), detail: prodText, href: releaseUrl('prod'), highlight: false },
    ]
  })

  // Per-PR inclusion evidence — collapsible list when present.
  const prList = $derived(deploy.prs ?? [])
</script>

<ol class="flex flex-col">
  {#each steps as step, i (step.at)}
    {@const reached = rank >= step.at}
    {@const isQaSwap = step.highlight && reached}
    <li class="flex gap-2.5">
      <!-- Left marker + connector -->
      <div class="flex flex-none flex-col items-center">
        <span
          class="mt-0.5 flex h-3 w-3 flex-none items-center justify-center rounded-full border transition-colors
            {isQaSwap
            ? 'border-[#2dd4bf] bg-[#2dd4bf]'
            : reached
              ? 'border-accent bg-accent'
              : 'border-border-strong bg-transparent'}"
        >
          {#if reached}
            <span class="h-1 w-1 rounded-full {isQaSwap ? 'bg-[#083344]' : 'bg-white'}"></span>
          {/if}
        </span>
        {#if i < steps.length - 1}
          <span
            class="my-0.5 w-px flex-1 {rank > step.at ? 'bg-border-strong' : 'bg-border-subtle'}"
          ></span>
        {/if}
      </div>

      <!-- Step content -->
      <div class="min-w-0 flex-1 pb-3">
        <div class="flex items-center gap-1.5">
          <span
            class="text-[12px] font-medium {isQaSwap
              ? 'text-[#5eead4]'
              : reached
                ? 'text-text-primary'
                : 'text-text-muted'}"
          >
            {step.label}
          </span>
          {#if step.href}
            <a
              href={step.href}
              target="_blank"
              rel="noopener noreferrer"
              class="text-[11px] text-accent-text hover:underline"
            >
              ↗
            </a>
          {/if}
        </div>
        {#if step.detail}
          <div class="mt-0.5 truncate font-mono text-[11px] text-text-muted" title={step.detail}>
            {step.detail}
          </div>
        {/if}
      </div>
    </li>
  {/each}
</ol>

<!-- Per-PR inclusion evidence (when present) -->
{#if prList.length > 0}
  <div class="mt-1 border-t border-border-subtle pt-3">
    <div class="mb-1.5 text-[11px] font-medium text-text-muted">{t('deploy.byPr')}</div>
    <ul class="flex flex-col gap-1">
      {#each prList as pr (pr.number)}
        <li class="flex items-center gap-2 text-[12px]">
          <span
            class="h-1.5 w-1.5 flex-none rounded-full {pr.included_in
              ? 'bg-status-done'
              : pr.merged
                ? 'bg-status-stale'
                : 'bg-border-strong'}"
            title={pr.included_in
              ? t('deploy.includedIn', { tag: pr.included_in })
              : pr.merged
                ? t('deploy.mergedNoRelease')
                : t('deploy.unmerged')}
          ></span>
          {#if pr.url}
            <a
              href={pr.url}
              target="_blank"
              rel="noopener noreferrer"
              class="font-mono text-[11px] text-accent-text hover:underline"
            >
              #{pr.number}
            </a>
          {:else}
            <span class="font-mono text-[11px] text-text-muted">#{pr.number}</span>
          {/if}
          <span class="min-w-0 flex-1 truncate text-text-secondary" title={pr.title ?? ''}>
            {pr.title ?? ''}
          </span>
          {#if pr.included_in}
            <span class="flex-none font-mono text-[10px] text-text-muted">{pr.included_in}</span>
          {/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}
