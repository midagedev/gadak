<script lang="ts">
  /*
   * 일괄 작업 바 (bulk). 1개 이상 선택 시 리스트 상단(필터바 아래)에 나타난다.
   *  "N개 선택 · [상태 변경] [담당자 변경] [선택 해제]"
   *
   *  - 상태 변경: 선택 이슈들의 transitionsFor(로컬 맵)를 to_status 이름 기준으로 union
   *      → 드롭다운. 선택하면 각 이슈별로 해당 to_status 로 가는 transition 을 resolve 해 실행,
   *      없는 이슈는 skip(로컬 맵이 비면 원격 GET 폴백으로 한 번 더 시도).
   *  - 담당자 변경: 지정 가능 멤버(최근 선택 우선) 드롭다운. per-issue write.assign.
   *  - 실행: write 스토어의 per-issue 옵티미스틱 메서드를 동시성 3으로 배치. 진행 카운터 표시,
   *      실패 이슈 롤백은 기존 옵티미스틱 패턴이 처리. 끝나면 성공/실패/건너뜀 요약 토스트.
   *
   * 자격증명/로그인 게이트는 write.ensureWritable() 한 번으로 처리(기존과 동일한 안내).
   */
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

  // ── 배치 진행 상태 ──
  let running = $state(false)
  let progress = $state<{ done: number; total: number }>({ done: 0, total: 0 })

  /** 현재 카테고리 순위(전진 방향 우선 정렬용). */
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
    count: number // 이 상태로 갈 수 있는 선택 이슈 수(로컬 맵 기준)
  }

  /** 선택 이슈들의 전환을 to_status 이름 기준으로 union. */
  const statusOptions = $derived.by<StatusOption[]>(() => {
    const map = new Map<string, StatusOption>()
    for (const key of bulk.selected) {
      const issue = issues.pool.get(key)
      if (!issue) continue
      const list = write.transitionsFor(issue)
      if (!list) continue
      const seen = new Set<string>()
      for (const t of list) {
        if (seen.has(t.to_status)) continue // 한 이슈 내 같은 목적 상태 중복 방지
        seen.add(t.to_status)
        const e = map.get(t.to_status)
        if (e) e.count++
        else map.set(t.to_status, { to_status: t.to_status, to_category: t.to_category, count: 1 })
      }
    }
    return [...map.values()].sort((a, b) => {
      const fa = jiraRank(a.to_category)
      const fb = jiraRank(b.to_category)
      if (fa !== fb) return fa - fb // 신규→진행→완료 순
      return b.count - a.count // 더 많이 적용되는 상태 우선
    })
  })

  // ── 담당자 후보(최근 선택 우선) ──
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
      return (a.display_name || a.name).localeCompare(b.display_name || b.name, 'ko')
    })
  })

  function closeMenu() {
    menu = null
    assigneeQuery = ''
  }

  function toggleMenu(m: Exclude<Menu, null>) {
    menu = menu === m ? null : m
  }

  /** 동시성 3 배치. fn 은 각 키의 결과를 자체 집계. */
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

  /** 결과 요약 토스트 + 성공 시 선택 해제. */
  function finish(ok: number, fail: number, skip: number) {
    const parts = [`성공 ${ok}`, `실패 ${fail}`]
    if (skip) parts.push(`건너뜀 ${skip}`)
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
        // 로컬 맵이 비면 원격으로 한 번 더 확인(새 API 아님).
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

  // 바깥 클릭 / Escape: 메뉴가 열려 있으면 메뉴만 닫고, 아니면 선택 해제.
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
    <span class="flex-none font-medium text-text-primary">{bulk.count}개 선택</span>
    <span class="flex-none text-text-muted">·</span>

    <!-- 상태 변경 -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('status')}
        disabled={running}
        class="rounded-md border border-border-subtle px-2.5 py-1 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        상태 변경
      </button>
      {#if menu === 'status'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-56 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl"
          role="listbox"
        >
          {#if statusOptions.length === 0}
            <div class="px-3 py-2 text-[12px] text-text-muted">공통 전환이 없습니다.</div>
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

    <!-- 담당자 변경 -->
    <div class="relative flex-none">
      <button
        type="button"
        onclick={() => toggleMenu('assignee')}
        disabled={running}
        class="rounded-md border border-border-subtle px-2.5 py-1 text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
      >
        담당자 변경
      </button>
      {#if menu === 'assignee'}
        <div
          class="anim-enter absolute left-0 top-full z-30 mt-1 w-64 rounded-md border border-border-subtle bg-bg-elevated shadow-xl"
          role="dialog"
          aria-label="담당자 선택"
        >
          <div class="border-b border-border-subtle p-2">
            <!-- svelte-ignore a11y_autofocus -->
            <input
              bind:value={assigneeQuery}
              type="text"
              placeholder="이름 또는 이메일 검색"
              autofocus
              class="w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent"
            />
          </div>
          <div class="max-h-72 overflow-y-auto py-1">
            <!-- 미할당 -->
            <button
              type="button"
              onclick={() => runAssign(null)}
              class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            >
              <span
                class="flex h-4 w-4 flex-none items-center justify-center rounded-full border border-dashed border-border-strong text-[9px]"
                >–</span
              >
              미할당
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
              <div class="px-3 py-1.5 text-[11px] text-text-muted">결과 없음</div>
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- 선택 해제 -->
    <button
      type="button"
      onclick={() => bulk.clear()}
      disabled={running}
      class="flex-none rounded-md px-2 py-1 text-text-muted transition-colors hover:text-text-primary disabled:opacity-50"
    >
      선택 해제
    </button>

    <!-- 진행 표시(카운터 — 무한 애니메이션 금지) -->
    {#if running}
      <span class="ml-auto flex flex-none items-center gap-1.5 text-[11px] text-text-secondary">
        <span class="text-text-muted">처리 중</span>
        {progress.done}/{progress.total}
      </span>
    {/if}
  </div>
{/if}
