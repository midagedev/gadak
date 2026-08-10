<script lang="ts">
  /*
   * Assignee display + assign popover (write, local-first + personalized sort).
   *  - Idle: "Assignee [avatar] name" (gray when unassigned). Edit affordance on hover.
   *  - Click → popover. Empty query: local members (with jira_account_id) in personal order:
   *      ① me ② reporter ③ recent ④ same team ⑤ rest (name). Subtle gaps between groups.
   *  - Typing switches to full search (local filter + ≥2-char GET users/, outside-team fallback).
   *  - Assign: local members with jira_account_id call write.assign() immediately; members
   *    without account_id (backend lag etc.) re-resolve via users/ by email/name.
   */
  import { t, collator } from '../../lib/i18n'
  import type { IssueLite, JiraUser, Member } from '../../lib/types'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { createUserSearch } from '../../lib/user-search.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { recentOf } from '../../lib/recency'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  // The list's Avatar: a person wears the same name-derived color here that
  // they wear in every row behind this popover.
  import Avatar from '../list/Avatar.svelte'

  let { issue }: { issue: IssueLite } = $props()

  interface Cand {
    key: string
    account_id: string | null
    display_name: string
    email: string | null
    member?: Member
    avatar_url?: string | null
    label?: string // special label e.g. "Assign to me"
  }

  let open = $state(false)
  let query = $state('')
  let busy = $state(false)

  const userSearch = createUserSearch(() => query, {
    debounceMs: 250,
    minLength: 2,
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'credential_required') {
        open = false
        write.openSettings()
      }
    },
  })
  // Template keeps `searching` / derived reads of server results.
  const searching = $derived(userSearch.searching)
  const serverUsers = $derived(userSearch.results)
  let inputEl: HTMLInputElement | null = $state(null)

  const hasAssignee = $derived(Boolean(issue.assignee_id || issue.assignee || issue.assignee_email))

  function candOf(m: Member, label?: string): Cand {
    return {
      key: m.jira_account_id ?? m.email,
      account_id: m.jira_account_id ?? null,
      display_name: m.display_name || m.name,
      email: m.email,
      member: m,
      label,
    }
  }

  const byName = (a: Member, b: Member) =>
    collator().compare(a.display_name || a.name, b.display_name || b.name)

  const meMember = $derived(me.identified && me.email ? issues.members.get(me.email) : undefined)

  /** jira_account_id → Member (restore recent assignees). */
  const memberByAccount = $derived.by(() => {
    const map = new Map<string, Member>()
    for (const m of issues.members.values()) if (m.jira_account_id) map.set(m.jira_account_id, m)
    return map
  })

  /** Assignable members (exclude resigned + require account_id). */
  const assignableMembers = $derived.by<Member[]>(() =>
    [...issues.members.values()].filter((m) => m.status !== 'RESIGN' && m.jira_account_id),
  )

  /** Personalized groups (no query). Subtle gaps = "quiet suggestions". */
  const groups = $derived.by<Cand[][]>(() => {
    const seen = new Set<string>()
    const take = (list: (Member | undefined)[], label?: string): Cand[] => {
      const r: Cand[] = []
      for (const m of list) {
        const acc = m?.jira_account_id
        if (!m || !acc || seen.has(acc)) continue
        seen.add(acc)
        r.push(candOf(m, label))
      }
      return r
    }
    const g1 = take([meMember], t('write.assignToMe'))
    const g2 = take([
      issues.memberOfAccountId(issue.reporter_id) ?? issues.memberOf(issue.reporter_email),
    ])
    const g3 = take(recentOf('assignee').map((acc) => memberByAccount.get(acc)))
    const g4 = take(
      assignableMembers.filter((m) => issue.team_group && m.group === issue.team_group).sort(byName),
    )
    const g5 = take([...assignableMembers].sort(byName))
    return [g1, g2, g3, g4, g5].filter((g) => g.length)
  })

  /** Search mode (typing): flat candidates = local filter + server fallback. */
  const typed = $derived.by<Cand[]>(() => {
    const q = query.trim().toLowerCase()
    if (!q) return []
    const seenEmail = new Set<string>()
    const local: Cand[] = []
    for (const m of issues.members.values()) {
      if (m.status === 'RESIGN') continue
      const hay = `${m.name} ${m.display_name ?? ''} ${m.email}`.toLowerCase()
      if (!hay.includes(q)) continue
      seenEmail.add(m.email.toLowerCase())
      local.push(candOf(m))
      if (local.length >= 8) break
    }
    const server: Cand[] = serverUsers
      .filter((u) => !u.email || !seenEmail.has(u.email.toLowerCase()))
      .map((u) => ({
        key: u.account_id,
        account_id: u.account_id,
        display_name: u.display_name,
        email: u.email || null,
        avatar_url: u.avatar_url,
      }))
    return [...local, ...server]
  })

  async function openPicker() {
    if (!(await write.ensureWritable())) return
    open = true
    query = ''
    queueMicrotask(() => inputEl?.focus())
  }

  async function doAssign(user: JiraUser | null) {
    busy = true
    const ok = await write.assign(issue.issue_key, user)
    busy = false
    if (ok) open = false
  }

  async function pickCand(c: Cand) {
    if (c.account_id) {
      return doAssign({
        account_id: c.account_id,
        display_name: c.display_name,
        email: c.email ?? '',
        avatar_url: c.avatar_url ?? '',
        active: true,
      })
    }
    // Local member without account_id → resolve via email/name then assign (fallback)
    busy = true
    try {
      const res = await api.searchUsers(c.email || c.display_name)
      const match =
        res.users.find((u) => u.email && c.email && u.email.toLowerCase() === c.email.toLowerCase()) ??
        res.users[0]
      if (!match) {
        write.toast(t('write.userNotFound'), 'error')
        busy = false
        return
      }
      busy = false
      return doAssign(match)
    } catch {
      write.toast(t('write.assignSpecifyFailed'), 'error')
      busy = false
    }
  }

  const isSearching = $derived(query.trim().length > 0)
</script>

{#snippet candRow(c: Cand)}
  <button
    type="button"
    onclick={() => pickCand(c)}
    disabled={busy}
    class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
  >
    <!-- One 20px circle whichever branch renders: a 16px circle could not hold a
         legible initial, and the three branches have to line up in one column. -->
    {#if c.member}
      <Avatar name={c.display_name} email={c.email} size={20} />
    {:else if c.avatar_url}
      <img src={c.avatar_url} alt={c.display_name} class="h-5 w-5 flex-none rounded-full object-cover" loading="lazy" />
    {:else}
      <span class="flex h-5 w-5 flex-none items-center justify-center rounded-full bg-bg-active text-micro text-text-secondary">{c.display_name.slice(0, 1)}</span>
    {/if}
    <span class="min-w-0 flex-1 truncate {c.label ? 'text-text-primary' : ''}">{c.label ?? c.display_name}</span>
    {#if c.email}<span class="flex-none text-micro text-text-muted">{c.email.split('@')[0]}</span>{/if}
  </button>
{/snippet}

<!-- Outside click closes. The boundary is this root rather than the panel
     below, so the trigger counts as inside — otherwise the mousedown that
     closes and the click that reopens would cancel each other out. -->
<div
  class="relative flex items-center gap-1.5"
  use:onOutsideClick={{ handler: () => (open = false), enabled: open }}
>
  <span class="w-12 flex-none text-text-muted">{t('write.assigneeLabel')}</span>
  <button
    type="button"
    onclick={openPicker}
    data-testid="assignee-picker"
    class="group flex items-center gap-1.5 rounded-md px-1 py-0.5 text-left transition-colors hover:bg-bg-hover"
    title={me.identified ? t('write.changeAssignee') : (issue.assignee ?? t('common.unassigned'))}
    disabled={busy}
  >
    {#if hasAssignee}
      <Avatar name={issue.assignee} email={issue.assignee_email} size={16} />
      <span class="text-text-secondary">{issue.assignee ?? issue.assignee_email}</span>
    {:else}
      <span class="text-text-muted italic">{t('common.unassigned')}</span>
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
      use:onEscape={() => (open = false)}
      class="anim-enter absolute left-12 top-full z-30 mt-1 w-64 rounded-lg border border-border-strong bg-bg-elevated shadow-overlay"
      role="dialog"
      aria-label={t('write.pickAssignee')}
    >
      <div class="border-b border-border-subtle p-2">
        <input
          bind:this={inputEl}
          bind:value={query}
          type="text"
          placeholder={t('write.searchNameEmail')}
          class="h-control-sm w-full rounded border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent"
        />
      </div>
      <div class="max-h-72 overflow-y-auto py-1">
        <!-- Unassign -->
        <button
          type="button"
          onclick={() => doAssign(null)}
          disabled={busy}
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
        >
          <span class="flex h-5 w-5 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-micro">–</span>
          {t('common.unassigned')}
        </button>

        {#if isSearching}
          <!-- Search mode: flat list -->
          {#each typed as c (c.key)}
            {@render candRow(c)}
          {/each}
          {#if searching}
            <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.searching')}</div>
          {:else if typed.length === 0}
            <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.noResults')}</div>
          {/if}
        {:else}
          <!-- Personalized groups: subtle gaps between groups -->
          {#each groups as g, gi (gi)}
            <div class={gi > 0 ? 'mt-1 border-t border-border-subtle pt-1' : ''}>
              {#each g as c (c.key)}
                {@render candRow(c)}
              {/each}
            </div>
          {/each}
          {#if groups.length === 0}
            <div class="px-3 py-1.5 text-micro text-text-muted">{t('write.typeToSearch')}</div>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</div>
