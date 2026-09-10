<script lang="ts">
  /*
   * Priority chip + dropdown. Idle looks like the old read-only chip; hover
   * shows the same chevron as status. The list is the site catalog (id +
   * localized name), most urgent first, plus "none" to clear.
   */
  import { t } from '../../lib/i18n'
  import { EMPTY_VALUE } from '../../lib/chrome'
  import MeterBar from '../ui/MeterBar.svelte'
  import type { IssueLite, PriorityOption } from '../../lib/types'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { filters } from '../../stores/filters.svelte'
  import { priorityMeta } from '../../lib/format'
  import { effectiveCategory } from '../../lib/view-config'
  import { ESC_TIER, isEscapeKey, onEscape, onOutsideClick } from '../../lib/dom-actions'
  import { MenuOriginTimeout, withMenuTimeout } from '../../lib/menu-loading'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { DETAIL_TESTID } from '../../lib/commands'
  import { asKeyTarget } from '../../lib/key-targets'
  import LoadingState from '../ui/LoadingState.svelte'

  let { issue }: { issue: IssueLite } = $props()

  let open = $state(false)
  let busy = $state(false)
  let listEl = $state<HTMLDivElement | null>(null)
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let rootEl = $state<HTMLDivElement | null>(null)

  /* ── Origin wait (GDK-1566) ──
     The per-key catalog proxies to the origin; an unreachable one held the
     menu on a bare "Loading…" for its whole upstream timeout (measured 15 s).
     The wait obeys the shared skeleton grace, ends at the shared cap
     (MENU_ORIGIN_TIMEOUT_MS), and what replaces it is an answer: the site
     catalog stands in — labeled with the offline note — when one exists,
     otherwise the catalog's failure sentence and a Retry. A late per-key
     success still lands in the store; `options` flips to it and the note
     drops on its own. */
  let load = $state<'idle' | 'loading' | 'cached' | 'error'>('idle')
  const loadGrace = createSkeletonGrace(() => load === 'loading', () => issue.issue_key)

  const meta = $derived(priorityMeta(issue.priority_rank, issue.priority))
  const canEdit = $derived(me.identified)
  // Cached fallback = site rows while the per-key answer is missing.
  const options = $derived(
    load === 'cached' && !write.hasPrioritiesFor(issue.issue_key)
      ? write.priorities
      : write.prioritiesFor(issue.issue_key),
  )

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

  function close() {
    // A closed picker hands focus back to its trigger, as dialogs do to their
    // opener (focus-trap) — but only when the picker still holds it, so
    // closing on an outside click never steals focus from where the user
    // moved it.
    if (rootEl?.contains(document.activeElement)) triggerEl?.focus()
    open = false
  }

  // Menu-tier claim on the Esc stack (GDK-1565): the picker outranks the
  // surface it sits on, and acting spends the key so the detail panel keeps
  // its own Esc. The delegated onkeydown below is the same handler one
  // phase early — it sees the key while it still walks the picker, where its
  // stopPropagation shields the shell keymap without the tier's help.
  function onEsc(e: KeyboardEvent) {
    if (!isEscapeKey(e) || !open) return
    e.preventDefault()
    e.stopPropagation()
    close()
  }

  /** Load the per-key catalog under the shared cap; sets the wait state. */
  async function loadOptions(): Promise<void> {
    load = 'loading'
    try {
      const ok = await withMenuTimeout(write.loadPrioritiesFor(issue.issue_key))
      if (!ok) {
        // Gate refusal or a refused GET — the store already dialoged/toasted
        // it; the old behavior on this path was to close, and still is.
        close()
        return
      }
      load = 'idle'
    } catch (e) {
      if (!(e instanceof MenuOriginTimeout)) throw e
      load = write.priorities.length ? 'cached' : 'error'
    }
  }

  async function toggle() {
    if (open) {
      close()
      return
    }
    if (!(await write.ensureWritableFor(issue.issue_key))) return
    open = true
    if (!write.hasPrioritiesFor(issue.issue_key)) await loadOptions()
    queueMicrotask(() => listEl?.querySelector('button')?.focus())
  }

  async function pick(p: PriorityOption | null) {
    if (p && p.name === issue.priority) {
      close()
      return
    }
    if (!p && !issue.priority) {
      close()
      return
    }
    busy = true
    const ok = await write.setPriority(issue.issue_key, p)
    busy = false
    if (ok) close()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative inline-block"
  bind:this={rootEl}
  onkeydown={onEsc}
  use:onEscape={{ handler: onEsc, priority: ESC_TIER.menu, label: 'priority-picker' }}
  use:onOutsideClick={{ handler: close, enabled: open }}
>
  <!-- One option's distribution share (M4): a mini bar scaled to the widest
       rank, and the count. Null (nothing at this rank, or an empty view)
       renders nothing. -->
  {#snippet share(s: { n: number; pct: number; bar: number } | null)}
    {#if s}
      <span class="ml-auto flex flex-none items-center gap-1.5 pl-2">
        <MeterBar percent={s.bar} height="h-[3px]" width="w-10" class="block flex-none" />
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
    bind:this={triggerEl}
    use:asKeyTarget={DETAIL_TESTID.priority}
    data-testid={DETAIL_TESTID.priority}
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
      <span class={EMPTY_VALUE}>{t('list.priorityNone')}</span>
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
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-48 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
      role="listbox"
      aria-label={t('common.priority')}
    >
      {#if load === 'loading'}
        <div data-skeleton={loadGrace.attr}>
          {#if loadGrace.visible}<LoadingState compact />{/if}
        </div>
      {:else if load === 'error'}
        <div class="flex flex-col gap-1 px-3 py-2">
          <p class="text-micro text-status-reopen" data-testid="menu-load-error">
            {t('write.prioritiesFailed')}
          </p>
          <button
            type="button"
            data-testid="menu-load-retry"
            onclick={() => void loadOptions()}
            class="self-start text-micro font-medium text-accent-text hover:underline"
          >
            {t('common.retry')}
          </button>
        </div>
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
        {#if load === 'cached' && !write.hasPrioritiesFor(issue.issue_key)}
          <!-- The rows above are the site catalog standing in for the
               per-key answer the origin never gave (GDK-1566). -->
          <div
            class="border-t border-border-subtle px-3 py-1.5 text-micro text-text-muted"
            data-testid="menu-cached-note"
          >
            {t('app.offlineBanner')}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
