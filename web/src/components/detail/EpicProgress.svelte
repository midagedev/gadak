<script lang="ts">
  /*
   * Epic progress ([detail]) — shown when the open issue is an epic that owns
   *  work, i.e. the pool holds issues whose epic_key is this key. Type names are
   *  localized per site, so membership decides this, not issue_type.
   *
   *  Everything here comes from the pool: the epic's rollup is answered without
   *  a request, which is the point of mirroring locally.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { CATEGORY_META, categoryOf } from '../../lib/format'
  import Section from './Section.svelte'

  let { issueKey }: { issueKey: string } = $props()

  /** Collapse threshold — past this the section leads with the rollup, not a wall of rows. */
  const PREVIEW = 20

  const children = $derived(issues.allIssues.filter((i) => i.epic_key === issueKey))
  const doneCount = $derived(children.filter((i) => categoryOf(i) === 'done').length)
  const percent = $derived(
    children.length === 0 ? 0 : Math.round((doneCount / children.length) * 100),
  )

  let expanded = $state(false)
  // A new epic starts collapsed again; otherwise the previous one's choice leaks.
  $effect(() => {
    void issueKey
    expanded = false
  })

  const shown = $derived<IssueLite[]>(
    children.length > PREVIEW && !expanded ? children.slice(0, PREVIEW) : children,
  )
  const hidden = $derived(children.length - shown.length)
</script>

{#if children.length > 0}
  <Section title={t('detail.epicChildren')} count={children.length}>
    <div data-testid="epic-progress" class="mb-3">
      <div class="mb-2 flex items-baseline gap-2 text-[12px]">
        <span class="text-text-secondary">
          {t('detail.epicProgress', { done: doneCount, total: children.length })}
        </span>
        <span class="tabular-nums text-text-muted">{percent}%</span>
      </div>
      <!-- Same three-bucket vocabulary as the list's status dots, as one bar. -->
      <div class="h-1 w-full overflow-hidden rounded-full bg-bg-elevated" aria-hidden="true">
        <div
          class="h-full rounded-full transition-[width]"
          style:width="{percent}%"
          style:background={CATEGORY_META.done.color}
        ></div>
      </div>
    </div>

    <ul class="flex flex-col gap-1">
      {#each shown as child (child.issue_key)}
        <li>
          <button
            type="button"
            data-testid="epic-child-row"
            onclick={() => selection.select(child.issue_key)}
            class="group flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-hover"
          >
            <span
              class="h-1.5 w-1.5 flex-none rounded-full"
              style:background={CATEGORY_META[categoryOf(child)].color}
              title={child.status}
            ></span>
            <span class="w-[76px] flex-none truncate font-mono text-micro font-medium text-accent-text">
              {child.issue_key}
            </span>
            <span class="min-w-0 flex-1 truncate text-[12px] text-text-secondary group-hover:text-text-primary">
              {child.summary}
            </span>
            <span class="max-w-[88px] flex-none truncate text-micro text-text-muted">
              {child.status}
            </span>
          </button>
        </li>
      {/each}
    </ul>

    {#if hidden > 0 || expanded}
      <button
        type="button"
        data-testid="epic-children-toggle"
        onclick={() => (expanded = !expanded)}
        class="mt-1 rounded-md px-2 py-1 text-micro text-text-muted transition-colors hover:bg-bg-hover hover:text-text-secondary"
      >
        {expanded ? t('detail.epicShowLess') : t('detail.epicShowAll', { n: hidden })}
      </button>
    {/if}
  </Section>
{/if}
