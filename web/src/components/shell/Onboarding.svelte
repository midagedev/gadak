<script lang="ts">
  /*
   * First-run setup, in the browser: connect → pick projects → first sync, then
   * an optional fourth step that connects an agent.
   * `gadak serve` plus this dialog is the whole path; the CLI is optional.
   *
   * Shown in place of the list only while the mirror is empty AND setup is
   * incomplete (onboarding store gating) — once a single issue has synced this
   * never renders again, so a normal empty filter result keeps using EmptyState.
   *
   * The three server calls are onboarding-only: PUT onboarding/connect/ (which
   * also stores the site, unlike PUT credential/), GET projects/available/, and
   * POST sync/ + GET sync/progress/. The token is typed once, posted once, and
   * never read back.
   */
  import { t, formatNumber } from '../../lib/i18n'
  import { copyText } from '../../lib/copy-text'
  import * as api from '../../lib/api'
  import { ApiError } from '../../lib/api'
  import { surface } from '../../lib/config'
  import { openInAppBrowser } from '../../lib/desktop-links'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { onboarding, onboardingHold } from '../../stores/onboarding.svelte'
  import Icon from '../ui/Icon.svelte'
  import BrandMark from '../ui/BrandMark.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  const TOKEN_URL = 'https://id.atlassian.com/manage-profile/security/api-tokens'
  const DOCS_BASE = 'https://github.com/midagedev/gadak/blob/main/docs'
  const POLL_MS = 1000
  const onDesktop = surface() === 'desktop'
  let tokenEl: HTMLInputElement | null = $state(null)

  /*
   * Desktop first-run: open the token page in the browse pane and focus the
   * paste field (GDK-71). Serve / hosted keep target="_blank". The sibling
   * link on this hint is the IdP escape hatch — the document interceptor
   * already sends off-site https to the system browser.
   */
  function onCreateTokenClick(event: MouseEvent): void {
    if (!onDesktop) return
    event.preventDefault()
    void openInAppBrowser(TOKEN_URL, { inApp: true, kind: 'other', key: null })
    tokenEl?.focus()
  }

  // The CLI contract (cmd/gadak/mcp_install.go): claude execs `claude mcp add`,
  // the other two print config to paste. Shown, never run — the server has no
  // endpoint for this on purpose, and anyone reading step 4 has a terminal.
  // Two ways in, and the skill is first on purpose: what gadak gives an agent is
  // knowledge (the schema, the queries, the one filter mistake), not tools, so a
  // file that loads when it becomes relevant fits better than a server whose
  // tool definitions sit in context all session. MCP stays for clients with no
  // shell to run the CLI from.
  const SKILL_COMMAND = 'gadak skill install'
  const CLAUDE_COMMAND = 'gadak mcp install claude'
  const MCP_COMMANDS = [CLAUDE_COMMAND, 'gadak mcp install cursor', 'gadak mcp install codex']

  const INPUT =
    'h-control w-full rounded-md border border-border-strong bg-bg-base px-2.5 text-body text-text-primary outline-none placeholder:text-text-muted focus:border-accent'
  const PRIMARY =
    'inline-flex h-control items-center rounded-md bg-accent px-3 text-body font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50'
  const GHOST =
    'inline-flex h-control items-center rounded-md border border-border-strong px-2.5 text-micro font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
  const COPY_BTN =
    'inline-flex h-control-sm flex-none items-center rounded border border-border-strong px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
  const DOC_LINK = 'inline-flex items-center gap-1 text-micro text-accent hover:underline'

  type Step = 1 | 2 | 3 | 4
  let step = $state<Step>(1)

  /* ── 1. connect ── */
  let site = $state('')
  let email = $state('')
  let token = $state('')
  let tokenExpires = $state('')
  let connecting = $state(false)
  let connectError = $state<string | null>(null)
  let owner = $state('')
  let standaloneBlock = $state<{ issues: number; persist: string } | null>(null)
  let replaceStandalone = $state(false)

  async function connect(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    // Second request only after an explicit confirm — never auto-retry the 409.
    if (standaloneBlock && !replaceStandalone) return
    connecting = true
    connectError = null
    try {
      const cred = await api.connectJira(site.trim(), email.trim(), token, tokenExpires, {
        replaceStandalone,
      })
      owner = cred.display_name || cred.jira_email
      // The credential is the identity, so the rest of the app has to re-read it.
      await me.refreshIdentity()
      token = '' // no reason to keep it in a live component
      standaloneBlock = null
      replaceStandalone = false
      step = 2
      void loadProjects()
    } catch (e) {
      if (e instanceof api.StandaloneDataPresentError) {
        standaloneBlock = { issues: e.issues, persist: e.persist }
        replaceStandalone = false
        connectError = null
      } else {
        connectError = connectMessage(e)
      }
    } finally {
      connecting = false
    }
  }

  /** Each failure gets its own sentence — a rejected token is not an unreachable site. */
  function connectMessage(e: unknown): string {
    const code = e instanceof ApiError ? e.code : null
    // Three different mistakes reach Jira as the same 401, and only one of them
    // is recognisable from the token itself: the server answers
    // credential_rejected_org_key when the pasted token carries the ATCTT prefix
    // (internal/server/onboarding.go). Everything else — a scoped token, or a
    // plain typo — stays credential_rejected, so that branch names the scoped
    // trap with a check the user can run instead of a verdict we cannot make.
    if (code === 'credential_rejected_org_key') {
      return `${t('onboarding.errRejected')} ${t('onboarding.errRejectedOrgKey')}`
    }
    if (code === 'credential_rejected') {
      return `${t('onboarding.errRejected')} ${t('onboarding.errRejectedScoped')}`
    }
    if (code === 'site_required') return t('onboarding.errSite')
    if (code === 'email_and_token_required') return t('onboarding.errFields')
    if (code === 'invalid_token_expires') return t('onboarding.errExpires')
    if (code === 'standalone_data_present') return t('onboarding.standaloneBlocked', { n: 0 })
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
      progress = await api.startSync('full')
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
      // The mirror is full, but the store is not refilled here: that would drop
      // the pane before step 4 is read. Entering the app is a click away.
      if (p.done) {
        onboardingHold.active = true
        step = 4
      }
    } catch (e) {
      stopPolling()
      syncError = t('onboarding.errSync', { message: reason(e) })
    }
  }

  /* ── 4. connect an agent (optional) ── */
  let copiedCommand = $state<string | null>(null)

  async function copyCommand(command: string): Promise<void> {
    // copy-text.ts owns the desktop-vs-web transport (GDK-178); the copied
    // state only ever shows on a write that actually happened.
    if (await copyText(command)) {
      copiedCommand = command
      setTimeout(() => {
        if (copiedCommand === command) copiedCommand = null
      }, 1500)
    }
  }

  /**
   * The one exit from step 4, shared by "open the app" and "skip": release the
   * pane, then refill the store from the mirror the third step just filled. The
   * list appears in place, with no reload, and this component unmounts.
   */
  async function finish(): Promise<void> {
    onboardingHold.active = false
    await issues.refresh()
  }

  $effect(() => () => {
    stopPolling()
    onboardingHold.active = false
  })

  const fetched = $derived(progress?.fetched ?? 0)
  // Zero picked is a scope, not an empty form: the CLI, PUT settings/ and
  // POST sync/ have all read an empty project list as "every project this
  // account can see" (internal/server/onboarding.go handleStartSync). The
  // wizard was the only surface calling it illegal, which made the one path a
  // first-time user is on demand a decision the product does not require —
  // and on a large site, the picker is truncated anyway, so "select all" was
  // never the same thing as "everything" (GDK-99).
  const canContinue = $derived(!saving)
  const STEP_LABELS = [
    t('onboarding.stepCredential'),
    t('onboarding.stepProjects'),
    t('onboarding.stepSync'),
    t('onboarding.stepAgent'),
  ]
</script>

<div
  class="flex h-full items-start justify-center overflow-y-auto px-6 py-12"
  data-testid="onboarding"
  data-onboarding-reason={onboarding.reason ?? ''}
>
  <div class="anim-enter w-full max-w-md">
    <div class="mb-5 flex items-center gap-2">
      <BrandMark size={18} class="text-accent" />
      <span class="type-subject wordmark leading-none text-text-primary">gadak</span>
    </div>
    <p class="text-micro uppercase tracking-wide text-text-muted">
      {step === 4 ? t('onboarding.stepOptional') : t('onboarding.stepOf', { n: step })} · {STEP_LABELS[step - 1]}
    </p>
    <h2 class="type-subject mt-1 text-heading text-text-primary">{t('onboarding.title')}</h2>
    <p class="mt-1.5 text-body text-text-secondary">
      {step === 4 ? t('onboarding.agentIntro') : t('onboarding.intro')}
    </p>

    <!-- Progress: three required segments, then the optional one — dimmer even
         when it is the current step, because setup is already done by then. -->
    <div class="mt-4 flex gap-1" aria-hidden="true">
      {#each [1, 2, 3] as n (n)}
        <span class="h-0.5 flex-1 rounded-full {n <= step ? 'bg-accent' : 'bg-border-strong'}"></span>
      {/each}
      <span
        class="h-0.5 flex-1 rounded-full {step === 4 ? 'bg-accent/40' : 'bg-border-subtle'}"
      ></span>
    </div>

    {#if step === 1}
      <form class="mt-5 flex flex-col gap-3" onsubmit={connect} data-testid="onboarding-connect">
        <label class="flex flex-col gap-1">
          <span class="text-micro font-medium text-text-secondary">{t('onboarding.site')}</span>
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
          <span class="text-micro font-medium text-text-secondary">{t('onboarding.email')}</span>
          <input class={INPUT} type="email" name="email" autocomplete="email" bind:value={email} />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-micro font-medium text-text-secondary">{t('onboarding.token')}</span>
          <input
            bind:this={tokenEl}
            class={INPUT}
            type="password"
            name="token"
            autocomplete="off"
            bind:value={token}
          />
          <span class="text-micro text-text-muted">
            {t('onboarding.tokenHint')}
            <a
              class="text-accent hover:underline"
              href={TOKEN_URL}
              target="_blank"
              rel="noreferrer noopener"
              onclick={onDesktop ? onCreateTokenClick : undefined}
            >
              {t('onboarding.tokenLink')}
            </a>
            {#if onDesktop}
              {' · '}
              <a class="text-accent hover:underline" href={TOKEN_URL} target="_blank" rel="noreferrer noopener">
                {t('browse.openExternal')}
              </a>
            {/if}
          </span>
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-micro font-medium text-text-secondary">{t('onboarding.tokenExpires')}</span>
          <input class={INPUT} type="date" name="token_expires_at" bind:value={tokenExpires} />
          <span class="text-micro text-text-muted">{t('onboarding.tokenExpiresHint')}</span>
        </label>

        {#if standaloneBlock}
          <div class="flex flex-col gap-2" role="alert" data-testid="onboarding-standalone-block">
            <p class="text-body text-status-reopen">
              {t('onboarding.standaloneBlocked', { n: standaloneBlock.issues })}
            </p>
            <p class="font-mono text-micro text-text-secondary" data-testid="onboarding-standalone-persist">
              {t('onboarding.standalonePersist', { path: standaloneBlock.persist })}
            </p>
            <p class="text-micro text-text-secondary">{t('onboarding.standaloneOtherWorkspace')}</p>
            <label class="flex items-start gap-2 text-body text-text-secondary">
              <input
                type="checkbox"
                class="mt-0.5 accent-accent"
                data-testid="onboarding-replace-standalone"
                bind:checked={replaceStandalone}
              />
              <span>{t('onboarding.standaloneReplaceConfirm')}</span>
            </label>
          </div>
        {/if}

        {#if connectError}
          <div class="flex flex-col gap-1" role="alert" data-testid="onboarding-error">
            <p class="text-body text-status-reopen">{connectError}</p>
            <a
              class="text-micro text-accent hover:underline"
              href={TOKEN_URL}
              target="_blank"
              rel="noreferrer noopener"
              onclick={onDesktop ? onCreateTokenClick : undefined}
            >
              {t('onboarding.tokenLink')}
            </a>
          </div>
        {/if}

        <div class="flex items-center gap-2">
          <button
            class={PRIMARY}
            type="submit"
            disabled={connecting || (!!standaloneBlock && !replaceStandalone)}
          >
            {connecting
              ? t('common.verifying')
              : standaloneBlock
                ? t('onboarding.standaloneReplace')
                : t('onboarding.connect')}
          </button>
          <button class={GHOST} type="button" onclick={onOpenSettings}>{t('onboarding.openSettings')}</button>
        </div>
      </form>
    {:else if step === 2}
      <div class="mt-5 flex flex-col gap-3" data-testid="onboarding-projects">
        <p class="text-body text-text-secondary">
          {t('onboarding.connectedAs', { name: owner || me.name || me.email || '' })}
        </p>
        <p class="text-body text-text-muted">{t('onboarding.projectsIntro')}</p>

        {#if loadingProjects}
          <p class="text-micro text-text-muted">{t('onboarding.loadingProjects')}</p>
        {:else if projectsError}
          <p class="text-body text-status-reopen" role="alert" data-testid="onboarding-error">{projectsError}</p>
          <button class={GHOST} type="button" onclick={() => void loadProjects()}>{t('common.retry')}</button>
        {:else if projects.length === 0}
          <div class="flex flex-col gap-1.5" data-testid="onboarding-no-projects">
            <p class="text-micro text-text-secondary">{t('onboarding.noProjects')}</p>
            <p class="text-micro text-text-muted">{t('onboarding.noProjectsChecklist')}</p>
            <p class="text-micro text-text-muted">{t('onboarding.noProjectsManual')}</p>
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
                  <span class="font-mono text-micro text-text-primary">{p.key}</span>
                  <span class="min-w-0 flex-1 truncate text-body text-text-secondary">{p.name}</span>
                </label>
              </li>
            {/each}
          </ul>
          {#if truncated}
            <p class="text-micro text-text-muted">
              {t('onboarding.projectsTruncated', { n: formatNumber(projects.length) })}
            </p>
          {/if}
        {/if}

        <div class="flex flex-wrap items-center gap-2">
          <button class={PRIMARY} type="button" disabled={!canContinue} onclick={() => void saveProjectsAndSync()}>
            {saving ? t('common.saving') : t('onboarding.startSync')}
          </button>
          <button class={GHOST} type="button" onclick={goBackToConnect}>{t('onboarding.back')}</button>
          <button class={GHOST} type="button" onclick={goBackToConnect}>{t('onboarding.switchAccount')}</button>
          <span class="text-micro text-text-muted">
            {t('onboarding.selectedCount', { n: picked.length })}
          </span>
        </div>
      </div>
    {:else if step === 3}
      <div class="mt-5 flex flex-col gap-3" data-testid="onboarding-sync">
        {#if syncError}
          <p class="text-body text-status-reopen" role="alert" data-testid="onboarding-error">{syncError}</p>
          <button class={GHOST} type="button" onclick={() => void startSync()}>{t('common.retry')}</button>
        {:else}
          <p class="text-body text-text-primary" data-testid="onboarding-sync-count">
            {progress ? t('sync.busyIssues') : t('onboarding.syncStarting')}
            {#if fetched > 0}
              <span class="ml-1 font-mono tabular-nums text-text-secondary">{formatNumber(fetched)}</span>
            {/if}
          </p>
          <!-- Unknown total — show activity only, not a dishonest percent. -->
          <div class="h-0.5 overflow-hidden rounded-full bg-border-strong">
            <div class="h-full w-1/3 animate-pulse rounded-full bg-accent"></div>
          </div>
        {/if}
      </div>
    {:else}
      <div class="mt-5 flex flex-col gap-4" data-testid="onboarding-agent">
        <div class="flex flex-col gap-1.5">
          <p class="text-body text-text-primary" data-testid="onboarding-sync-done">
            {t('onboarding.syncDone', { n: formatNumber(fetched) })}
          </p>
          <p class="text-micro text-text-muted">{t('onboarding.agentWhy')}</p>
        </div>

        <div class="flex flex-col gap-2 rounded-md border border-border-subtle bg-bg-panel/60 p-3">
          <div class="flex flex-col gap-0.5">
            <span class="text-micro font-medium text-text-secondary">{t('onboarding.agentCommandsLabel')}</span>
            <span class="text-micro text-text-muted">{t('onboarding.agentCommandsHint')}</span>
          </div>

          <div
            class="flex items-center gap-2 rounded border border-border-strong bg-bg-base px-2 py-1.5"
            data-testid="onboarding-cmd-skill"
          >
            <code class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-body text-text-primary">
              {SKILL_COMMAND}
            </code>
            <button
              class={COPY_BTN}
              type="button"
              data-testid="onboarding-copy-skill"
              onclick={() => void copyCommand(SKILL_COMMAND)}
            >
              {copiedCommand === SKILL_COMMAND ? t('settings.copied') : t('settings.copy')}
            </button>
          </div>
          <p class="text-micro text-text-muted">{t('onboarding.agentSkillCaption')}</p>

          <!-- One box on this card, and it is the recommended command. The MCP
               routes are quiet rows: an independent look found that giving the
               claude row the same border, size and weight as the skill box left
               the priority resting on the word "Or", so two identical boxes
               asked the reader to choose without saying how. Same Copy buttons
               on the same right edge — the demotion is border and type, not
               reach. -->
          <p class="mt-3 text-micro text-text-muted">{t('onboarding.agentMcpCaption')}</p>
          <div class="flex flex-col gap-1">
            {#each MCP_COMMANDS as cmd (cmd)}
              <div
                class="flex items-center gap-2 rounded border border-transparent px-2"
                data-testid={cmd === CLAUDE_COMMAND ? 'onboarding-cmd-claude' : undefined}
              >
                <code class="min-w-0 flex-1 truncate font-mono text-micro text-text-secondary">{cmd}</code>
                <button
                  class={COPY_BTN}
                  type="button"
                  data-testid={cmd === CLAUDE_COMMAND ? 'onboarding-copy-claude' : undefined}
                  onclick={() => void copyCommand(cmd)}
                >
                  {copiedCommand === cmd ? t('settings.copied') : t('settings.copy')}
                </button>
              </div>
            {/each}
          </div>
        </div>

        <p class="text-micro text-text-muted">{t('onboarding.agentNoCli')}</p>

        <div class="flex flex-wrap items-center gap-3">
          <a class={DOC_LINK} href="{DOCS_BASE}/AGENT_SETUP.md" target="_blank" rel="noreferrer noopener">
            {t('onboarding.agentDocsSetup')}<Icon name="arrow-up-right" size={12} />
          </a>
          <a class={DOC_LINK} href="{DOCS_BASE}/RECIPES.md" target="_blank" rel="noreferrer noopener">
            {t('onboarding.agentDocsRecipes')}<Icon name="arrow-up-right" size={12} />
          </a>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button class={PRIMARY} type="button" data-testid="onboarding-finish" onclick={() => void finish()}>
            {t('onboarding.agentDone')}
          </button>
          <button class={GHOST} type="button" data-testid="onboarding-skip" onclick={() => void finish()}>
            {t('onboarding.agentSkip')}
          </button>
        </div>
      </div>
    {/if}

    <p class="mt-5 text-micro text-text-muted">
      {step === 4 ? t('onboarding.syncServeHint') : t('onboarding.cliHint')}
    </p>
  </div>
</div>
