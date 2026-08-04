<script lang="ts">
  /*
   * QA 메타 필드 인라인 에디터 (쓰기, 로컬 우선).
   *  - 4종만 편집 지원: 수정방법(솔루션)·개발검증여부(Pass/Fail)·개발테스트담당자·수정버전.
   *  - 평시엔 IssueFields 의 값 칩과 동일한 모습. hover 시 편집 어포던스(연필/쉐브론).
   *  - 클릭 → write.ensureEditMeta(key)(쓰기 게이트 + editmeta 허용값 로드) → 드롭다운.
   *      · option        : 단일 select(+"없음" 해제).
   *      · version_array : 체크박스 다중선택 + "적용".
   *      · user          : 로컬 멤버(개인화 정렬) + 2자↑ 서버 검색 폴백(+"해제").
   *  - 선택 시 write.setField() 옵티미스틱(표시값 즉시 반영 → 서버 재동기화로 확정).
   * Falls back to read-only when editmeta is missing (not editable / no credential).
   */
  import { t, collator } from '../../lib/i18n'
  import type { EditMetaOption, IssueLite, JiraUser, Member } from '../../lib/types'
  import * as api from '../../lib/api'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import Avatar from './Avatar.svelte'

  type Kind = 'option' | 'user' | 'version_array'

  let {
    issue,
    field,
    kind,
    values,
  }: {
    issue: IssueLite
    field: string
    kind: Kind
    values: string[] // 현재 표시 중인 값(칩) — 읽기 표시용
  } = $props()

  let open = $state(false)
  let busy = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)
  let inputEl = $state<HTMLInputElement | null>(null)

  // user 검색 상태
  let query = $state('')
  let serverUsers = $state<JiraUser[]>([])
  let searching = $state(false)

  // version_array 초안 선택(버전 id 집합) + 필터(버전 옵션이 수백 개라 검색 필수)
  let draft = $state<Set<string>>(new Set())
  let vquery = $state('')

  /** 필터된 버전 옵션 — 선택된 항목은 항상 상위 노출. */
  const versionOptions = $derived.by<EditMetaOption[]>(() => {
    const q = vquery.trim().toLowerCase()
    const filtered = q ? options.filter((o) => o.value.toLowerCase().includes(q)) : options
    return [...filtered].sort((a, b) => {
      const sa = draft.has(a.id) ? 0 : 1
      const sb = draft.has(b.id) ? 0 : 1
      if (sa !== sb) return sa - sb
      return b.value.localeCompare(a.value) // 최신(이름 역순) 우선
    })
  })

  const key = $derived(issue.issue_key)
  const meta = $derived(write.editFieldMeta(key, field))
  const options = $derived<EditMetaOption[]>(meta?.options ?? [])

  function resultClass(value: string): string {
    const v = value.toLowerCase()
    if (v === 'pass') return 'bg-status-done/15 text-status-done'
    if (v === 'fail') return 'bg-status-reopen/15 text-status-reopen'
    return 'bg-bg-elevated text-text-secondary'
  }
  const chipClass = (v: string) =>
    field === 'development_test_result' ? resultClass(v) : 'bg-bg-elevated text-text-secondary'

  /* ── 열기/닫기 ── */

  async function toggle() {
    if (open) {
      open = false
      return
    }
    if (!(await write.ensureEditMeta(key))) return
    // editmeta 로드 후에도 이 필드가 편집 불가면 열지 않음(읽기 전용 폴백).
    if (!write.editFieldMeta(key, field)) return
    query = ''
    vquery = ''
    serverUsers = []
    if (kind === 'version_array') draft = new Set(currentVersionIds())
    open = true
    if (kind === 'user' || kind === 'version_array') queueMicrotask(() => inputEl?.focus())
  }

  /* ── option (단일 select) ── */

  async function pickOption(opt: EditMetaOption | null) {
    busy = true
    const ok = await write.setField(key, field, opt ? opt.id : null, {
      [field]: opt ? opt.value : null,
    } as Partial<IssueLite>)
    busy = false
    if (ok) open = false
  }

  /* ── version_array (다중) ── */

  /** 현재 표시 버전명 → editmeta 옵션 id 로 역매핑. */
  function currentVersionIds(): string[] {
    const byName = new Map(options.map((o) => [o.value, o.id]))
    return (issue.fix_versions ?? []).map((n) => byName.get(n)).filter((x): x is string => !!x)
  }

  function toggleVersion(id: string) {
    const next = new Set(draft)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    draft = next
  }

  async function applyVersions() {
    const ids = [...draft]
    const names = options.filter((o) => draft.has(o.id)).map((o) => o.value)
    busy = true
    const ok = await write.setField(key, 'fix_versions', ids, { fix_versions: names })
    busy = false
    if (ok) open = false
  }

  /* ── user (개발테스트담당자) ── */

  interface Cand {
    account_id: string
    display_name: string
    email: string | null
    member?: Member
    avatar_url?: string | null
    label?: string
  }

  const meMember = $derived(me.identified && me.email ? issues.members.get(me.email) : undefined)
  const reporterMember = $derived(issues.memberOf(issue.reporter_email))

  /** 로컬 멤버(개인화) — 나 → 보고자 → 이름순. jira_account_id 있는 멤버만. */
  const localCands = $derived.by<Cand[]>(() => {
    const withId = [...issues.members.values()].filter((m) => m.jira_account_id)
    const byName = (a: Member, b: Member) =>
      collator().compare(a.display_name || a.name, b.display_name || b.name)
    const seen = new Set<string>()
    const out: Cand[] = []
    const push = (m: Member | undefined, label?: string) => {
      if (!m || !m.jira_account_id || seen.has(m.jira_account_id)) return
      seen.add(m.jira_account_id)
      out.push({
        account_id: m.jira_account_id,
        display_name: m.display_name || m.name,
        email: m.email,
        member: m,
        label,
      })
    }
    push(meMember, t('common.me'))
    push(reporterMember, t('qaEditor.reporter'))
    for (const m of withId.sort(byName)) push(m)
    return out
  })

  /** 검색어 반영 후보(로컬 필터 + 서버 검색 병합). */
  const cands = $derived.by<Cand[]>(() => {
    const q = query.trim().toLowerCase()
    const base = q
      ? localCands.filter(
          (c) =>
            c.display_name.toLowerCase().includes(q) || (c.email ?? '').toLowerCase().includes(q),
        )
      : localCands
    if (!q) return base.slice(0, 40)
    const seen = new Set(base.map((c) => c.account_id))
    const merged = [...base]
    for (const u of serverUsers) {
      if (u.account_id && !seen.has(u.account_id)) {
        merged.push({
          account_id: u.account_id,
          display_name: u.display_name,
          email: u.email || null,
          avatar_url: u.avatar_url,
        })
      }
    }
    return merged.slice(0, 40)
  })

  // 2자↑ 입력 시 서버 사용자 검색(팀 외 사람 폴백).
  $effect(() => {
    const q = query.trim()
    if (q.length < 2) {
      serverUsers = []
      return
    }
    let cancelled = false
    searching = true
    api
      .searchUsers(q)
      .then((res) => {
        if (!cancelled) serverUsers = res.users
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) searching = false
      })
    return () => {
      cancelled = true
    }
  })

  async function pickUser(c: Cand | null) {
    busy = true
    const ok = await write.setField(key, 'development_test_assignee', c ? c.account_id : null, {
      development_test_assignee: c ? c.display_name : null,
      development_test_assignee_email: c ? c.email : null,
    })
    busy = false
    if (ok) open = false
  }

  /* ── 바깥 클릭 / Esc ── */
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

  const canEdit = $derived(me.identified)
</script>

<div class="relative inline-block w-full" bind:this={rootEl}>
  <button
    type="button"
    onclick={toggle}
    class="group flex w-full min-w-0 items-center gap-1 rounded px-1 -mx-1 py-0.5 text-left transition-colors hover:bg-bg-hover"
    aria-haspopup="listbox"
    aria-expanded={open}
    title={canEdit ? t('common.change') : undefined}
  >
    {#if values.length === 0}
      <span class="text-text-muted">{t('qaEditor.none')}</span>
    {:else}
      <span class="flex min-w-0 flex-wrap gap-1">
        {#each values as v (v)}
          <span class="max-w-full break-words rounded px-1.5 py-0.5 {chipClass(v)}">{v}</span>
        {/each}
      </span>
    {/if}
    <svg
      width="9"
      height="9"
      viewBox="0 0 10 10"
      fill="none"
      aria-hidden="true"
      class="ml-auto flex-none text-text-muted opacity-0 transition-opacity group-hover:opacity-100"
    >
      <path
        d="M2.5 4l2.5 2.5L7.5 4"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  </button>

  {#if open}
    <div
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-60 overflow-y-auto rounded-md border border-border-subtle bg-bg-elevated py-1 shadow-xl"
      role="listbox"
    >
      {#if kind === 'option'}
        <button
          type="button"
          role="option"
          aria-selected={values.length === 0}
          onclick={() => pickOption(null)}
          disabled={busy}
          class="flex w-full items-center px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover focus:bg-bg-hover focus:outline-none disabled:opacity-50"
        >
          {t('qaEditor.none')}
        </button>
        {#each options as opt (opt.id)}
          {@const selected = values.includes(opt.value)}
          <button
            type="button"
            role="option"
            aria-selected={selected}
            onclick={() => pickOption(opt)}
            disabled={busy}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors hover:bg-bg-hover focus:bg-bg-hover focus:outline-none disabled:opacity-50 {selected
              ? 'text-text-primary'
              : 'text-text-secondary'}"
          >
            <span class="min-w-0 flex-1 truncate">{opt.value}</span>
            {#if selected}<span class="flex-none text-accent">✓</span>{/if}
          </button>
        {/each}
      {:else if kind === 'version_array'}
        {#if options.length === 0}
          <div class="px-3 py-2 text-[12px] text-text-muted">{t('qaEditor.noVersions')}</div>
        {:else}
          <div class="px-2 pb-1">
            <input
              bind:this={inputEl}
              bind:value={vquery}
              type="text"
              placeholder={t('qaEditor.searchVersion')}
              class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:border-accent focus:outline-none"
            />
          </div>
          <div class="max-h-52 overflow-y-auto">
            {#each versionOptions as opt (opt.id)}
              {@const checked = draft.has(opt.id)}
              <button
                type="button"
                onclick={() => toggleVersion(opt.id)}
                disabled={busy}
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors hover:bg-bg-hover focus:bg-bg-hover focus:outline-none disabled:opacity-50 {checked
                  ? 'text-text-primary'
                  : 'text-text-secondary'}"
              >
                <span
                  class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded-sm border {checked
                    ? 'border-accent bg-accent text-white'
                    : 'border-border-subtle'}"
                >
                  {#if checked}<span class="text-[9px] leading-none">✓</span>{/if}
                </span>
                <span class="min-w-0 flex-1 truncate">{opt.value}</span>
              </button>
            {/each}
            {#if versionOptions.length === 0}
              <div class="px-3 py-1.5 text-[11px] text-text-muted">{t('common.noResults')}</div>
            {/if}
          </div>
          <div class="mt-1 flex items-center justify-end gap-2 border-t border-border-subtle px-2 pt-1.5">
            <button
              type="button"
              onclick={() => (open = false)}
              disabled={busy}
              class="rounded px-2 py-1 text-[11px] text-text-muted hover:bg-bg-hover disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onclick={applyVersions}
              disabled={busy}
              class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-white hover:opacity-90 disabled:opacity-50"
            >
              {t('common.apply')}
            </button>
          </div>
        {/if}
      {:else}
        <!-- user -->
        <div class="px-2 pb-1">
          <input
            bind:this={inputEl}
            bind:value={query}
            type="text"
            placeholder={t('qaEditor.searchPerson')}
            class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:border-accent focus:outline-none"
          />
        </div>
        <button
          type="button"
          onclick={() => pickUser(null)}
          disabled={busy}
          class="flex w-full items-center px-3 py-1.5 text-left text-[12px] text-text-muted transition-colors hover:bg-bg-hover focus:bg-bg-hover focus:outline-none disabled:opacity-50"
        >
          {t('qaEditor.clearAssignee')}
        </button>
        {#each cands as c (c.account_id)}
          <button
            type="button"
            role="option"
            aria-selected={c.email != null && c.email === issue.development_test_assignee_email}
            onclick={() => pickUser(c)}
            disabled={busy}
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary focus:bg-bg-hover focus:outline-none disabled:opacity-50"
          >
            <Avatar member={c.member} name={c.display_name} email={c.email} size={18} />
            <span class="min-w-0 flex-1 truncate">{c.display_name}</span>
            {#if c.label}<span class="flex-none text-[10px] text-text-muted">{c.label}</span>{/if}
          </button>
        {/each}
        {#if searching}
          <div class="px-3 py-1.5 text-[11px] text-text-muted">{t('common.searching')}</div>
        {:else if cands.length === 0}
          <div class="px-3 py-1.5 text-[11px] text-text-muted">{t('common.noResults')}</div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
