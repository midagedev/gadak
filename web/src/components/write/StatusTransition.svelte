<script lang="ts">
  /*
   * Status chip + transition dropdown (write, local-first).
   *  - Idle looks like the status chip; hover shows edit affordance (chevron).
   *  - Click → render from write.transitionsFor(issue) local map at 0ms; else GET
   *    <key>/transitions/ fallback.
   *  - Sort (quiet suggestions): ① recent transitions (per project) ② workflow forward
   *    (new→inprogress→done) ③ rest. Focus first item → Enter runs immediately.
   *  - On pick: write.transition() optimistic. Local fail (stale map) → remote refresh.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite, Transition } from '../../lib/types'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { recentOf } from '../../lib/recency'
  import { effectiveCategory } from '../../lib/view-config'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'

  let { issue }: { issue: IssueLite } = $props()

  let open = $state(false)
  let loading = $state(false)
  let remote = $state<Transition[] | null>(null) // fallback GET result
  let source = $state<'local' | 'remote'>('local')
  let loadError = $state<string | null>(null)
  let busyId = $state<string | null>(null)
  let listEl = $state<HTMLDivElement | null>(null)

  const cat = $derived(effectiveCategory(issue))
  const dotClass = $derived(
    cat === 'done' ? 'bg-status-done' : cat === 'new' ? 'bg-status-new' : 'bg-status-inprogress',
  )

  /** Jira statusCategory key → dot color class (via the single category owner). */
  function catDot(key: string): string {
    const c = effectiveCategory(key)
    return c === 'done' ? 'bg-status-done' : c === 'new' ? 'bg-status-new' : 'bg-status-inprogress'
  }

  /** Current category rank (new=0, inprogress=1, done=2) — for forward-direction. */
  const curRank = $derived(cat === 'new' ? 0 : cat === 'done' ? 2 : 1)
  function jiraRank(key: string): number {
    const c = effectiveCategory(key)
    return c === 'new' ? 0 : c === 'done' ? 2 : 1
  }

  /** Sorted transitions (local first → fallback). */
  const sorted = $derived.by<Transition[]>(() => {
    const base = remote ?? write.transitionsFor(issue) ?? []
    if (base.length === 0) return []
    const proj = write.projectOf(issue)
    const recent = recentOf(`transition:${proj}`)
    const recIdx = (id: string) => {
      const i = recent.indexOf(id)
      return i === -1 ? Infinity : i
    }
    return [...base].sort((a, b) => {
      const ra = recIdx(a.id)
      const rb = recIdx(b.id)
      if (ra !== rb) return ra - rb // ① recent
      const fa = jiraRank(a.to_category) > curRank ? 0 : 1
      const fb = jiraRank(b.to_category) > curRank ? 0 : 1
      if (fa !== fb) return fa - fb // ② forward direction first
      return 0
    })
  })

  async function toggle() {
    if (open) {
      open = false
      return
    }
    remote = null
    source = 'local'
    loadError = null
    // Local map hit → open immediately (0ms). Else remote fallback.
    if (write.transitionsFor(issue)) {
      open = true
      focusFirst()
    } else {
      await loadRemote()
    }
  }

  async function loadRemote() {
    // Remote GET needs auth → gate first.
    if (!(await write.ensureWritable())) {
      open = false
      return
    }
    open = true
    loading = true
    source = 'remote'
    loadError = null
    try {
      const res = await api.getTransitions(issue.issue_key)
      remote = res.transitions
      focusFirst()
    } catch (e) {
      if (
        e instanceof ApiError &&
        (e.code === 'credential_required' || e.code === 'credential_rejected')
      ) {
        open = false
        // #handleError path is inside write; surface the right copy here too.
        if (e.code === 'credential_rejected') write.toast(t('write.tokenRejected'), 'error')
        else write.toast(t('write.needToken'), 'info')
        write.openSettings()
        return
      }
      loadError = t('write.transitionsFailed')
      remote = []
    } finally {
      loading = false
    }
  }

  /** Focus first item → Enter runs immediately. */
  function focusFirst() {
    queueMicrotask(() => listEl?.querySelector('button')?.focus())
  }

  async function pick(t: Transition) {
    busyId = t.id
    const wasLocal = source === 'local'
    const ok = await write.transition(issue.issue_key, t)
    busyId = null
    if (ok) {
      open = false
      remote = null // status changed — refetch next time
    } else if (wasLocal) {
      // Local map may be stale → re-fetch remote list (keep dropdown open)
      await loadRemote()
    }
  }

  const canEdit = $derived(me.identified)
</script>

<!-- Outside click closes. The boundary is this root rather than the list below,
     so the trigger counts as inside — otherwise the mousedown that closes and
     the click that reopens would cancel each other out. -->
<div class="relative inline-block" use:onOutsideClick={{ handler: () => (open = false), enabled: open }}>
  <button
    type="button"
    onclick={toggle}
    data-testid="status-transition"
    class="group inline-flex items-center gap-1.5 rounded-md bg-bg-elevated px-2 py-0.5 text-micro font-medium text-text-secondary transition-colors hover:bg-bg-hover"
    aria-haspopup="listbox"
    aria-expanded={open}
    title={canEdit ? t('write.changeStatus') : issue.status}
  >
    <span class="h-1.5 w-1.5 rounded-full {dotClass}"></span>
    {issue.status}
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
      use:onEscape={() => (open = false)}
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-52 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
      role="listbox"
    >
      {#if loading}
        <div class="px-3 py-2 text-[12px] text-text-muted">{t('write.loadingTransitions')}</div>
      {:else if loadError}
        <div class="px-3 py-2 text-[12px] text-status-reopen">{loadError}</div>
      {:else if sorted.length === 0}
        <div class="px-3 py-2 text-[12px] text-text-muted">{t('write.noTransitions')}</div>
      {:else}
        {#each sorted as t (t.id)}
          <button
            type="button"
            role="option"
            aria-selected="false"
            onclick={() => pick(t)}
            disabled={busyId !== null}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none disabled:opacity-50"
          >
            <span class="h-1.5 w-1.5 flex-none rounded-full {catDot(t.to_category)}"></span>
            <span class="min-w-0 flex-1 truncate">{t.name}</span>
            {#if busyId === t.id}
              <span class="flex-none text-micro text-text-muted">…</span>
            {:else if t.to_status && t.to_status !== t.name}
              <span class="flex-none text-micro text-text-muted">→ {t.to_status}</span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>
