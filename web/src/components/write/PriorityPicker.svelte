<script lang="ts">
  /*
   * Priority chip + dropdown. Idle looks like the old read-only chip; hover
   * shows the same chevron as status. The list is the site catalog (id +
   * localized name), most urgent first, plus "none" to clear.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite, PriorityOption } from '../../lib/types'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { filters } from '../../stores/filters.svelte'
  import { priorityMeta } from '../../lib/format'
  import { effectiveCategory } from '../../lib/view-config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'

  let { issue }: { issue: IssueLite } = $props()

  let open = $state(false)
  let loading = $state(false)
  let busy = $state(false)
  let listEl = $state<HTMLDivElement | null>(null)

  const meta = $derived(priorityMeta(issue.priority_rank, issue.priority))
  const canEdit = $derived(me.identified)
  const options = $derived(write.prioritiesFor(issue.issue_key))

  /* ── Coaching, M4 (G4) ──
     The distribution the reader is about to enter: each option carries a mini
     bar and the count of this view's open issues already at that rank. No
     judgement text — the arrangement is the message (THEORY.md "Choosing a
     priority"). Read-only on the filters store; grouping is by priority_rank
     (0 = unset → the none row), never by display name. The option objects
     carry only the origin's priority id + name, so the rank join is the same
     index+1 convention the picker already uses for priorityMeta/setPriority —
     the catalog is most-urgent-first and "none" sits above rank 1. */
  const shares = $derived.by(() => {
    const byRank = new Map<number, number>()
    let total = 0
    for (const it of filters.visibleIssues) {
      if (effectiveCategory(it) === 'done') continue
      const rank = it.priority_rank ?? 0
      byRank.set(rank, (byRank.get(rank) ?? 0) + 1)
      total++
    }
    let widest = 0
    for (const n of byRank.values()) if (n > widest) widest = n
    return { byRank, total, widest }
  })

  /** One option's share, or null when there is nothing to show for it. */
  function shareOf(rank: number): { n: number; pct: number; bar: number } | null {
    const n = shares.byRank.get(rank) ?? 0
    if (shares.total === 0 || n === 0) return null
    return {
      n,
      pct: Math.round((n / shares.total) * 100),
      bar: shares.widest > 0 ? Math.round((n / shares.widest) * 100) : 0,
    }
  }

  async function toggle() {
    if (open) {
      open = false
      return
    }
    if (!(await write.ensureWritableFor(issue.issue_key))) return
    open = true
    if (!write.hasPrioritiesFor(issue.issue_key)) {
      loading = true
      const ok = await write.loadPrioritiesFor(issue.issue_key)
      loading = false
      if (!ok) {
        open = false
        return
      }
    }
    queueMicrotask(() => listEl?.querySelector('button')?.focus())
  }

  async function pick(p: PriorityOption | null) {
    if (p && p.name === issue.priority) {
      open = false
      return
    }
    if (!p && !issue.priority) {
      open = false
      return
    }
    busy = true
    const ok = await write.setPriority(issue.issue_key, p)
    busy = false
    if (ok) open = false
  }
</script>

<div
  class="relative inline-block"
  use:onOutsideClick={{ handler: () => (open = false), enabled: open }}
>
  <!-- One option's distribution share (M4): a mini bar scaled to the widest
       rank, and the count. Null (nothing at this rank, or an empty view)
       renders nothing. -->
  {#snippet share(s: { n: number; pct: number; bar: number } | null)}
    {#if s}
      <span class="ml-auto flex flex-none items-center gap-1.5 pl-2">
        <span class="h-[3px] w-10 overflow-hidden rounded-full bg-text-muted/15" aria-hidden="true">
          <span class="block h-full rounded-full bg-text-muted/30" style="width:{s.bar}%"></span>
        </span>
        <span
          class="text-micro text-text-muted tabular-nums"
          data-testid="priority-share"
          title={t('detail.priorityShare', { n: s.n, total: shares.total, pct: s.pct })}
        >
          {s.n}
        </span>
      </span>
    {/if}
  {/snippet}
  <button
    type="button"
    onclick={toggle}
    data-testid="priority-picker"
    class="group inline-flex items-center gap-1.5 rounded-md bg-bg-elevated px-2 py-0.5 text-micro font-medium text-text-secondary transition-colors hover:bg-bg-hover"
    aria-haspopup="listbox"
    aria-expanded={open}
    title={canEdit ? t('write.changePriority') : (issue.priority ?? t('list.priorityNone'))}
  >
    <span
      class="h-1.5 w-1.5 flex-none rounded-full {issue.priority
        ? ''
        : 'border border-dashed border-border-strong bg-transparent'}"
      style={issue.priority ? `background:${meta.color}` : undefined}
    ></span>
    {#if issue.priority}
      {t('detail.priorityShort', { p: issue.priority })}
    {:else}
      <span class="italic text-text-muted">{t('list.priorityNone')}</span>
    {/if}
    <svg
      width="9"
      height="9"
      viewBox="0 0 10 10"
      fill="none"
      aria-hidden="true"
      class="text-text-muted opacity-0 transition-opacity group-hover:opacity-100"
    >
      <path d="M2.5 4l2.5 2.5L7.5 4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </button>

  {#if open}
    <div
      bind:this={listEl}
      use:onEscape={(e) => {
        e.preventDefault()
        open = false
      }}
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-48 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
      role="listbox"
      aria-label={t('common.priority')}
    >
      {#if loading}
        <div class="px-3 py-2 text-micro text-text-muted">{t('common.loading')}</div>
      {:else}
        <button
          type="button"
          role="option"
          aria-selected={!issue.priority}
          onclick={() => pick(null)}
          disabled={busy}
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-body text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none disabled:opacity-50"
        >
          <span class="h-1.5 w-1.5 flex-none rounded-full border border-dashed border-border-strong"></span>
          {t('common.none')}
          {@render share(shareOf(0))}
        </button>
        {#each options as p, i (p.id)}
          {@const opt = priorityMeta(i + 1, p.name)}
          <button
            type="button"
            role="option"
            aria-selected={p.name === issue.priority}
            onclick={() => pick(p)}
            disabled={busy}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-body transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none disabled:opacity-50 {p.name ===
            issue.priority
              ? 'text-text-primary'
              : 'text-text-secondary'}"
          >
            <span class="h-1.5 w-1.5 flex-none rounded-full" style="background:{opt.color}"></span>
            <span class="min-w-0 flex-1 truncate">{p.name}</span>
            {@render share(shareOf(i + 1))}
          </button>
        {/each}
        {#if options.length === 0}
          <div class="px-3 py-2 text-micro text-text-muted">{t('write.noPriorities')}</div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
