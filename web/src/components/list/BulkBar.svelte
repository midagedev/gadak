<script lang="ts">
  /*
   * Bulk action bar. Appears above the list (under the filter bar) when ≥1 selected.
   *  "N selected · [change status] [change assignee] [change labels] [deselect]"
   *
   *  - Status: union selected issues' transitionsFor (local map) by to_status name →
   *      dropdown. On pick, resolve each issue's transition to that to_status; skip
   *      missing (if local map empty, one remote GET fallback).
   *  - Assignee: assignable members (recent first) dropdown; per-issue write.assign.
   *  - Labels: add a label to each issue that does not have it; remove one that
   *      already sits on the selection. Same PUT as the detail editor.
   *  - Run: write store's per-issue optimistic methods, concurrency 3, progress counter.
   *      Failed issues roll back via existing optimistic pattern. End toast: ok/fail/skip.
   *
   * Credential/login gate is a single write.ensureWritable() (same guidance as elsewhere).
   *
   * Which popover is open lives in the triage store, not here: `s`/`a`/`l` open the
   * same menus from the keyboard, and a component-local flag would leave them
   * with nothing to set.
   */
  import { t, collator } from '../../lib/i18n'
  import type { IssueLite, JiraUser, Member, Transition } from '../../lib/types'
  import * as api from '../../lib/api'
  import { bulk } from '../../stores/bulk.svelte'
  import { triage, type TriageMenu } from '../../stores/triage.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { recentOf } from '../../lib/recency'
  import { effectiveCategory } from '../../lib/view-config'
  import { onOutsideClick } from '../../lib/dom-actions'
  import Avatar from './Avatar.svelte'

  const menu = $derived(triage.menu)
  let assigneeQuery = $state('')
  let labelQuery = $state('')

  // ── Batch progress ──
  let running = $state(false)
  let progress = $state<{ done: number; total: number }>({ done: 0, total: 0 })

  /** Category rank (forward-direction sort priority), via the category owner. */
  function jiraRank(key: string): number {
    const c = effectiveCategory(key)
    return c === 'new' ? 0 : c === 'done' ? 2 : 1
  }
  /** Jira statusCategory key → dot color class (via the single category owner). */
  function catDot(key: string): string {
    const c = effectiveCategory(key)
    return c === 'done' ? 'bg-status-done' : c === 'new' ? 'bg-status-new' : 'bg-status-inprogress'
  }

  interface StatusOption {
    to_status: string
    to_category: string
    count: number // selected issues that can reach this status (local map)
  }

  /** Union selected issues' transitions by to_status name. */
  const statusOptions = $derived.by<StatusOption[]>(() => {
    const map = new Map<string, StatusOption>()
    for (const key of bulk.selected) {
      const issue = issues.pool.get(key)
      if (!issue) continue
      const list = write.transitionsFor(issue)
      if (!list) continue
      const seen = new Set<string>()
      for (const t of list) {
        if (seen.has(t.to_status)) continue // dedupe same target status within one issue
        seen.add(t.to_status)
        const e = map.get(t.to_status)
        if (e) e.count++
        else map.set(t.to_status, { to_status: t.to_status, to_category: t.to_category, count: 1 })
      }
    }
    return [...map.values()].sort((a, b) => {
      const fa = jiraRank(a.to_category)
      const fb = jiraRank(b.to_category)
      if (fa !== fb) return fa - fb // new → inprogress → done
      return b.count - a.count // statuses that apply to more issues first
    })
  })

  // ── Assignee candidates (recent first) ──
  const assignable = $derived.by<Member[]>(() =>
    [...issues.members.values()].filter((m) => m.status !== 'RESIGN' && m.jira_account_id),
  )

  const assigneeCands = $derived.by<Member[]>(() => {
    const q = assigneeQuery.trim().toLowerCase()
    const recent = recentOf('assignee')
    const rank = (m: Member) => {
      const i = recent.indexOf(m.jira_account_id ?? '')
      return i === -1 ? Infinity : i
    }
    let list = assignable
    if (q)
      list = list.filter((m) =>
        `${m.name} ${m.display_name ?? ''} ${m.email}`.toLowerCase().includes(q),
      )
    return [...list].sort((a, b) => {
      const ra = rank(a)
      const rb = rank(b)
      if (ra !== rb) return ra - rb
      return collator().compare(a.display_name || a.name, b.display_name || b.name)
    })
  })

  function closeMenu() {
    triage.closeMenu()
    assigneeQuery = ''
    labelQuery = ''
  }

  function normalizeLabel(raw: string): string {
    return raw.trim().replace(/\s+/g, '-')
  }

  const selectedIssues = $derived.by<IssueLite[]>(() => {
    const out: IssueLite[] = []
    for (const key of bulk.selected) {
      const issue = issues.pool.get(key)
      if (issue) out.push(issue)
    }
    return out
  })

  /** Labels on the selection, most-shared first. Click removes from those that have it. */
  const onSelection = $derived.by<{ label: string; count: number }[]>(() => {
    const counts = new Map<string, number>()
    for (const issue of selectedIssues) {
      for (const l of issue.labels) counts.set(l, (counts.get(l) ?? 0) + 1)
    }
    return [...counts.entries()]
      .map(([label, count]) => ({ label, count }))
      .sort((a, b) => b.count - a.count || collator().compare(a.label, b.label))
  })

  const labelFreq = $derived.by(() => {
    const projects = new Set(selectedIssues.map((it) => write.projectOf(it)))
    const freq = new Map<string, number>()
    for (const it of issues.allIssues) {
      if (!projects.has(write.projectOf(it))) continue
      for (const l of it.labels) freq.set(l, (freq.get(l) ?? 0) + 1)
    }
    return freq
  })

  const typedLabel = $derived(normalizeLabel(labelQuery))

  const labelSuggestions = $derived.by<string[]>(() => {
    const q = typedLabel.toLowerCase()
    const n = selectedIssues.length
    const onAll = new Set(onSelection.filter((x) => x.count === n).map((x) => x.label))
    const recent = recentOf('label')
    const all = new Set<string>([...recent, ...labelFreq.keys()])
    const arr = [...all].filter((l) => !onAll.has(l) && (!q || l.toLowerCase().includes(q)))
    arr.sort((a, b) => {
      const ra = recent.indexOf(a)
      const rb = recent.indexOf(b)
      const rra = ra === -1 ? Infinity : ra
      const rrb = rb === -1 ? Infinity : rb
      if (rra !== rrb) return rra - rrb
      return (labelFreq.get(b) ?? 0) - (labelFreq.get(a) ?? 0)
    })
    return arr.slice(0, 8)
  })

  const canCreateLabel = $derived(
    Boolean(typedLabel && !selectedIssues.every((it) => it.labels.includes(typedLabel))),
  )

  function toggleMenu(m: TriageMenu) {
    if (menu === m) closeMenu()
    else triage.menu = m
  }

  // The bar is the menu's anchor, so an emptied selection takes the menu with it.
  $effect(() => {
    if (!bulk.active && triage.menu) triage.closeMenu()
  })

  /** Concurrency-3 batch. fn tallies each key's outcome itself. */
  async function runBatch(keys: string[], fn: (key: string) => Promise<void>): Promise<void> {
    running = true
    progress = { done: 0, total: keys.length }
    let idx = 0
    const worker = async () => {
      while (idx < keys.length) {
        const k = keys[idx++]
        await fn(k)
        progress = { ...progress, done: progress.done + 1 }
      }
    }
    try {
      await Promise.all([worker(), worker(), worker()])
    } finally {
      running = false
    }
  }

  /** Summary toast; clear selection on full success. */
  function finish(ok: number, fail: number, skip: number) {
    const parts = [t('bulk.resultOk', { n: ok }), t('bulk.resultFail', { n: fail })]
    if (skip) parts.push(t('bulk.resultSkip', { n: skip }))
    write.toast(parts.join(' · '), fail > 0 ? 'error' : 'success')
    if (fail === 0) bulk.clear()
  }

  async function runStatus(opt: StatusOption) {
    closeMenu()
    if (!(await write.ensureWritable())) return
    const keys = bulk.keys()
    let ok = 0
    let fail = 0
    let skip = 0
    await runBatch(keys, async (key) => {
      const issue = issues.pool.get(key)
      if (!issue) {
        skip++
        return
      }
      let list: Transition[] | null = write.transitionsFor(issue)
      if (!list) {
        // Empty local map → one remote check (not a new API).
        try {
          list = (await api.getTransitions(key)).transitions
        } catch {
          list = []
        }
      }
      const t = list?.find((x) => x.to_status === opt.to_status)
      if (!t) {
        skip++
        return
      }
      const done = await write.transition(key, t)
      if (done) ok++
      else fail++
    })
    finish(ok, fail, skip)
  }

  async function runAssign(m: Member | null) {
    closeMenu()
    if (!(await write.ensureWritable())) return
    const user: JiraUser | null = m
      ? {
          account_id: m.jira_account_id!,
          display_name: m.display_name || m.name,
          email: m.email,
          avatar_url: '',
          active: true,
        }
      : null
    const keys = bulk.keys()
    let ok = 0
    let fail = 0
    await runBatch(keys, async (key) => {
      const done = await write.assign(key, user)
      if (done) ok++
      else fail++
    })
    finish(ok, fail, 0)
  }

  async function runAddLabel(raw: string) {
    const v = normalizeLabel(raw)
    if (!v) return
    closeMenu()
    if (!(await write.ensureWritable())) return
    const keys = bulk.keys()
    let ok = 0
    let fail = 0
    let skip = 0
    await runBatch(keys, async (key) => {
      const issue = issues.pool.get(key)
      if (!issue) {
        skip++
        return
      }
      if (issue.labels.includes(v)) {
        skip++
        return
      }
      const done = await write.setLabels(key, [...issue.labels, v])
      if (done) ok++
      else fail++
    })
    finish(ok, fail, skip)
  }

  async function runRemoveLabel(label: string) {
    closeMenu()
    if (!(await write.ensureWritable())) return
    const keys = bulk.keys()
    let ok = 0
    let fail = 0
    let skip = 0
    await runBatch(keys, async (key) => {
      const issue = issues.pool.get(key)
      if (!issue) {
        skip++
        return
      }
      if (!issue.labels.includes(label)) {
        skip++
        return
      }
      const done = await write.setLabels(
        key,
        issue.labels.filter((x) => x !== label),
      )
      if (done) ok++
      else fail++
    })
    finish(ok, fail, skip)
  }

  function onLabelKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      void runAddLabel(labelSuggestions[0] ?? typedLabel)
    }
  }

  function onLabelTyped(e: Event) {
    const el = e.currentTarget as HTMLInputElement
    const next = el.value.replace(/\s+/g, '-')
    if (next !== el.value) el.value = next
    labelQuery = next
  }

  // Escape closes an open menu. Escape with no menu open is the shell's (it drops
  // the selection) — handling it here too would clear the selection in the same
  // keystroke that closed the popover. This one stays on window because the
  // assignee search box has focus while its menu is open.
  //
  // It also stays an $effect rather than becoming a use:onEscape on the bar
  // below, and the difference is registration order: DetailPanel declines an Esc
  // this one has already spent, which only works while this listener is the
  // earlier of the two. An action would bind when the bar appears — after the
  // panel, whenever rows are picked with an issue already open — and the panel
  // would then close on the keystroke that was meant for the popover.
  $effect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape' || !menu) return
      e.preventDefault() // spend the Esc here so the detail panel keeps its own
      closeMenu()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
</script>

{#if bulk.active}
  <!-- Outside click closes an open menu, bound one task late (see the action's
       `defer`): the palette runs its commands on mousedown, so a listener
       attached during that same dispatch would see the opening click as an
       outside click and close the menu at once. -->
  <div
    data-testid="bulk-bar"
    class="anim-enter flex flex-none items-center gap-2 border-b border-border-strong/70 bg-bg-elevated px-4 py-2 text-[12px]"
    use:onOutsideClick={{ handler: closeMenu, enabled: !!menu, defer: true }}
  >
    <span class="flex-none font-medium text-text-primary">{t('list.selectedCount', { n: bulk.count })}</span>
    <span class="flex-none text-text-muted">·</span>

    <!-- {t('bulk.changeStatus')} -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('status')}
        disabled={running}
        class="inline-flex h-control-sm items-center rounded-md border border-border-subtle px-2.5 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        {t('bulk.changeStatus')}
      </button>
      {#if menu === 'status'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-56 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
          role="listbox"
          data-testid="bulk-status-menu"
        >
          {#if statusOptions.length === 0}
            <div class="px-3 py-2 text-[12px] text-text-muted">{t('bulk.noCommonTransitions')}</div>
          {:else}
            {#each statusOptions as opt (opt.to_status)}
              <button
                type="button"
                role="option"
                aria-selected="false"
                onclick={() => runStatus(opt)}
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
              >
                <span class="h-1.5 w-1.5 flex-none rounded-full {catDot(opt.to_category)}"></span>
                <span class="min-w-0 flex-1 truncate">{opt.to_status}</span>
                <span class="flex-none text-micro text-text-muted">{opt.count}</span>
              </button>
            {/each}
          {/if}
        </div>
      {/if}
    </div>

    <!-- {t('bulk.changeAssignee')} -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('assignee')}
        disabled={running}
        class="inline-flex h-control-sm items-center rounded-md border border-border-subtle px-2.5 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        {t('bulk.changeAssignee')}
      </button>
      {#if menu === 'assignee'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-64 rounded-lg border border-border-strong bg-bg-elevated shadow-overlay"
          role="dialog"
          aria-label={t('bulk.pickAssignee')}
          data-testid="bulk-assignee-menu"
        >
          <div class="border-b border-border-subtle p-2">
            <!-- svelte-ignore a11y_autofocus -->
            <input
              bind:value={assigneeQuery}
              type="text"
              placeholder={t('bulk.searchPerson')}
              autofocus
              class="h-control-sm w-full rounded border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent"
            />
          </div>
          <div class="max-h-72 overflow-y-auto py-1">
            <!-- {t('common.unassigned')} -->
            <button
              type="button"
              onclick={() => runAssign(null)}
              class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            >
              <span
                class="flex h-5 w-5 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-micro"
                >–</span
              >
              {t('common.unassigned')}
            </button>
            {#each assigneeCands as m (m.email)}
              <button
                type="button"
                onclick={() => runAssign(m)}
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
              >
                <Avatar name={m.display_name || m.name} email={m.email} size={20} />
                <span class="min-w-0 flex-1 truncate">{m.display_name || m.name}</span>
                <span class="flex-none text-micro text-text-muted">{m.email.split('@')[0]}</span>
              </button>
            {/each}
            {#if assigneeCands.length === 0}
              <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.noResults')}</div>
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- {t('bulk.changeLabels')} -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('labels')}
        disabled={running}
        class="inline-flex h-control-sm items-center rounded-md border border-border-subtle px-2.5 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        {t('bulk.changeLabels')}
      </button>
      {#if menu === 'labels'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-64 rounded-lg border border-border-strong bg-bg-elevated shadow-overlay"
          role="dialog"
          aria-label={t('bulk.pickLabel')}
          data-testid="bulk-labels-menu"
        >
          <div class="border-b border-border-subtle p-2">
            <!-- svelte-ignore a11y_autofocus -->
            <input
              value={labelQuery}
              oninput={onLabelTyped}
              onkeydown={onLabelKeydown}
              type="text"
              placeholder={t('bulk.searchLabel')}
              autofocus
              class="h-control-sm w-full rounded border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent"
            />
          </div>
          <div class="max-h-72 overflow-y-auto py-1">
            {#if onSelection.length > 0}
              <div class="px-3 py-1 text-micro text-text-muted">{t('bulk.onSelection')}</div>
              {#each onSelection as item (item.label)}
                <button
                  type="button"
                  onclick={() => runRemoveLabel(item.label)}
                  class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                  title={t('write.removeLabel', { label: item.label })}
                >
                  <span
                    class="flex h-5 w-5 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-micro text-text-muted"
                    >–</span
                  >
                  <span class="min-w-0 flex-1 truncate">{item.label}</span>
                  <span class="flex-none text-micro text-text-muted">{item.count}</span>
                </button>
              {/each}
              <div class="my-1 border-t border-border-subtle"></div>
            {/if}
            {#each labelSuggestions as l (l)}
              <button
                type="button"
                onclick={() => runAddLabel(l)}
                class="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
              >
                <span class="min-w-0 flex-1 truncate">{l}</span>
                {#if labelFreq.get(l)}
                  <span class="flex-none text-micro text-text-muted">{labelFreq.get(l)}</span>
                {/if}
              </button>
            {/each}
            {#if canCreateLabel && !labelSuggestions.includes(typedLabel)}
              <button
                type="button"
                onclick={() => runAddLabel(typedLabel)}
                class="flex w-full items-center px-3 py-1.5 text-left text-[12px] text-text-primary transition-colors hover:bg-bg-hover"
              >
                {t('write.addLabelNamed', { label: typedLabel })}
              </button>
            {/if}
            {#if labelSuggestions.length === 0 && !canCreateLabel}
              <div class="px-3 py-1.5 text-micro text-text-muted">{t('bulk.typeALabel')}</div>
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- {t('common.deselect')} -->
    <button
      type="button"
      onclick={() => bulk.clear()}
      disabled={running}
      class="inline-flex h-control-sm flex-none items-center rounded-md px-2 text-text-muted transition-colors hover:text-text-primary disabled:opacity-50"
    >
      {t('common.deselect')}
    </button>

    <!-- Progress (counter — no infinite spinner) -->
    {#if running}
      <span class="ml-auto flex flex-none items-center gap-1.5 text-micro text-text-secondary">
        <span class="text-text-muted">{t('common.processing')}</span>
        {progress.done}/{progress.total}
      </span>
    {/if}
  </div>
{/if}
