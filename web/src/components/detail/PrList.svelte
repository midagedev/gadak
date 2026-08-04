<script lang="ts">
  /*
   * 연결 PR ([detail]). 상태 칩(open/merged/closed) + repo#number + 제목 + 새탭 링크.
   */
  import { t } from '../../lib/i18n'
  import type { LinkedPr } from '../../lib/types'

  let { prs }: { prs: LinkedPr[] } = $props()

  /** PR 상태 → 칩 색 클래스. merged=보라, open=초록, closed=빨강. */
  function stateClass(state: string): string {
    const s = state.toLowerCase()
    if (s === 'merged') return 'bg-accent/15 text-accent-text'
    if (s === 'open') return 'bg-status-done/15 text-status-done'
    if (s === 'closed') return 'bg-status-reopen/15 text-status-reopen'
    return 'bg-bg-active text-text-secondary'
  }

  function repoLabel(pr: LinkedPr): string {
    const repo = pr.repo ? pr.repo.split('/').pop() : null
    return `${repo ?? 'repo'}#${pr.number}`
  }
</script>

{#if prs.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noPrs')}</p>
{:else}
  <ul class="flex flex-col gap-1">
    {#each prs as pr (pr.url)}
      <li>
        <a
          href={pr.url}
          target="_blank"
          rel="noopener noreferrer"
          class="group flex items-start gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-bg-hover"
        >
          <span
            class="mt-px flex-none rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase {stateClass(pr.state)}"
          >
            {pr.state}
          </span>
          <span class="min-w-0 flex-1">
            <span class="font-mono text-[11px] text-text-muted">{repoLabel(pr)}</span>
            <span class="block truncate text-[12px] text-text-secondary group-hover:text-text-primary">
              {pr.title}
            </span>
          </span>
        </a>
      </li>
    {/each}
  </ul>
{/if}
