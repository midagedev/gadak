<script lang="ts">
  /*
   * Bulk action bar. Appears above the list (under the filter bar) when ≥1 selected.
   *  "N selected · [change status] [change assignee] [deselect]"
   *
   *  - Status: union selected issues' transitionsFor (local map) by to_status name →
   *      dropdown. On pick, resolve each issue's transition to that to_status; skip
   *      missing (if local map empty, one remote GET fallback).
   *  - Assignee: assignable members (recent first) dropdown; per-issue write.assign.
   *  - Run: write store's per-issue optimistic methods, concurrency 3, progress counter.
   *      Failed issues roll back via existing optimistic pattern. End toast: ok/fail/skip.
   *
   * Credential/login gate is a single write.ensureWritable() (same guidance as elsewhere).
   */
  import { t, collator } from '../../lib/i18n'
  import type { JiraUser, Member, Transition } from '../../lib/types'
  import * as api from '../../lib/api'
  import { bulk } from '../../stores/bulk.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { recentOf } from '../../lib/recency'
  import Avatar from '../detail/Avatar.svelte'

  type Menu = 'status' | 'assignee' | null
  let menu = $state<Menu>(null)
  let assigneeQuery = $state('')
  let rootEl = $state<HTMLDivElement | null>(null)

  // ── Batch progress ──
  let running = $state(false)
  let progress = $state<{ done: number; total: number }>({ done: 0, total: 0 })

  /** Category rank (forward-direction sort priority). */
  function jiraRank(key: string): number {
    const c = (key || '').toLowerCase()
    if (c === 'new') return 0
    if (c === 'done') return 2
    return 1
  }
  function catDot(key: string): string {
    const c = (key || '').toLowerCase()
    if (c === 'done') return 'bg-status-done'
    if (c === 'new') return 'bg-status-new'
    return 'bg-status-inprogress'
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
    menu = null
    assigneeQuery = ''
  }

  function toggleMenu(m: Exclude<Menu, null>) {
    menu = menu === m ? null : m
  }

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

  // Outside click / Escape: close menu if open, else deselect.
  $effect(() => {
    function onDown(e: MouseEvent) {
      if (menu && rootEl && !rootEl.contains(e.target as Node)) closeMenu()
    }
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if (menu) closeMenu()
      else if (bulk.active && !running) bulk.clear()
    }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  })
</script>

{#if bulk.active}
  <div
    bind:this={rootEl}
    class="anim-enter flex flex-none items-center gap-2 border-b border-border-strong/70 bg-bg-elevated px-4 py-2 text-[12px]"
  >
    <span class="flex-none font-medium text-text-primary">{t('list.selectedCount', { n: bulk.count })}</span>
    <span class="flex-none text-text-muted">·</span>

    <!-- {t('bulk.changeStatus')} -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('status')}
        disabled={running}
        class="rounded-md border border-border-subtle px-2.5 py-1 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        {t('bulk.changeStatus')}
      </button>
      {#if menu === 'status'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-56 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl"
          role="listbox"
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
                <span class="flex-none text-[11px] text-text-muted">{opt.count}</span>
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
        class="rounded-md border border-border-subtle px-2.5 py-1 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        {t('bulk.changeAssignee')}
      </button>
      {#if menu === 'assignee'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-64 rounded-md border border-border-subtle bg-bg-elevated shadow-xl"
          role="dialog"
          aria-label={t('bulk.pickAssignee')}
        >
          <div class="border-b border-border-subtle p-2">
            <!-- svelte-ignore a11y_autofocus -->
            <input
              bind:value={assigneeQuery}
              type="text"
              placeholder={t('bulk.searchPerson')}
              autofocus
              class="w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent"
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
                class="flex h-4 w-4 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-[9px]"
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
                <Avatar member={m} name={m.display_name || m.name} email={m.email} size={16} />
                <span class="min-w-0 flex-1 truncate">{m.display_name || m.name}</span>
                <span class="flex-none text-[10px] text-text-muted">{m.email.split('@')[0]}</span>
              </button>
            {/each}
            {#if assigneeCands.length === 0}
              <div class="px-3 py-1.5 text-[11px] text-text-muted">{t('common.noResults')}</div>
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
      class="flex-none rounded-md px-2 py-1 text-text-muted transition-colors hover:text-text-primary disabled:opacity-50"
    >
      {t('common.deselect')}
    </button>

    <!-- Progress (counter — no infinite spinner) -->
    {#if running}
      <span class="ml-auto flex flex-none items-center gap-1.5 text-[11px] text-text-secondary">
        <span class="text-text-muted">{t('common.processing')}</span>
        {progress.done}/{progress.total}
      </span>
    {/if}
  </div>
{/if}
