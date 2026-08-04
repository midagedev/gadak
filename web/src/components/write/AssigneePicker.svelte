<script lang="ts">
  /*
   * 담당자 표시 + 지정 팝오버 (쓰기, 로컬 우선 + 개인화 정렬).
   *  - 평시엔 "담당 [아바타] 이름"(미할당이면 회색). hover 시 편집 어포던스.
   *  - 클릭 → 팝오버. 검색어 없음: 로컬 멤버(jira_account_id 보유)를 개인화 순서로 즉시 표시.
   *      ① 나에게 할당 ② 이슈 보고자 ③ 최근 지정 ④ 같은 파트 ⑤ 나머지(이름순). 그룹 간 미묘한 간격.
   *  - 타이핑 시작하면 전체 검색으로 전환(로컬 필터 + 2자+ GET users/ 병합, 팀 외 사람 폴백).
   *  - 지정: 로컬 멤버는 jira_account_id 로 서버 호출 없이 즉시 → write.assign(). account_id 가
   *    없는 멤버(백엔드 미배포 등)만 이메일/이름으로 users/ 재조회 폴백.
   */
  import type { IssueLite, JiraUser, Member } from '../../lib/types'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { recentOf } from '../../lib/recency'
  import Avatar from '../detail/Avatar.svelte'

  let { issue }: { issue: IssueLite } = $props()

  interface Cand {
    key: string
    account_id: string | null
    display_name: string
    email: string | null
    member?: Member
    avatar_url?: string | null
    label?: string // "나에게 할당" 등 특수 라벨
  }

  let open = $state(false)
  let query = $state('')
  let serverUsers = $state<JiraUser[]>([])
  let searching = $state(false)
  let busy = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)
  let inputEl: HTMLInputElement | null = $state(null)

  const assigneeMember = $derived(issues.memberOf(issue.assignee_email))
  const hasAssignee = $derived(Boolean(issue.assignee || issue.assignee_email))

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
    (a.display_name || a.name).localeCompare(b.display_name || b.name, 'ko')

  const meMember = $derived(me.authed && me.email ? issues.members.get(me.email) : undefined)

  /** jira_account_id → Member (최근 지정 복원용). */
  const memberByAccount = $derived.by(() => {
    const map = new Map<string, Member>()
    for (const m of issues.members.values()) if (m.jira_account_id) map.set(m.jira_account_id, m)
    return map
  })

  /** 지정 가능(퇴사자 제외 + account_id 보유) 멤버. */
  const assignableMembers = $derived.by<Member[]>(() =>
    [...issues.members.values()].filter((m) => m.status !== 'RESIGN' && m.jira_account_id),
  )

  /** 개인화 그룹(검색어 없을 때). 그룹 간 미묘한 간격으로 "조용한 제안". */
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
    const g1 = take([meMember], '나에게 할당')
    const g2 = take([issue.reporter_email ? issues.members.get(issue.reporter_email) : undefined])
    const g3 = take(recentOf('assignee').map((acc) => memberByAccount.get(acc)))
    const g4 = take(
      assignableMembers.filter((m) => issue.d1_group && m.group === issue.d1_group).sort(byName),
    )
    const g5 = take([...assignableMembers].sort(byName))
    return [g1, g2, g3, g4, g5].filter((g) => g.length)
  })

  /** 검색 모드(타이핑 시) 평면 후보: 로컬 필터 + 서버 폴백. */
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

  let debounce: ReturnType<typeof setTimeout> | null = null
  $effect(() => {
    const q = query.trim()
    if (debounce) clearTimeout(debounce)
    if (q.length < 2) {
      serverUsers = []
      return
    }
    debounce = setTimeout(() => void runSearch(q), 250)
  })

  async function runSearch(q: string) {
    searching = true
    try {
      const res = await api.searchUsers(q)
      serverUsers = res.users
    } catch (e) {
      if (e instanceof ApiError && e.code === 'credential_required') {
        open = false
        write.openSettings()
      }
      serverUsers = []
    } finally {
      searching = false
    }
  }

  async function openPicker() {
    if (!(await write.ensureWritable())) return
    open = true
    query = ''
    serverUsers = []
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
    // account_id 없는 로컬 멤버 → 이메일/이름으로 해석 후 지정(폴백)
    busy = true
    try {
      const res = await api.searchUsers(c.email || c.display_name)
      const match =
        res.users.find((u) => u.email && c.email && u.email.toLowerCase() === c.email.toLowerCase()) ??
        res.users[0]
      if (!match) {
        write.toast('Jira 사용자를 찾지 못했습니다.', 'error')
        busy = false
        return
      }
      busy = false
      return doAssign(match)
    } catch {
      write.toast('담당자 지정에 실패했습니다.', 'error')
      busy = false
    }
  }

  $effect(() => {
    if (!open) return
    function onDown(e: MouseEvent) {
      if (rootEl && !rootEl.contains(e.target as Node)) open = false
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') open = false
    }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  })

  const isSearching = $derived(query.trim().length > 0)
</script>

{#snippet candRow(c: Cand)}
  <button
    type="button"
    onclick={() => pickCand(c)}
    disabled={busy}
    class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
  >
    {#if c.member}
      <Avatar member={c.member} name={c.display_name} email={c.email} size={16} />
    {:else if c.avatar_url}
      <img src={c.avatar_url} alt={c.display_name} class="h-4 w-4 flex-none rounded-full object-cover" loading="lazy" />
    {:else}
      <span class="flex h-4 w-4 flex-none items-center justify-center rounded-full bg-bg-active text-[9px] text-text-secondary">{c.display_name.slice(0, 1)}</span>
    {/if}
    <span class="min-w-0 flex-1 truncate {c.label ? 'text-text-primary' : ''}">{c.label ?? c.display_name}</span>
    {#if c.email}<span class="flex-none text-[10px] text-text-muted">{c.email.split('@')[0]}</span>{/if}
  </button>
{/snippet}

<div class="relative flex items-center gap-1.5" bind:this={rootEl}>
  <span class="w-12 flex-none text-text-muted">담당</span>
  <button
    type="button"
    onclick={openPicker}
    class="group flex items-center gap-1.5 rounded-md px-1 py-0.5 text-left transition-colors hover:bg-bg-hover"
    title={me.authed ? '담당자 변경' : (issue.assignee ?? '미할당')}
    disabled={busy}
  >
    {#if hasAssignee}
      <Avatar member={assigneeMember} name={issue.assignee} email={issue.assignee_email} size={16} />
      <span class="text-text-secondary">{issue.assignee ?? issue.assignee_email}</span>
    {:else}
      <span class="text-text-muted italic">미할당</span>
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
      class="anim-enter absolute left-12 top-full z-30 mt-1 w-64 rounded-md border border-border-subtle bg-bg-elevated shadow-xl"
      role="dialog"
      aria-label="담당자 선택"
    >
      <div class="border-b border-border-subtle p-2">
        <input
          bind:this={inputEl}
          bind:value={query}
          type="text"
          placeholder="이름 또는 이메일 검색"
          class="w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent"
        />
      </div>
      <div class="max-h-72 overflow-y-auto py-1">
        <!-- 미할당 -->
        <button
          type="button"
          onclick={() => doAssign(null)}
          disabled={busy}
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
        >
          <span class="flex h-4 w-4 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-[9px]">–</span>
          미할당
        </button>

        {#if isSearching}
          <!-- 검색 모드: 평면 목록 -->
          {#each typed as c (c.key)}
            {@render candRow(c)}
          {/each}
          {#if searching}
            <div class="px-3 py-1.5 text-[11px] text-text-muted">검색 중…</div>
          {:else if typed.length === 0}
            <div class="px-3 py-1.5 text-[11px] text-text-muted">결과 없음</div>
          {/if}
        {:else}
          <!-- 개인화 그룹: 그룹 간 미묘한 간격 -->
          {#each groups as g, gi (gi)}
            <div class={gi > 0 ? 'mt-1 border-t border-border-subtle pt-1' : ''}>
              {#each g as c (c.key)}
                {@render candRow(c)}
              {/each}
            </div>
          {/each}
          {#if groups.length === 0}
            <div class="px-3 py-1.5 text-[11px] text-text-muted">이름을 입력해 검색하세요</div>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</div>
