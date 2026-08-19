<script lang="ts">
  /*
   * New-issue dialog (write, local-first + personalized defaults).
   *  - Project/type = write.writeMetaProjects (local create-meta, 0ms).
   *    write-meta already asked origin for create-meta. Settled-empty or no
   *    credential is terminal — do not GET. Retry is the only create-meta GET.
   *  - Defaults (quiet suggestions):
   *      Project ① recent create ② current filter source_project ③ selected issue ④ first
   *      Type = per-project recent (else first type from create-meta)
   *      Assignee = empty (no force)
   *  - Labels: autocomplete by project frequency in local pool + recent on top.
   *  - On success: close + selection.select(new key) → detail opens (recency via createIssue).
   *  Entry: sidebar "+ New issue" / shortcut c (App.svelte).
   */
  import { t } from '../../lib/i18n'
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { createUserSearch } from '../../lib/user-search.svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import { write } from '../../stores/write.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { filters } from '../../stores/filters.svelte'
  import { recentOf, whenReady } from '../../lib/recency'
  import { isHostedDemo } from '../../lib/config'
  import type { CreateMetaProject, JiraUser, PriorityOption } from '../../lib/types'
  import Icon from '../ui/Icon.svelte'

  type WriteDialogState = 'loading' | 'need-token' | 'meta-failed' | 'form'

  // A native <select> keeps its keyboard model and its OS popup; only the closed
  // state is ours. appearance-none drops the platform arrow, so the chevron has
  // to come back from the icon set — and it must not eat the click.
  const SELECT =
    'h-control w-full appearance-none rounded-md border border-border-strong bg-bg-base pl-2.5 pr-7 text-body text-text-primary outline-none focus:border-accent'
  const SELECT_CHEVRON =
    'pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rotate-90 text-text-muted'

  let fallbackProjects = $state<CreateMetaProject[]>([])
  let fetchingMeta = $state(false)
  let fetchError = $state<string | null>(null)
  let credRequired = $state(false)
  let recencyReady = $state(false)

  // Local meta first, else fallback.
  const projects = $derived(write.writeMetaProjects.length ? write.writeMetaProjects : fallbackProjects)

  /**
   * Single owner for "can this dialog show the create form".
   * write.ensureWritable already gates opening; this asks the same facts
   * (credential + settled write-meta) instead of a weaker "projects empty?".
   */
  const writeState = $derived.by((): WriteDialogState => {
    if (projects.length > 0) return recencyReady ? 'form' : 'loading'
    if (fetchingMeta) return 'loading'
    if (credRequired) return 'need-token'
    if (!write.credentialLoaded) return 'loading'
    if (!write.configured && !isHostedDemo()) return 'need-token'
    if (fetchError) return 'meta-failed'
    if (write.writeMetaLoaded) return 'meta-failed'
    return 'loading'
  })

  let projectKey = $state('')
  let issueTypeId = $state('')
  let summary = $state('')
  let description = $state('')
  let priority = $state('')
  let priorities = $state<PriorityOption[]>([])
  let prioritiesError = $state<string | null>(null)
  let prioritiesLoading = $state(true)
  let duedate = $state('')

  // Assignee (optional)
  let assignee = $state<JiraUser | null>(null)
  let userQuery = $state('')
  let userMenuOpen = $state(false)

  const userSearch = createUserSearch(() => userQuery, {
    debounceMs: 250,
    minLength: 2,
    onResults: () => {
      userMenuOpen = true
    },
  })
  const userResults = $derived(userSearch.results)
  const userSearching = $derived(userSearch.searching)

  // Labels (optional)
  let labels = $state<string[]>([])
  let labelInput = $state('')
  let labelMenuOpen = $state(false)

  let submitting = $state(false)
  let submitError = $state<string | null>(null)
  let summaryEl: HTMLInputElement | null = $state(null)

  const selectedProject = $derived(projects.find((p) => p.key === projectKey))
  const issueTypes = $derived(selectedProject?.issue_types ?? [])

  onMount(() => {
    void whenReady().then(() => {
      recencyReady = true
    })
    void loadPriorities()
  })

  async function loadPriorities() {
    prioritiesLoading = true
    prioritiesError = null
    try {
      const res = await api.getPriorities()
      priorities = res.priorities.filter((p) => p.id)
    } catch {
      priorities = []
      prioritiesError = t('write.prioritiesFailed')
    } finally {
      prioritiesLoading = false
    }
  }

  // write-meta is the owner. Do not GET create-meta just because projects
  // are still empty — that is the in-flight / fake-origin hang. Retry only.
  $effect(() => {
    if (writeState === 'form') applyDefaults()
  })

  async function loadFallback() {
    fetchingMeta = true
    fetchError = null
    try {
      const res = await api.getCreateMeta()
      fallbackProjects = res.projects
      if (res.projects.length === 0) fetchError = t('write.metaFailed')
    } catch (e) {
      if (e instanceof ApiError && e.code === 'credential_required') {
        credRequired = true
        return
      }
      fetchError = t('write.metaFailed')
    } finally {
      fetchingMeta = false
    }
  }

  function retryMeta() {
    fetchError = null
    credRequired = false
    void loadFallback()
  }

  /** Infer project/type defaults (once). */
  let defaultsApplied = false
  function applyDefaults() {
    if (defaultsApplied || projects.length === 0) return
    defaultsApplied = true
    projectKey = inferProject()
    issueTypeId = inferType(projectKey)
    queueMicrotask(() => summaryEl?.focus())
  }

  function inferProject(): string {
    const keys = projects.map((p) => p.key)
    for (const r of recentOf('create-project')) if (keys.includes(r)) return r
    const fromFilter = filters.filters.jira_project?.[0]
    if (fromFilter && keys.includes(fromFilter)) return fromFilter
    const sel = selection.selectedKey ? issues.get(selection.selectedKey) : undefined
    if (sel) {
      const p = write.projectOf(sel)
      if (keys.includes(p)) return p
    }
    return keys[0] ?? ''
  }

  function inferType(pk: string): string {
    const p = projects.find((x) => x.key === pk)
    if (!p) return ''
    const types = p.issue_types
    // Last-used type id for this project, else create-meta order (never a localized name).
    for (const r of recentOf(`create-type:${pk}`)) if (types.some((t) => t.id === r)) return r
    return types[0]?.id ?? ''
  }

  // On project change, if type is invalid, switch to the inferred type for that project.
  $effect(() => {
    if (selectedProject && !issueTypes.some((t) => t.id === issueTypeId)) {
      issueTypeId = inferType(projectKey)
    }
  })

  function pickUser(u: JiraUser) {
    assignee = u
    userQuery = ''
    userMenuOpen = false
  }
  function clearUser() {
    assignee = null
  }

  // ── Label autocomplete ── project frequency + recent on top.
  const labelFreq = $derived.by(() => {
    const freq = new Map<string, number>()
    for (const it of issues.allIssues) {
      if (write.projectOf(it) !== projectKey) continue
      for (const l of it.labels ?? []) freq.set(l, (freq.get(l) ?? 0) + 1)
    }
    return freq
  })
  const labelSuggestions = $derived.by<string[]>(() => {
    const q = labelInput.trim().toLowerCase()
    const recent = recentOf('label')
    const all = new Set<string>([...recent, ...labelFreq.keys()])
    const arr = [...all].filter(
      (l) => !labels.includes(l) && (!q || l.toLowerCase().includes(q)),
    )
    arr.sort((a, b) => {
      const ra = recent.indexOf(a)
      const rb = recent.indexOf(b)
      const rra = ra === -1 ? Infinity : ra
      const rrb = rb === -1 ? Infinity : rb
      if (rra !== rrb) return rra - rrb // recent first
      return (labelFreq.get(b) ?? 0) - (labelFreq.get(a) ?? 0) // then by frequency
    })
    return arr.slice(0, 8)
  })

  function addLabel(l: string) {
    const v = l.trim()
    if (!v || labels.includes(v)) {
      labelInput = ''
      return
    }
    labels = [...labels, v]
    labelInput = ''
  }
  function removeLabel(l: string) {
    labels = labels.filter((x) => x !== l)
  }
  function onLabelKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      addLabel(labelSuggestions[0] ?? labelInput)
    } else if (e.key === 'Backspace' && !labelInput && labels.length) {
      labels = labels.slice(0, -1)
    }
  }

  async function submit(e: Event) {
    e.preventDefault()
    if (submitting) return
    const s = summary.trim()
    if (!projectKey || !issueTypeId || !s) {
      submitError = t('write.requiredFields')
      return
    }
    submitting = true
    submitError = null
    const res = await write.createIssue({
      project_key: projectKey,
      issue_type: issueTypeId,
      summary: s,
      description_text: description.trim() || undefined,
      assignee_account_id: assignee?.account_id ?? undefined,
      priority_id: priority || undefined,
      labels: labels.length ? labels : undefined,
      duedate: duedate || undefined,
    })
    submitting = false
    if (res.ok && res.key) {
      write.closeNewIssue()
      selection.select(res.key)
    } else {
      submitError = res.error ?? t('write.createFailed')
    }
  }

  function close() {
    write.closeNewIssue()
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-[#1c1812]/28 p-4 pt-[8vh] backdrop-blur-[2px]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close()
  }}
>
  <div
    use:trapFocus
    class="anim-enter w-full max-w-lg rounded-lg border border-border-strong bg-bg-panel p-5 shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('write.newIssue')}
    data-testid="new-issue-dialog"
    data-write-state={writeState}
  >
    <h2 class="type-subject mb-4 text-[18px] leading-snug text-text-primary">{t('write.newIssue')}</h2>

    {#if writeState === 'loading'}
      <div class="py-8 text-center text-body text-text-muted">{t('common.loading')}</div>
    {:else if writeState === 'need-token'}
      <div class="flex flex-col items-center gap-3 py-8 text-center">
        <p class="text-body text-status-reopen">{t('write.needToken')}</p>
        <button
          type="button"
          onclick={() => write.openSettings()}
          class="inline-flex h-control items-center rounded-md border border-border-strong px-3 text-[12px] text-text-secondary hover:bg-bg-hover"
          >{t('common.setCredentials')}</button
        >
      </div>
    {:else if writeState === 'meta-failed'}
      <div class="flex flex-col items-center gap-3 py-8 text-center">
        <p class="text-body text-status-reopen">{t('write.metaFailed')}</p>
        <button
          type="button"
          onclick={retryMeta}
          class="inline-flex h-control items-center rounded-md border border-border-strong px-3 text-[12px] text-text-secondary hover:bg-bg-hover"
          >{t('common.retry')}</button
        >
      </div>
    {:else}
      <form onsubmit={submit} class="flex flex-col gap-3">
        <!-- Project + type -->
        <div class="flex gap-3">
          <label class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('common.project')}</span>
            <span class="relative flex">
              <select bind:value={projectKey} class={SELECT}>
                {#each projects as p (p.key)}
                  <option value={p.key}>{p.key} · {p.name}</option>
                {/each}
              </select>
              <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
            </span>
          </label>
          <label class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('common.type')}</span>
            <span class="relative flex">
              <select bind:value={issueTypeId} class={SELECT}>
                {#each issueTypes as t (t.id)}
                  <option value={t.id}>{t.name}</option>
                {/each}
              </select>
              <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
            </span>
          </label>
        </div>

        <!-- Title -->
        <label class="flex flex-col gap-1">
          <span class="text-micro text-text-secondary">{t('common.title')} <span class="text-status-reopen">*</span></span>
          <input
            bind:this={summaryEl}
            bind:value={summary}
            type="text"
            required
            maxlength="255"
            class="h-control rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none focus:border-accent"
            placeholder={t('write.issueTitle')}
          />
        </label>

        <!-- Description -->
        <label class="flex flex-col gap-1">
          <span class="text-micro text-text-secondary">{t('common.description')}</span>
          <textarea
            bind:value={description}
            rows="4"
            class="resize-y rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-body text-text-primary outline-none focus:border-accent"
            placeholder={t('write.descriptionPlain')}
          ></textarea>
        </label>

        <!-- Assignee + priority -->
        <div class="flex gap-3">
          <div class="relative flex min-w-0 flex-1 flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('common.assignee')}</span>
            {#if assignee}
              <div class="flex h-control items-center gap-2 rounded-md border border-border-strong bg-bg-base px-2 text-body">
                <span class="min-w-0 flex-1 truncate text-text-primary">{assignee.display_name}</span>
                <button type="button" onclick={clearUser} class="flex flex-none items-center text-text-muted hover:text-status-reopen"><Icon name="x" size={13} /></button>
              </div>
            {:else}
              <input
                bind:value={userQuery}
                type="text"
                placeholder={t('write.searchPersonOptional')}
                onfocus={() => (userMenuOpen = userResults.length > 0)}
                class="h-control rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none focus:border-accent"
              />
              {#if userMenuOpen && (userResults.length > 0 || userSearching)}
                <div class="absolute left-0 right-0 top-full z-20 mt-1 max-h-48 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay">
                  {#if userSearching}
                    <div class="px-3 py-1.5 text-micro text-text-muted">{t('common.searching')}</div>
                  {/if}
                  {#each userResults as u (u.account_id)}
                    <button
                      type="button"
                      onclick={() => pickUser(u)}
                      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                    >
                      {#if u.avatar_url}
                        <img src={u.avatar_url} alt={u.display_name} class="h-4 w-4 flex-none rounded-full object-cover" loading="lazy" />
                      {/if}
                      <span class="min-w-0 flex-1 truncate">{u.display_name}</span>
                    </button>
                  {/each}
                </div>
              {/if}
            {/if}
          </div>
          <label class="flex w-32 flex-none flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('common.priority')}</span>
            <span class="relative flex">
              <select
                bind:value={priority}
                class={SELECT}
                disabled={!!prioritiesError || prioritiesLoading}
                data-testid="new-issue-priority"
              >
                <option value="">{t('common.defaultParen')}</option>
                {#each priorities as p (p.id)}
                  <option value={p.id}>{p.name}</option>
                {/each}
              </select>
              <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
            </span>
            {#if prioritiesError}
              <span class="text-micro text-status-reopen">{prioritiesError}</span>
            {/if}
          </label>
        </div>

        <label class="flex flex-col gap-1">
          <span class="text-micro text-text-secondary">{t('common.due')}</span>
          <input
            bind:value={duedate}
            type="date"
            data-testid="new-issue-duedate"
            class="h-control rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none focus:border-accent"
          />
        </label>

        <!-- Labels -->
        <div class="relative flex flex-col gap-1">
          <span class="text-micro text-text-secondary">{t('common.labels')}</span>
          <div class="flex min-h-control flex-wrap items-center gap-1 rounded-md border border-border-strong bg-bg-base px-2 py-1">
            {#each labels as l (l)}
              <span class="inline-flex items-center gap-1 rounded bg-bg-elevated px-1.5 py-0.5 text-micro text-text-secondary">
                {l}
                <button type="button" onclick={() => removeLabel(l)} class="flex items-center text-text-muted hover:text-status-reopen"><Icon name="x" size={11} /></button>
              </span>
            {/each}
            <input
              bind:value={labelInput}
              onkeydown={onLabelKeydown}
              onfocus={() => (labelMenuOpen = true)}
              onblur={() => setTimeout(() => (labelMenuOpen = false), 120)}
              type="text"
              placeholder={labels.length ? '' : t('write.addLabelOptional')}
              class="min-w-24 flex-1 bg-transparent text-body text-text-primary outline-none"
            />
          </div>
          {#if labelMenuOpen && labelSuggestions.length > 0}
            <div class="absolute left-0 right-0 top-full z-20 mt-1 max-h-40 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay">
              {#each labelSuggestions as l (l)}
                <button
                  type="button"
                  onclick={() => addLabel(l)}
                  class="flex w-full items-center justify-between gap-2 px-3 py-1 text-left text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary"
                >
                  <span class="min-w-0 flex-1 truncate">{l}</span>
                  {#if labelFreq.get(l)}<span class="flex-none text-micro text-text-muted">{labelFreq.get(l)}</span>{/if}
                </button>
              {/each}
            </div>
          {/if}
        </div>

        {#if submitError}
          <p class="whitespace-pre-wrap text-[12px] text-status-reopen" data-testid="new-issue-error">{submitError}</p>
        {/if}

        <div class="mt-1 flex items-center justify-end gap-2">
          <button
            type="button"
            onclick={close}
            class="inline-flex h-control items-center rounded-md px-3 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
            >{t('common.cancel')}</button
          >
          <button
            type="submit"
            disabled={submitting}
            class="inline-flex h-control items-center rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {submitting ? t('common.creating') : t('common.create')}
          </button>
        </div>
      </form>
    {/if}
  </div>
</div>
