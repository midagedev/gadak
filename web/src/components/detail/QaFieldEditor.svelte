<script lang="ts">
  /*
   * QA meta field inline editor (write, local-first).
   *  - Four fields only: solution, dev verify (Pass/Fail), dev-test assignee, fix versions.
   *  - Idle look matches IssueFields value chips; hover shows edit affordance (pencil/chevron).
   *  - Click → write.ensureEditMeta(key) (write gate + load allowed values) → dropdown.
   *      · option        : single select (+ clear "none").
   *      · version_array : multi checkbox + Apply.
   *      · user          : local members (personalized sort) + ≥2-char server search (+ clear).
   *  - On pick: write.setField() optimistic (UI updates immediately; server resync confirms).
   * Falls back to read-only when editmeta is missing (not editable / no credential).
   */
  import { t, collator } from '../../lib/i18n'
  import type { EditMetaOption, IssueLite, Member } from '../../lib/types'
  import { createUserSearch } from '../../lib/user-search.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  // The list's Avatar: one person, one color, everywhere they appear.
  import Avatar from '../list/Avatar.svelte'
  import Icon from '../ui/Icon.svelte'

  type Kind = 'option' | 'user' | 'version_array' | 'multi_option'

  let {
    issue,
    field,
    kind,
    values,
  }: {
    issue: IssueLite
    field: string
    kind: Kind
    values: string[] // currently shown chip values (read display)
  } = $props()

  let open = $state(false)
  let busy = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)
  let inputEl = $state<HTMLInputElement | null>(null)

  // user search state
  let query = $state('')
  const userSearch = createUserSearch(() => query, {
    debounceMs: 180,
    minLength: 2,
  })
  const serverUsers = $derived(userSearch.results)
  const searching = $derived(userSearch.searching)

  // Multi-select (version_array / multi_option) draft id set + filter (hundreds of options)
  let draft = $state<Set<string>>(new Set())
  let vquery = $state('')

  /** version_array and multi_option share the whole multi-select flow; only the payload id source differs. */
  const isMulti = $derived(kind === 'version_array' || kind === 'multi_option')

  /** Filtered version options — selected items always sort first. */
  const versionOptions = $derived.by<EditMetaOption[]>(() => {
    const q = vquery.trim().toLowerCase()
    const filtered = q ? options.filter((o) => o.value.toLowerCase().includes(q)) : options
    return [...filtered].sort((a, b) => {
      const sa = draft.has(a.id) ? 0 : 1
      const sb = draft.has(b.id) ? 0 : 1
      if (sa !== sb) return sa - sb
      return b.value.localeCompare(a.value) // newest-ish (name reverse)
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

  /* ── Open / close ── */

  async function toggle() {
    if (open) {
      open = false
      return
    }
    if (!(await write.ensureEditMeta(key))) return
    // Still not editable after editmeta load → keep closed (read-only fallback).
    if (!write.editFieldMeta(key, field)) return
    query = ''
    vquery = ''
    if (isMulti) draft = new Set(currentSelectedIds())
    open = true
    if (kind === 'user' || isMulti) queueMicrotask(() => inputEl?.focus())
  }

  /* ── option (single select) ── */

  async function pickOption(opt: EditMetaOption | null) {
    busy = true
    const ok = await write.setField(key, field, opt ? opt.id : null, {
      [field]: opt ? opt.value : null,
    } as Partial<IssueLite>)
    busy = false
    if (ok) open = false
  }

  /* ── multi select (version_array / multi_option) ── */

  /** Currently shown display values → editmeta option ids (reverse map). */
  function currentSelectedIds(): string[] {
    const byName = new Map(options.map((o) => [o.value, o.id]))
    return values.map((n) => byName.get(n)).filter((x): x is string => !!x)
  }

  function toggleVersion(id: string) {
    const next = new Set(draft)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    draft = next
  }

  async function applyMulti() {
    const ids = [...draft]
    const names = options.filter((o) => draft.has(o.id)).map((o) => o.value)
    busy = true
    const ok = await write.setField(key, field, ids, { [field]: names } as Partial<IssueLite>)
    busy = false
    if (ok) open = false
  }

  /* ── user (dev-test assignee) ── */

  interface Cand {
    account_id: string
    display_name: string
    email: string | null
    avatar_url?: string | null
    label?: string
  }

  const meMember = $derived(me.identified && me.email ? issues.members.get(me.email) : undefined)
  const reporterMember = $derived(issues.memberOf(issue.reporter_email))

  /** Local members (personalized) — me → reporter → name. Only with jira_account_id. */
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
        label,
      })
    }
    push(meMember, t('common.me'))
    push(reporterMember, t('qaEditor.reporter'))
    for (const m of withId.sort(byName)) push(m)
    return out
  })

  /** Query-aware candidates (local filter + server search merge). */
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

  async function pickUser(c: Cand | null) {
    busy = true
    const ok = await write.setField(key, 'development_test_assignee', c ? c.account_id : null, {
      development_test_assignee: c ? c.display_name : null,
      development_test_assignee_email: c ? c.email : null,
    })
    busy = false
    if (ok) open = false
  }

  /* ── Outside click / Esc ── */
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
      class="anim-enter absolute left-0 top-full z-30 mt-1 max-h-72 w-60 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
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
            {#if selected}<Icon name="check" size={13} class="text-accent" />{/if}
          </button>
        {/each}
      {:else if isMulti}
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
                  {#if checked}<Icon name="check" size={10} />{/if}
                </span>
                <span class="min-w-0 flex-1 truncate">{opt.value}</span>
              </button>
            {/each}
            {#if versionOptions.length === 0}
              <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.noResults')}</div>
            {/if}
          </div>
          <div class="mt-1 flex items-center justify-end gap-2 border-t border-border-subtle px-2 pt-1.5">
            <button
              type="button"
              onclick={() => (open = false)}
              disabled={busy}
              class="rounded px-2 py-1 text-micro text-text-muted hover:bg-bg-hover disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onclick={applyMulti}
              disabled={busy}
              class="rounded bg-accent px-2 py-1 text-micro font-medium text-white hover:opacity-90 disabled:opacity-50"
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
            <Avatar email={c.email ?? null} name={c.display_name} size={18} />
            <span class="min-w-0 flex-1 truncate">{c.display_name}</span>
            {#if c.label}<span class="flex-none text-micro text-text-muted">{c.label}</span>{/if}
          </button>
        {/each}
        {#if searching}
          <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.searching')}</div>
        {:else if cands.length === 0}
          <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.noResults')}</div>
        {/if}
      {/if}
    </div>
  {/if}
</div>
