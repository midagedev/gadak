<script lang="ts">
  /*
   * Status chip + transition dropdown (write, local-first).
   *  - Idle looks like the status chip; hover shows edit affordance (chevron).
   *  - Click → render from write.transitionsFor(issue) local map at 0ms; else GET
   *    <key>/transitions/ fallback.
   *  - Sort (quiet suggestions): ① recent transitions (per project) ② workflow forward
   *    (new→inprogress→done) ③ rest. Focus first item → Enter runs immediately.
   *  - On pick: required fields → inline form; else write.transition() optimistic.
   *    Local fail (stale map) → remote refresh.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite, Transition } from '../../lib/types'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { recentOf } from '../../lib/recency'
  import { effectiveCategory } from '../../lib/view-config'
  import { assignedTo } from '../../lib/person-match'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'

  let { issue }: { issue: IssueLite } = $props()

  let open = $state(false)
  let loading = $state(false)
  let remote = $state<Transition[] | null>(null) // fallback GET result
  let source = $state<'local' | 'remote'>('local')
  let loadError = $state<string | null>(null)
  let busyId = $state<string | null>(null)
  let listEl = $state<HTMLDivElement | null>(null)
  let collecting = $state<Transition | null>(null)
  let fieldDraft = $state<Record<string, string>>({})

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

  function closeMenu() {
    open = false
    collecting = null
    fieldDraft = {}
  }

  async function toggle() {
    if (open) {
      closeMenu()
      return
    }
    remote = null
    source = 'local'
    loadError = null
    collecting = null
    fieldDraft = {}
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
    if (!(await write.ensureWritableFor(issue.issue_key))) {
      closeMenu()
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
        closeMenu()
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

  const canSubmit = $derived(
    !!collecting &&
      (collecting.fields ?? []).every((f) => (fieldDraft[f.id] ?? '').trim() !== ''),
  )

  function startCollect(t: Transition) {
    collecting = t
    const next: Record<string, string> = {}
    for (const f of t.fields ?? []) next[f.id] = ''
    fieldDraft = next
    queueMicrotask(() => {
      const el = listEl?.querySelector('select, input') as HTMLElement | null
      el?.focus()
    })
  }

  function buildFields(t: Transition): Record<string, unknown> {
    const out: Record<string, unknown> = {}
    for (const f of t.fields ?? []) {
      const raw = (fieldDraft[f.id] ?? '').trim()
      if ((f.options ?? []).length > 0) out[f.id] = { id: raw }
      else out[f.id] = raw
    }
    return out
  }

  async function send(t: Transition, fields?: Record<string, unknown>) {
    busyId = t.id
    const wasLocal = source === 'local'
    const ok = await write.transition(issue.issue_key, t, fields)
    busyId = null
    if (ok) {
      closeMenu()
      remote = null // status changed — refetch next time
    } else if (wasLocal) {
      collecting = null
      fieldDraft = {}
      // Local map may be stale → re-fetch remote list (keep dropdown open)
      await loadRemote()
    }
  }

  async function pick(t: Transition) {
    if (t.fields && t.fields.length > 0) {
      startCollect(t)
      return
    }
    await send(t)
  }

  async function submitCollected() {
    if (!collecting || !canSubmit) return
    await send(collecting, buildFields(collecting))
  }

  const canEdit = $derived(me.identified)

  /* ── Coaching, M3 (G6) ──
     A transition into in-progress carries the reader's other in-progress
     count — the fact the decision rests on, as dim tabular meta. No warning
     icon, no confirm: the count never blocks the click. Ownership goes
     through the one rule (person-match.assignedTo: id first, then email);
     anonymous or zero renders nothing, because a count that cannot see the
     reader would be a wrong count. The current issue is excluded — the
     question is what else is already on the plate. */
  const meRef = $derived({ accountId: me.accountId, email: me.email })
  const wipCount = $derived.by(() => {
    if (!me.identified) return 0
    let n = 0
    for (const it of issues.pool.values()) {
      if (it.issue_key === issue.issue_key) continue
      if (effectiveCategory(it) === 'inprogress' && assignedTo(it, meRef)) n++
    }
    return n
  })
  // Composed here, not in the each body below: the loop variable `t` shadows
  // the i18n `t` inside that block.
  const wipLabel = $derived(wipCount > 0 ? t('detail.wipCount', { n: wipCount }) : '')
  const wipWhy = t('detail.wipCountWhy')

  /* Coaching, M2 receive end: CommentList's "Move to done" asks this menu to
     open (write.requestTransitionMenu — same nonce bridge as replyRequest).
     Consumed here, never left set, so a remount cannot auto-open on a stale
     request; the pick itself still goes through the menu's own write path. */
  let lastMenuNonce = 0
  $effect(() => {
    const req = write.transitionMenuRequest
    if (!req || req.key !== issue.issue_key || req.n === lastMenuNonce) return
    lastMenuNonce = req.n
    write.transitionMenuRequest = null
    if (!open) void toggle()
  })
</script>

<!-- Outside click closes. The boundary is this root rather than the list below,
     so the trigger counts as inside — otherwise the mousedown that closes and
     the click that reopens would cancel each other out. -->
<div class="relative inline-block" use:onOutsideClick={{ handler: closeMenu, enabled: open }}>
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
      use:onEscape={closeMenu}
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-52 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
      role="listbox"
    >
      {#if loading}
        <div class="px-3 py-2 text-micro text-text-muted">{t('common.loading')}</div>
      {:else if loadError}
        <div class="px-3 py-2 text-body text-status-reopen">{loadError}</div>
      {:else if collecting}
        <div class="px-2 py-1.5" data-testid="transition-required-fields">
          <div class="px-1 pb-1.5 text-body text-text-secondary">{collecting.name}</div>
          {#each collecting.fields ?? [] as f (f.id)}
            <label class="mb-1.5 flex flex-col gap-0.5 px-1">
              <span class="text-micro text-text-muted">{f.name}</span>
              {#if (f.options ?? []).length > 0}
                <select
                  class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-body text-text-primary focus:border-accent focus:outline-none"
                  bind:value={fieldDraft[f.id]}
                  aria-label={f.name}
                  disabled={busyId !== null}
                >
                  <option value=""></option>
                  {#each f.options as o (o.id)}
                    <option value={o.id}>{o.value}</option>
                  {/each}
                </select>
              {:else}
                <input
                  type="text"
                  class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-body text-text-primary focus:border-accent focus:outline-none"
                  bind:value={fieldDraft[f.id]}
                  aria-label={f.name}
                  disabled={busyId !== null}
                />
              {/if}
            </label>
          {/each}
          <div class="flex justify-end px-1 pt-0.5">
            <button
              type="button"
              onclick={submitCollected}
              disabled={!canSubmit || busyId !== null}
              class="rounded bg-accent px-2 py-1 text-micro font-medium text-white hover:opacity-90 disabled:opacity-50"
            >
              {t('common.apply')}
            </button>
          </div>
        </div>
      {:else if sorted.length === 0}
        <div class="px-3 py-2 text-micro text-text-muted">{t('write.noTransitions')}</div>
      {:else}
        {#each sorted as t (t.id)}
          <button
            type="button"
            role="option"
            aria-selected="false"
            onclick={() => pick(t)}
            disabled={busyId !== null}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:text-text-primary focus:outline-none disabled:opacity-50"
          >
            <span class="h-1.5 w-1.5 flex-none rounded-full {catDot(t.to_category)}"></span>
            <span class="min-w-0 flex-1 truncate">{t.name}</span>
            {#if busyId === t.id}
              <span class="flex-none text-micro text-text-muted">…</span>
            {:else if t.to_status && t.to_status !== t.name}
              <span class="flex-none text-micro text-text-muted">→ {t.to_status}</span>
            {/if}
            {#if effectiveCategory(t.to_category) === 'inprogress' && wipCount > 0}
              <span
                class="flex-none text-micro text-text-muted tabular-nums"
                data-testid="transition-wip-count"
                title={wipWhy}
              >
                {wipLabel}
              </span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>
