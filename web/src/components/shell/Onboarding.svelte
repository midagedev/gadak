<script lang="ts">
  /*
   * First-run setup, in the browser: connect → pick projects → first sync.
   * `scry serve` plus this dialog is the whole path; the CLI is optional.
   *
   * Shown in place of the list only while the mirror is empty AND setup is
   * incomplete (ListView gating) — once a single issue has synced this never
   * renders again, so a normal empty filter result keeps using EmptyState.
   *
   * The three server calls are onboarding-only: PUT onboarding/connect/ (which
   * also stores the site, unlike PUT credential/), GET projects/available/, and
   * POST sync/ + GET sync/progress/. The token is typed once, posted once, and
   * never read back.
   */
  import { t, formatNumber } from '../../lib/i18n'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  const TOKEN_URL = 'https://id.atlassian.com/manage-profile/security/api-tokens'
  const POLL_MS = 1000

  const INPUT =
    'w-full rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[12px] text-text-primary outline-none placeholder:text-text-muted focus:border-accent'
  const PRIMARY =
    'rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50'
  const GHOST =
    'rounded-md border border-border-strong px-2.5 py-1.5 text-[11px] font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'

  type Step = 1 | 2 | 3
  let step = $state<Step>(1)

  /* ── 1. connect ── */
  let site = $state('')
  let email = $state('')
  let token = $state('')
  let connecting = $state(false)
  let connectError = $state<string | null>(null)
  let owner = $state('')

  async function connect(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    connecting = true
    connectError = null
    try {
      const cred = await api.connectJira(site.trim(), email.trim(), token)
      owner = cred.display_name || cred.jira_email
      // The credential is the identity, so the rest of the app has to re-read it.
      await me.refreshIdentity()
      token = '' // no reason to keep it in a live component
      step = 2
      void loadProjects()
    } catch (e) {
      connectError = connectMessage(e)
    } finally {
      connecting = false
    }
  }

  /** Each failure gets its own sentence — a rejected token is not an unreachable site. */
  function connectMessage(e: unknown): string {
    const code = e instanceof ApiError ? e.code : null
    if (code === 'credential_rejected') {
      // CLI init already warns about org keys; surface the same hard-won hint here.
      return `${t('onboarding.errRejected')} ${t('onboarding.errRejectedOrgKey')}`
    }
    if (code === 'site_required') return t('onboarding.errSite')
    if (code === 'email_and_token_required') return t('onboarding.errFields')
    return t('onboarding.errConnect', { message: reason(e) })
  }

  function goBackToConnect(): void {
    step = 1
    projectsError = null
    syncError = null
    stopPolling()
  }

  function reason(e: unknown): string {
    if (e instanceof ApiError) return e.code ?? String(e.status)
    return e instanceof Error ? e.message : String(e)
  }

  /* ── 2. projects ── */
  let projects = $state<api.AvailableProject[]>([])
  let truncated = $state(false)
  let loadingProjects = $state(false)
  let projectsError = $state<string | null>(null)
  let picked = $state<string[]>([])
  let saving = $state(false)

  async function loadProjects(): Promise<void> {
    loadingProjects = true
    projectsError = null
    try {
      const res = await api.getAvailableProjects()
      projects = res.projects
      truncated = res.truncated
    } catch (e) {
      projectsError = t('onboarding.errProjects', { message: reason(e) })
    } finally {
      loadingProjects = false
    }
  }

  function toggle(key: string): void {
    picked = picked.includes(key) ? picked.filter((k) => k !== key) : [...picked, key]
  }

  function selectAll(): void {
    picked = projects.map((p) => p.key)
  }

  function selectNone(): void {
    picked = []
  }

  /** PUT settings/ replaces the whole document, so the current one is read first. */
  async function saveProjectsAndSync(): Promise<void> {
    saving = true
    projectsError = null
    try {
      const current = await api.getSettings()
      await api.putSettings({ ...current, projects: picked })
      step = 3
      void startSync()
    } catch (e) {
      projectsError = t('onboarding.errSaveProjects', { message: reason(e) })
    } finally {
      saving = false
    }
  }

  /* ── 3. first sync ── */
  let progress = $state<api.SyncProgress | null>(null)
  let syncError = $state<string | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function startSync(): Promise<void> {
    syncError = null
    progress = null
    try {
      progress = await api.startFullSync()
    } catch (e) {
      // A run already in flight is not a failure: fall through to polling it.
      if (!(e instanceof ApiError && e.code === 'sync_in_progress')) {
        syncError = t('onboarding.errSync', { message: reason(e) })
        return
      }
    }
    poll()
  }

  function poll(): void {
    stopPolling()
    timer = setInterval(() => void tick(), POLL_MS)
    void tick()
  }

  function stopPolling(): void {
    if (timer) clearInterval(timer)
    timer = null
  }

  async function tick(): Promise<void> {
    try {
      const p = await api.getSyncProgress()
      progress = p
      if (p.running) return
      stopPolling()
      if (p.error) {
        syncError = t('onboarding.errSync', { message: p.error })
        return
      }
      // Refill the store from the freshly filled mirror: the list appears in
      // place, with no reload — and this component unmounts once the pool is warm.
      if (p.done) await issues.refresh()
    } catch (e) {
      stopPolling()
      syncError = t('onboarding.errSync', { message: reason(e) })
    }
  }

  $effect(() => stopPolling)

  const fetched = $derived(progress?.fetched ?? 0)
  const canContinue = $derived(picked.length > 0 && !saving)
  const STEP_LABELS = [t('onboarding.stepCredential'), t('onboarding.stepProjects'), t('onboarding.stepSync')]
</script>

<div class="flex h-full items-start justify-center overflow-y-auto px-6 py-12" data-testid="onboarding">
  <div class="anim-enter w-full max-w-md">
    <p class="text-[10px] uppercase tracking-wide text-text-muted">
      {t('onboarding.stepOf', { n: step })} · {STEP_LABELS[step - 1]}
    </p>
    <h2 class="mt-1 text-[15px] font-semibold text-text-primary">{t('onboarding.title')}</h2>
    <p class="mt-1.5 text-[12px] text-text-secondary">{t('onboarding.intro')}</p>

    <!-- 진행 표시: 얇은 3분할 바 -->
    <div class="mt-4 flex gap-1" aria-hidden="true">
      {#each [1, 2, 3] as n (n)}
        <span class="h-0.5 flex-1 rounded-full {n <= step ? 'bg-accent' : 'bg-border-strong'}"></span>
      {/each}
    </div>

    {#if step === 1}
      <form class="mt-5 flex flex-col gap-3" onsubmit={connect} data-testid="onboarding-connect">
        <label class="flex flex-col gap-1">
          <span class="text-[11px] font-medium text-text-secondary">{t('onboarding.site')}</span>
          <input
            class={INPUT}
            type="text"
            name="site"
            autocomplete="url"
            placeholder={t('onboarding.sitePlaceholder')}
            bind:value={site}
          />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-[11px] font-medium text-text-secondary">{t('onboarding.email')}</span>
          <input class={INPUT} type="email" name="email" autocomplete="email" bind:value={email} />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-[11px] font-medium text-text-secondary">{t('onboarding.token')}</span>
          <input class={INPUT} type="password" name="token" autocomplete="off" bind:value={token} />
          <span class="text-[11px] text-text-muted">
            {t('onboarding.tokenHint')}
            <a class="text-accent hover:underline" href={TOKEN_URL} target="_blank" rel="noreferrer noopener">
              {t('onboarding.tokenLink')}
            </a>
          </span>
        </label>

        {#if connectError}
          <div class="flex flex-col gap-1" role="alert" data-testid="onboarding-error">
            <p class="text-[12px] text-status-reopen">{connectError}</p>
            <a
              class="text-[11px] text-accent hover:underline"
              href={TOKEN_URL}
              target="_blank"
              rel="noreferrer noopener"
            >
              {t('onboarding.tokenLink')}
            </a>
          </div>
        {/if}

        <div class="flex items-center gap-2">
          <button class={PRIMARY} type="submit" disabled={connecting}>
            {connecting ? t('onboarding.connecting') : t('onboarding.connect')}
          </button>
          <button class={GHOST} type="button" onclick={onOpenSettings}>{t('onboarding.openSettings')}</button>
        </div>
      </form>
    {:else if step === 2}
      <div class="mt-5 flex flex-col gap-3" data-testid="onboarding-projects">
        <p class="text-[12px] text-text-secondary">
          {t('onboarding.connectedAs', { name: owner || me.name || me.email || '' })}
        </p>
        <p class="text-[12px] text-text-muted">{t('onboarding.projectsIntro')}</p>

        {#if loadingProjects}
          <p class="text-[12px] text-text-muted">{t('onboarding.loadingProjects')}</p>
        {:else if projectsError}
          <p class="text-[12px] text-status-reopen" role="alert" data-testid="onboarding-error">{projectsError}</p>
          <button class={GHOST} type="button" onclick={() => void loadProjects()}>{t('onboarding.retry')}</button>
        {:else if projects.length === 0}
          <div class="flex flex-col gap-1.5" data-testid="onboarding-no-projects">
            <p class="text-[12px] text-text-secondary">{t('onboarding.noProjects')}</p>
            <p class="text-[11px] text-text-muted">{t('onboarding.noProjectsChecklist')}</p>
            <p class="text-[11px] text-text-muted">{t('onboarding.noProjectsManual')}</p>
            <div class="flex flex-wrap items-center gap-2">
              <button class={GHOST} type="button" onclick={onOpenSettings}>{t('onboarding.openSettings')}</button>
              <button class={GHOST} type="button" onclick={goBackToConnect}>{t('onboarding.switchAccount')}</button>
            </div>
          </div>
        {:else}
          <div class="flex items-center gap-2">
            <button class={GHOST} type="button" data-testid="onboarding-select-all" onclick={selectAll}>
              {t('onboarding.selectAll')}
            </button>
            <button class={GHOST} type="button" data-testid="onboarding-select-none" onclick={selectNone}>
              {t('onboarding.selectNone')}
            </button>
          </div>
          <ul class="max-h-64 overflow-y-auto rounded-md border border-border-subtle bg-bg-panel/50">
            {#each projects as p (p.key)}
              <li class="border-b border-border-subtle/60 last:border-b-0">
                <label class="flex cursor-pointer items-center gap-2.5 px-3 py-2 hover:bg-bg-hover">
                  <input
                    type="checkbox"
                    class="accent-accent"
                    checked={picked.includes(p.key)}
                    onchange={() => toggle(p.key)}
                  />
                  <span class="font-mono text-[11px] text-text-primary">{p.key}</span>
                  <span class="min-w-0 flex-1 truncate text-[12px] text-text-secondary">{p.name}</span>
                </label>
              </li>
            {/each}
          </ul>
          {#if truncated}
            <p class="text-[11px] text-text-muted">
              {t('onboarding.projectsTruncated', { n: formatNumber(projects.length) })}
            </p>
          {/if}
        {/if}

        <div class="flex flex-wrap items-center gap-2">
          <button class={PRIMARY} type="button" disabled={!canContinue} onclick={() => void saveProjectsAndSync()}>
            {saving ? t('onboarding.saving') : t('onboarding.startSync')}
          </button>
          <button class={GHOST} type="button" onclick={goBackToConnect}>{t('onboarding.back')}</button>
          <button class={GHOST} type="button" onclick={goBackToConnect}>{t('onboarding.switchAccount')}</button>
          <span class="text-[11px] text-text-muted">
            {t('onboarding.selectedCount', { n: picked.length })}
          </span>
        </div>
      </div>
    {:else}
      <div class="mt-5 flex flex-col gap-3" data-testid="onboarding-sync">
        {#if syncError}
          <p class="text-[12px] text-status-reopen" role="alert" data-testid="onboarding-error">{syncError}</p>
          <button class={GHOST} type="button" onclick={() => void startSync()}>{t('onboarding.retry')}</button>
        {:else if progress?.done}
          <p class="text-[12px] text-text-secondary" data-testid="onboarding-sync-done">
            {t('onboarding.syncDone', { n: formatNumber(progress.fetched) })}
          </p>
          <p class="text-[11px] text-text-muted">{t('onboarding.syncServeHint')}</p>
        {:else}
          <p class="text-[13px] text-text-primary" data-testid="onboarding-sync-count">
            {progress ? t('onboarding.syncing') : t('onboarding.syncStarting')}
            {#if fetched > 0}
              <span class="ml-1 font-mono tabular-nums text-text-secondary">{formatNumber(fetched)}</span>
            {/if}
          </p>
          <!-- 총량을 모르므로 부정직한 퍼센트 대신 진행 중임만 보여준다. -->
          <div class="h-0.5 overflow-hidden rounded-full bg-border-strong">
            <div class="h-full w-1/3 animate-pulse rounded-full bg-accent"></div>
          </div>
        {/if}
      </div>
    {/if}

    <p class="mt-5 text-[11px] text-text-muted">{t('onboarding.cliHint')}</p>
  </div>
</div>
