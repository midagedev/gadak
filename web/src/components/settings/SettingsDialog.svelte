<script lang="ts">
  /*
   * Server settings editor dialog (`~/.scry/config.json`, loopback-only API).
   *  - Open → GET settings/; save → PUT settings/ (full replace) → location.reload() on ok.
   *    config.json, bootstrap members, and group inject are all server-derived — full
   *    reload is the honest path.
   *  - Records/arrays expand to editable row arrays and reassemble on save (drop empty keys).
   *  - Advanced JSON textarea vs form: last edit wins (successful parse rehydrates form).
   *  Personal Jira token is JiraKeySettings' job — only a link here.
   *  Same modal pattern as JiraKeySettings (Esc / backdrop close).
   */
  import { t, locale, setLocale, type Locale } from '../../lib/i18n'
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import type { ScrySettings, SettingsFieldSpec, SettingsRuntime } from '../../lib/api'
  import type { ScryFeatures } from '../../lib/config'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import KeyValueRows from './KeyValueRows.svelte'
  import ScopePicker, { type ScopeOption } from './ScopePicker.svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import Icon from '../ui/Icon.svelte'

  let { onclose }: { onclose: () => void } = $props()

  type Tab = 'sync' | 'sources' | 'features' | 'groups' | 'members' | 'fields'
  const TABS: [Tab, string][] = [
    ['sync', t('settings.tabSync')],
    ['sources', t('settings.tabSources')],
    ['features', t('settings.tabFeatures')],
    ['groups', t('settings.tabTeams')],
    ['members', t('settings.tabMembers')],
    ['fields', t('settings.tabFields')],
  ]

  const FEATURES: [keyof ScryFeatures, string, string][] = [
    ['feed', t('settings.featureFeed'), t('settings.featureFeedDesc')],
    ['push', t('settings.featurePush'), t('settings.featurePushDesc')],
    ['deploy', t('settings.featureDeploy'), t('settings.featureDeployDesc')],
    ['qa', t('settings.featureQa'), t('settings.featureQaDesc')],
    ['teamGroups', t('settings.featureTeams'), t('settings.featureTeamsDesc')],
  ]

  /** Preset values in seconds. 0 = server default; -1 = custom number entry. */
  const SYNC_PRESETS: { value: number; label: string }[] = [
    { value: 0, label: t('settings.intervalDefault') },
    { value: 30, label: t('settings.intervalPreset30s') },
    { value: 60, label: t('settings.intervalPreset1m') },
    { value: 300, label: t('settings.intervalPreset5m') },
    { value: 900, label: t('settings.intervalPreset15m') },
    { value: -1, label: t('settings.intervalCustom') },
  ]
  const RECONCILE_PRESETS: { value: number; label: string }[] = [
    { value: 0, label: t('settings.intervalDefault') },
    { value: 3600, label: t('settings.intervalPreset1h') },
    { value: 21600, label: t('settings.intervalPreset6h') },
    { value: 86400, label: t('settings.intervalPreset24h') },
    { value: -1, label: t('settings.intervalCustom') },
  ]

  const INPUT =
    'h-control w-full rounded-md border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent'
  // Spelled out rather than `${INPUT} pr-7`: px-2 and pr-7 resolve against each
  // other by cascade order, not by class order, so the chevron's clearance
  // would depend on how Tailwind happens to emit the two utilities.
  const SELECT =
    'h-control w-full appearance-none rounded-md border border-border-strong bg-bg-base pl-2 pr-7 text-[12px] text-text-primary outline-none focus:border-accent'
  const SELECT_CHEVRON =
    'pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rotate-90 text-text-muted'
  const ADD_BTN =
    'inline-flex h-control-sm items-center self-start rounded-md border border-border-strong px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
  const DEL_BTN =
    'flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-status-reopen'
  const COPY_BTN =
    'inline-flex h-control-sm items-center rounded border border-border-strong px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'

  interface Kv {
    k: string
    v: string
  }
  interface GroupRow {
    key: string
    label: string
    color: string
  }
  interface ProductRow {
    group: string
    key: string
    label: string
  }
  interface RuleRow {
    group: string
    projects: string
    labels: string
    components: string
  }
  interface MemberRow {
    email: string
    name: string
    display_name: string
    group: string
    department: string
    job_role: string
    jira_account_id: string
    avatar_url: string
  }

  let loading = $state(true)
  let saving = $state(false)
  let error = $state<string | null>(null)
  let tab = $state<Tab>('sync')

  let projectKeys = $state<string[]>([])
  /** Manual entry, used only while the site's project list is unreachable. */
  let projectsText = $state('')
  let staleText = $state('72')
  let qaDashboardUrl = $state('')
  let features = $state<Record<keyof ScryFeatures, boolean>>({
    feed: false,
    push: false,
    deploy: false,
    qa: false,
    teamGroups: false,
  })
  let groupRows = $state<GroupRow[]>([])
  let productRows = $state<ProductRow[]>([])
  let ruleRows = $state<RuleRow[]>([])
  let memberRows = $state<MemberRow[]>([])
  let openMember = $state<number | null>(null)
  let fieldMapRows = $state<Kv[]>([])
  let editableRows = $state<Kv[]>([])
  let bodyFieldsText = $state('')
  // Discovered field specs. specsTouched gates the PUT: absence preserves
  // discovery output on the server, so an untouched section costs nothing.
  let specRows = $state<SettingsFieldSpec[]>([])
  let specsTouched = $state(false)
  let specsSupported = $state(false)

  // Interval UI: preset select (-1 = custom) + custom seconds text.
  let syncPreset = $state(0)
  let syncCustomText = $state('')
  let reconcilePreset = $state(0)
  let reconcileCustomText = $state('')
  let defaultSyncSec = $state(60)
  let defaultReconcileSec = $state(3600)
  let runtime = $state<SettingsRuntime | null>(null)
  // Hidden until there is traffic to report: a row that only ever says zero
  // teaches nothing, and zero here means "nothing flushed yet", not "headroom".
  const apiUsage = $derived(
    runtime?.apiUsage && runtime.apiUsage.last_7_days.requests > 0 ? runtime.apiUsage : null,
  )
  let copiedKey = $state<string | null>(null)

  /* ── Sources tab (what the mirror pulls) ──
   * Both lists come from the live site, so they are fetched when the tab is
   * first opened rather than with the dialog: settings gets opened for plenty of
   * reasons that should not cost a Jira round-trip.
   */
  let sourcesRequested = false
  let projectOptions = $state<ScopeOption[]>([])
  /** Site list unreachable (no credential, Jira down) → keep the old text box,
   *  which is the only way to configure a scope without asking the site. */
  let projectsPickerReady = $state(false)
  let projectsLoading = $state(false)
  /** Hand-edited the manual keys — the list must not replace the field under a
   *  typing user, however late it arrives. */
  let projectsTouched = $state(false)
  /**
   * Whether the Confluence source is on for this profile. The section renders
   * either way now: it used to be hidden while off, which made an unconfigured
   * source and a missing feature look identical — and there was no other way to
   * turn it on, so the app quietly contradicted its own "Jira and Confluence"
   * promise for anyone who never edited config.json by hand.
   */
  let confluenceConfigured = $state(false)
  /** Pending on/off, applied on Save like every other field on this tab. */
  let confluenceOn = $state(false)

  /*
   * Choosing a space is the request to mirror it. Without this, picking one
   * while the source was off and pressing Save sent {enabled:false, spaces:[]}
   * — the chip sat on screen and the save discarded it silently, which is the
   * failure this whole screen exists to remove.
   *
   * The button is left to carry the one case that genuinely needs a decision:
   * turning the source on with *no* scope, which mirrors every team space.
   * Turning it off clears the scope, so this can never fight that click.
   */
  $effect(() => {
    if (spaceKeys.length > 0 && !confluenceOn) confluenceOn = true
  })
  let spaceKeys = $state<string[]>([])
  let spaceOptions = $state<ScopeOption[]>([])
  let spacesLoading = $state(false)
  let spacesError = $state<string | null>(null)
  let showPersonalSpaces = $state(false)

  // Personal spaces are one per colleague and almost never mirror targets, so
  // they stay out of the list until asked for — except one already selected,
  // which must stay visible or the picker would look like it dropped it.
  const visibleSpaceOptions = $derived(
    showPersonalSpaces
      ? spaceOptions
      : spaceOptions.filter((o) => o.hint !== 'personal' || spaceKeys.includes(o.value)),
  )

  let jsonText = $state('')
  let jsonError = $state<string | null>(null)

  const splitCsv = (s: string): string[] => s.split(',').map((v) => v.trim()).filter(Boolean)
  const joinCsv = (a: string[] | undefined): string => (a ?? []).join(', ')
  const kvRows = (r: Record<string, string> | undefined): Kv[] =>
    Object.entries(r ?? {}).map(([k, v]) => ({ k, v }))

  function rec(rows: Kv[]): Record<string, string> {
    const out: Record<string, string> = {}
    for (const row of rows) {
      const k = row.k.trim()
      if (k) out[k] = row.v.trim()
    }
    return out
  }

  function pickPreset(sec: number, presets: { value: number }[]): number {
    if (!sec || sec <= 0) return 0
    return presets.some((p) => p.value === sec) ? sec : -1
  }

  function resolveInterval(preset: number, customText: string): number {
    if (preset === 0) return 0
    if (preset > 0) return preset
    const n = Number(customText)
    return Number.isFinite(n) && n > 0 ? Math.floor(n) : 0
  }

  /** Expand server response (or JSON textarea) into form state. */
  function load(s: ScrySettings) {
    projectKeys = [...(s.projects ?? [])]
    projectsText = joinCsv(s.projects)
    // The key is absent unless the source is configured, and PUTting it while
    // it is off is rejected — so its presence is the section's on/off switch.
    confluenceConfigured = s.confluence !== undefined
    confluenceOn = confluenceConfigured
    spaceKeys = [...(s.confluence?.spaces ?? [])]
    staleText = String(s.staleThresholdHours ?? 72)
    qaDashboardUrl = s.qaDashboardUrl ?? ''
    features = { ...features, ...(s.features ?? {}) }

    if (s.runtime) {
      runtime = s.runtime
      defaultSyncSec = s.runtime.defaultSyncIntervalSec || 60
      defaultReconcileSec = s.runtime.defaultReconcileIntervalSec || 3600
    }

    const syncSec = s.syncIntervalSec ?? 0
    syncPreset = pickPreset(syncSec, SYNC_PRESETS)
    syncCustomText = syncPreset === -1 ? String(syncSec) : ''

    const recSec = s.reconcileIntervalSec ?? 0
    reconcilePreset = pickPreset(recSec, RECONCILE_PRESETS)
    reconcileCustomText = reconcilePreset === -1 ? String(recSec) : ''

    const groupKeys = [
      ...new Set([...Object.keys(s.groupLabels ?? {}), ...Object.keys(s.groupColors ?? {})]),
    ]
    groupRows = groupKeys.map((key) => ({
      key,
      label: s.groupLabels?.[key] ?? '',
      color: s.groupColors?.[key] ?? '',
    }))
    productRows = Object.entries(s.productByGroup ?? {}).map(([group, p]) => ({
      group,
      key: p?.key ?? '',
      label: p?.label ?? '',
    }))
    ruleRows = (s.groupRules ?? []).map((r) => ({
      group: r.group ?? '',
      projects: joinCsv(r.projects),
      labels: joinCsv(r.labels),
      components: joinCsv(r.components),
    }))
    memberRows = (s.members ?? []).map((m) => ({
      email: m.email ?? '',
      name: m.name ?? '',
      display_name: m.display_name ?? '',
      group: m.group ?? '',
      department: m.department ?? '',
      job_role: m.job_role ?? '',
      jira_account_id: m.jira_account_id ?? '',
      avatar_url: m.avatar_url ?? '',
    }))
    openMember = null

    fieldMapRows = kvRows(s.fieldMap)
    editableRows = kvRows(s.editableFields)
    bodyFieldsText = joinCsv(s.bodyFields)
    specsSupported = s.fieldSpecs !== undefined
    specRows = (s.fieldSpecs ?? []).map((sp) => ({ ...sp, ids: [...sp.ids] }))
    specsTouched = false
  }

  /** Form state → PUT payload (full replace). Do not send runtime/site. */
  function build(): ScrySettings {
    const groupLabels: Record<string, string> = {}
    const groupColors: Record<string, string> = {}
    for (const row of groupRows) {
      const key = row.key.trim()
      if (!key) continue
      if (row.label.trim()) groupLabels[key] = row.label.trim()
      if (row.color.trim()) groupColors[key] = row.color.trim()
    }
    const productByGroup: Record<string, { key: string; label: string }> = {}
    for (const row of productRows) {
      const group = row.group.trim()
      if (group) productByGroup[group] = { key: row.key.trim(), label: row.label.trim() }
    }
    const hours = Number(staleText)
    return {
      projects: projectsPickerReady ? projectKeys : splitCsv(projectsText),
      // Only when the source is on: the server rejects the key otherwise, and
      // omitting it leaves the stored scope alone.
      // `enabled` is what lets this tab turn the source on at all; the server
      // rejects a bare `spaces` while it is off. Sent whenever the section was
      // reachable, so switching it off is a save like any other.
      confluence: { enabled: confluenceOn, spaces: confluenceOn ? spaceKeys : [] },
      staleThresholdHours: Number.isFinite(hours) && hours > 0 ? hours : 72,
      syncIntervalSec: resolveInterval(syncPreset, syncCustomText),
      reconcileIntervalSec: resolveInterval(reconcilePreset, reconcileCustomText),
      qaDashboardUrl: qaDashboardUrl.trim(),
      features: { ...features },
      groupLabels,
      groupColors,
      productByGroup,
      groupRules: ruleRows
        .filter((r) => r.group.trim())
        .map((r) => ({
          group: r.group.trim(),
          projects: splitCsv(r.projects),
          labels: splitCsv(r.labels),
          components: splitCsv(r.components),
        })),
      members: memberRows
        .filter((m) => m.email.trim())
        .map((m) => ({ ...m, email: m.email.trim() })),
      fieldMap: rec(fieldMapRows),
      editableFields: rec(editableRows),
      bodyFields: splitCsv(bodyFieldsText),
      // Only when touched: the server treats absence as "keep discovery output".
      ...(specsTouched && specsSupported
        ? { fields: specRows.filter((r) => r.alias.trim() && r.ids.length > 0) }
        : {}),
    }
  }

  /** Any hand edit pins the row (auto:false) so `scry fields --apply` keeps it. */
  function touchSpec(i: number) {
    specsTouched = true
    specRows[i] = { ...specRows[i], auto: false }
  }

  async function copyText(key: string, text: string) {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      copiedKey = key
      setTimeout(() => {
        if (copiedKey === key) copiedKey = null
      }, 1500)
    } catch {
      /* clipboard may be denied — ignore */
    }
  }

  onMount(async () => {
    try {
      load(await api.getSettings())
    } catch (e) {
      error = e instanceof Error ? e.message : t('settings.loadFailed')
    }
    loading = false
  })

  /** Fetch the two scope lists once, the first time the Sources tab is shown. */
  async function loadSources() {
    if (sourcesRequested) return
    sourcesRequested = true

    // Manual entry is on screen from the first frame: asking the site for its
    // projects can take many seconds when it is unreachable, and a spinner over
    // the one field that works without the site is the wrong trade.
    /*
     * Two independent lists, two independent requests. They used to run in
     * sequence, so an unreachable Jira held the Confluence picker at "loading"
     * for as long as the socket took to give up — one dead source hiding the
     * other's controls, on the screen whose job is to configure both.
     */
    projectsLoading = true
    const projects = (async () => {
      try {
        const res = await api.getAvailableProjects()
        projectOptions = res.projects.map((p) => ({
          value: p.key,
          label: p.name,
          hint: p.projectTypeKey,
        }))
        if (!projectsTouched) {
          projectsPickerReady = true
          // The picker is now the field of record; drop the text mirror so a
          // stale string can never win the next build().
          projectsText = ''
        }
      } catch {
        // No credential (409) or the site is unreachable — the manual list stays.
        projectsPickerReady = false
      }
      projectsLoading = false
    })()

    // Loaded whether or not the source is on: picking spaces is how you decide
    // to turn it on, so the list has to arrive first.
    spacesLoading = true
    const spaces = (async () => {
      try {
        const res = await api.getSettingsSpaces()
        spaceOptions = res.spaces.map((s) => ({ value: s.key, label: s.name, hint: s.type }))
        // res.all_global_when_empty is the saved state's version of the same
        // rule; the picker reads the pending switch instead, or the label would
        // contradict the warning above it between a toggle and its save.
      } catch {
        spacesError = t('settings.spacesUnavailable')
      }
      spacesLoading = false
    })()

    await Promise.all([projects, spaces])
  }

  $effect(() => {
    if (tab === 'sources' && !loading) void loadSources()
  })

  function refreshJson() {
    jsonText = JSON.stringify(build(), null, 2)
    jsonError = null
  }

  function applyJson(text: string) {
    jsonText = text
    try {
      load(JSON.parse(text) as ScrySettings)
      jsonError = null
    } catch {
      jsonError = t('settings.jsonParseError')
    }
  }

  async function save() {
    if (saving || jsonError) return
    saving = true
    error = null
    try {
      await api.putSettings(build())
      write.toast(t('settings.savedReload'), 'success')
      setTimeout(() => location.reload(), 600)
    } catch (e) {
      error = e instanceof Error ? e.message : t('settings.saveFailed')
      saving = false
    }
  }

  function openJiraKey() {
    onclose()
    write.openSettings()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <!-- 92vh, not 88: the Sync tab runs THIS MIRROR plus four groups plus the
       personal-token entry point, and at 88vh the entry point sat 35px below
       the fold — an action nobody scrolls to find, because nothing above it
       suggests there is more. The alternative was to compress the groups, but
       they are already at 16px apart with 4px between label and control, at or
       under the floor that fix was given, so the height had to come from the
       dialog instead. 8vh still leaves 40px of backdrop above and below at a
       1000px viewport. (2026-08-06) -->
  <div
    use:trapFocus
    class="anim-pop flex max-h-[92vh] w-full max-w-3xl flex-col rounded-lg border border-border-strong bg-bg-panel shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('settings.title')}
  >
    <!-- Header + tabs -->
    <div class="flex-none border-b border-border-subtle px-5 pt-4">
      <h2 class="mb-0.5 text-title font-semibold text-text-primary">{t('settings.title')}</h2>
      <p class="mb-3 text-micro text-text-muted">
        {t('settings.introBefore')} <span class="font-mono">~/.scry/config.json</span> {t('settings.introAfter')}
      </p>
      <div class="flex gap-1">
        {#each TABS as [id, label] (id)}
          <button
            type="button"
            class="-mb-px flex h-control items-center border-b-2 px-2.5 text-[12px] transition-colors {tab === id
              ? 'border-accent text-text-primary'
              : 'border-transparent text-text-secondary hover:text-text-primary'}"
            onclick={() => (tab = id)}
          >
            {label}
          </button>
        {/each}
      </div>
    </div>

    <!-- Body -->
    <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-[12px]">
      {#if loading}
        <p class="py-8 text-center text-text-muted">{t('settings.loading')}</p>
      {:else}
        <!-- This mirror — read-only runtime facts (always above tab content) -->
        {#if runtime}
          <section
            class="mb-4 rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
            aria-label={t('settings.thisMirror')}
          >
            <div class="mb-2 text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.thisMirror')}
            </div>
            <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-micro">
              <dt class="text-text-muted">{t('settings.runtimeProfile')}</dt>
              <dd class="font-mono text-text-primary">{runtime.profile}</dd>

              <dt class="text-text-muted">{t('settings.runtimeDb')}</dt>
              <dd class="min-w-0">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="break-all font-mono text-text-primary">{runtime.dbPath || t('settings.none')}</span>
                  {#if runtime.dbPath}
                    <button type="button" class={COPY_BTN} onclick={() => copyText('db', runtime!.dbPath)}>
                      {copiedKey === 'db' ? t('settings.copied') : t('settings.copy')}
                    </button>
                    <button
                      type="button"
                      class={COPY_BTN}
                      title={t('settings.copySqlite')}
                      onclick={() => copyText('sqlite', `sqlite3 ${runtime!.dbPath}`)}
                    >
                      {copiedKey === 'sqlite' ? t('settings.copied') : 'sqlite3'}
                    </button>
                  {/if}
                </div>
                <div class="mt-0.5 text-text-muted">
                  {runtime.dbSizeHuman}
                  {#if runtime.dbModifiedAt}
                    · {t('settings.runtimeModified')} {runtime.dbModifiedAt}
                  {/if}
                </div>
              </dd>

              <dt class="text-text-muted">{t('settings.runtimeConfig')}</dt>
              <dd class="min-w-0">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="break-all font-mono text-text-primary">{runtime.configPath || t('settings.none')}</span>
                  {#if runtime.configPath}
                    <button
                      type="button"
                      class={COPY_BTN}
                      onclick={() => copyText('cfg', runtime!.configPath)}
                    >
                      {copiedKey === 'cfg' ? t('settings.copied') : t('settings.copy')}
                    </button>
                  {/if}
                </div>
              </dd>

              <dt class="text-text-muted">{t('settings.runtimeCounts')}</dt>
              <dd class="text-text-primary">
                {t('settings.runtimeIssues', { n: runtime.issueCount })}
                · {t('settings.runtimeComments', { n: runtime.commentCount })}
              </dd>

              <dt class="text-text-muted">{t('settings.runtimeSchema')}</dt>
              <dd class="font-mono text-text-primary">{runtime.schemaVersion}</dd>

              <dt class="text-text-muted">{t('settings.runtimeWatermark')}</dt>
              <dd class="font-mono text-text-primary">{runtime.watermark || t('settings.none')}</dd>

              <dt class="text-text-muted">{t('settings.runtimeFullSync')}</dt>
              <dd class="font-mono text-text-primary">{runtime.lastFullSyncAt || t('settings.none')}</dd>

              {#if runtime.lastError}
                <dt class="text-text-muted">{t('settings.runtimeLastError')}</dt>
                <dd class="break-all text-status-reopen">{runtime.lastError}</dd>
              {/if}

              {#if apiUsage}
                <dt class="text-text-muted">{t('settings.runtimeApiCalls')}</dt>
                <dd class="text-text-primary">
                  {t('settings.runtimeApiToday', { n: apiUsage.today.requests })}
                  {#if apiUsage.last_7_days.requests !== apiUsage.today.requests}
                    · {t('settings.runtimeApiWeek', { n: apiUsage.last_7_days.requests })}
                  {/if}
                  {#if apiUsage.last_7_days.throttled > 0}
                    · <span class="text-status-reopen"
                      >{t('settings.runtimeApiThrottled', { n: apiUsage.last_7_days.throttled })}</span
                    >
                  {/if}
                </dd>
              {/if}

              <dt class="text-text-muted">{t('settings.runtimeVersion')}</dt>
              <dd class="font-mono text-text-primary">{runtime.scryVersion}</dd>
            </dl>
          </section>
        {/if}

        {#if tab === 'sync'}
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('settings.syncInterval')}</span>
            <div class="flex flex-wrap items-center gap-2">
              <!-- selected on the option, not value on the select: a plain
                   value attribute applies before the #each options mount and
                   never re-syncs, leaving the control visibly empty. -->
              <span class="relative flex">
                <select
                  class="{SELECT} w-auto max-w-[12rem]"
                  onchange={(e) => {
                    syncPreset = Number(e.currentTarget.value)
                    if (syncPreset !== -1) syncCustomText = ''
                  }}
                >
                  {#each SYNC_PRESETS as p (p.value)}
                    <option value={p.value} selected={p.value === syncPreset}>
                      {p.label}{p.value === 0 ? ` (${defaultSyncSec}s)` : ''}
                    </option>
                  {/each}
                </select>
                <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
              </span>
              {#if syncPreset === -1}
                <input
                  class="{INPUT} w-28"
                  type="number"
                  min="15"
                  step="1"
                  bind:value={syncCustomText}
                  placeholder={String(defaultSyncSec)}
                  aria-label={t('settings.syncInterval')}
                />
                <span class="text-micro text-text-muted">{t('settings.intervalSeconds')}</span>
              {/if}
            </div>
            <span class="text-micro text-text-muted">{t('settings.syncIntervalHint')}</span>
          </div>

          <div class="flex flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('settings.reconcileInterval')}</span>
            <div class="flex flex-wrap items-center gap-2">
              <span class="relative flex">
                <select
                  class="{SELECT} w-auto max-w-[12rem]"
                  onchange={(e) => {
                    reconcilePreset = Number(e.currentTarget.value)
                    if (reconcilePreset !== -1) reconcileCustomText = ''
                  }}
                >
                  {#each RECONCILE_PRESETS as p (p.value)}
                    <option value={p.value} selected={p.value === reconcilePreset}>
                      {p.label}{p.value === 0 ? ` (${defaultReconcileSec}s)` : ''}
                    </option>
                  {/each}
                </select>
                <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
              </span>
              {#if reconcilePreset === -1}
                <input
                  class="{INPUT} w-28"
                  type="number"
                  min="300"
                  step="1"
                  bind:value={reconcileCustomText}
                  placeholder={String(defaultReconcileSec)}
                  aria-label={t('settings.reconcileInterval')}
                />
                <span class="text-micro text-text-muted">{t('settings.intervalSeconds')}</span>
              {/if}
            </div>
            <span class="text-micro text-text-muted">{t('settings.reconcileIntervalHint')}</span>
          </div>
          <p class="text-micro leading-relaxed text-text-muted">{t('settings.intervalApplies')}</p>

          <label class="flex max-w-[200px] flex-col gap-1">
            <span class="text-micro text-text-secondary">{t('settings.staleHours')}</span>
            <input class={INPUT} type="number" min="1" bind:value={staleText} />
            <span class="text-micro text-text-muted">
              {t('settings.staleHint')}
            </span>
          </label>
          <div class="border-t border-border-subtle pt-3">
            <button
              type="button"
              class={ADD_BTN}
              onclick={openJiraKey}
            >
              {t('settings.personalToken')}
            </button>
            <p class="mt-1 text-micro text-text-muted">
              {t('settings.credsElsewhere')}
            </p>
          </div>
        </div>
      {:else if tab === 'sources'}
        <div class="flex flex-col gap-5" data-testid="settings-sources">
          <!-- Jira projects -->
          {#if projectsPickerReady}
            <ScopePicker
              label={t('settings.sourcesProjects')}
              hint={t('settings.sourcesProjectsHint')}
              options={projectOptions}
              bind:selected={projectKeys}
              placeholder={t('settings.scopeProjectPlaceholder')}
              emptyLabel={t('settings.sourcesNoProjects')}
              testid="scope-projects"
            />
          {:else}
            <label class="flex flex-col gap-1" data-testid="scope-projects-fallback">
              <span class="text-micro text-text-secondary">{t('settings.projects')}</span>
              <input
                class={INPUT}
                bind:value={projectsText}
                oninput={() => (projectsTouched = true)}
                placeholder="NMB, NMA"
              />
              <span class="text-micro text-text-muted">
                {projectsLoading ? t('settings.scopeLoading') : t('settings.projectsManual')}
              </span>
            </label>
          {/if}

          <!--
            Confluence. Present whether the source is on or off — while it was
            hidden, a profile with the source off looked exactly like a build
            without the feature, and this screen was the only place it could
            have been turned on.

            Order is deliberate: choose spaces, then turn it on. The button says
            what it will mirror, because with nothing selected the answer is
            "the whole wiki" and that must be a sentence someone read, not the
            side effect of a generic Turn on.
          -->
          <div class="border-t border-border-subtle pt-4" data-testid="sources-confluence">
            <div class="mb-2.5 flex items-start justify-between gap-3">
              <div class="min-w-0">
                <!-- Same weight as the Jira projects label above: they are two
                     sources of one mirror, and a bolder heading here made
                     Confluence outrank Jira on a scan. -->
                <span class="text-micro text-text-secondary">
                  {t('settings.confluenceTitle')}
                </span>
                <!--
                  One state line, and the consequence lives in it rather than in
                  an extra line underneath. The block sits near the bottom of a
                  scrolling dialog, so a line added below the button rendered
                  under the fold at exactly the scroll position where the button
                  is clicked — the warning was invisible at the moment it was
                  earned.
                -->
                <p
                  class="mt-0.5 text-micro leading-relaxed {confluenceOn && spaceKeys.length === 0
                    ? 'text-status-stale'
                    : 'text-text-muted'}"
                  data-testid={confluenceOn && spaceKeys.length === 0
                    ? 'confluence-all-warning'
                    : undefined}
                >
                  {#if confluenceOn && spaceKeys.length === 0}
                    {t('settings.confluenceAllWarning')}
                  {:else if confluenceOn}
                    {t('settings.confluenceOnHint')}
                  {:else}
                    {t('settings.confluenceOffHint')}
                  {/if}
                </p>
              </div>
              <!-- Both states use the dialog's secondary button. The filled
                   accent belongs to Save alone: this control changes a pending
                   value like every other field here, and a second primary made
                   it look like it committed on click. -->
              {#if confluenceOn}
                <button
                  type="button"
                  class="{ADD_BTN} flex-none self-start"
                  onclick={() => {
                    confluenceOn = false
                    // Scope goes with it: a stored selection under an off
                    // source is a promise nothing keeps, and leaving it would
                    // make the effect above switch the source straight back on.
                    spaceKeys = []
                  }}
                  data-testid="confluence-turn-off"
                >
                  {t('settings.confluenceTurnOff')}
                </button>
              {:else}
                <button
                  type="button"
                  class="{ADD_BTN} flex-none self-start"
                  onclick={() => (confluenceOn = true)}
                  data-testid="confluence-turn-on"
                >
                  {spaceKeys.length
                    ? t('settings.confluenceTurnOnCount', { n: String(spaceKeys.length) })
                    : t('settings.confluenceTurnOnAll')}
                </button>
              {/if}
            </div>

            {#if spacesLoading}
              <p class="text-[12px] text-text-muted">{t('settings.scopeLoading')}</p>
            {:else if spacesError}
              <p class="text-[12px] text-status-stale" data-testid="scope-spaces-error">
                {spacesError}
              </p>
            {:else}
              <!-- The hint reads "Only these spaces are mirrored", which is
                   false with nothing selected — that is precisely the case
                   where every team space is — so it waits for a selection to
                   describe rather than contradicting the line above it. -->
              <ScopePicker
                label={t('settings.sourcesSpaces')}
                hint={spaceKeys.length ? t('settings.sourcesSpacesHint') : ''}
                options={visibleSpaceOptions}
                bind:selected={spaceKeys}
                placeholder={t('settings.scopeSpacePlaceholder')}
                emptyLabel={confluenceOn
                  ? t('settings.sourcesAllGlobal')
                  : t('settings.sourcesNoSpaces')}
                testid="scope-spaces"
              >
                {#snippet action()}
                  <label class="flex cursor-pointer items-center gap-1.5 text-micro text-text-muted">
                    <input
                      type="checkbox"
                      class="accent-[var(--color-accent,#3b82f6)]"
                      bind:checked={showPersonalSpaces}
                    />
                    {t('settings.showPersonalSpaces')}
                  </label>
                {/snippet}
              </ScopePicker>
            {/if}
          </div>

          <p class="border-t border-border-subtle pt-3 text-micro leading-relaxed text-text-muted">
            {t('settings.sourcesApplyHint')}
          </p>
        </div>
      {:else if tab === 'features'}
        <div class="flex flex-col gap-2.5">
          {#each FEATURES as [key, label, hint] (key)}
            <label class="flex cursor-pointer items-start gap-2.5">
              <input
                type="checkbox"
                class="mt-0.5 flex-none accent-[var(--color-accent,#3b82f6)]"
                bind:checked={features[key]}
              />
              <span class="min-w-0">
                <span class="text-text-primary">{label}</span>
                <span class="block text-micro leading-relaxed text-text-muted">{hint}</span>
              </span>
            </label>
          {/each}
          <!-- In-tab browser Notification permission (not web push / VAPID). -->
          <div class="mt-1 flex items-start gap-2.5 border-t border-border-subtle pt-3">
            <span class="min-w-0 flex-1">
              <span class="text-text-primary">{t('settings.browserNotify')}</span>
              <span class="block text-micro leading-relaxed text-text-muted">
                {t('settings.browserNotifyDesc')}
              </span>
              {#if me.browserNotifyPermission === 'granted'}
                <span class="mt-1 block text-micro text-text-secondary">{t('settings.browserNotifyGranted')}</span>
              {:else if me.browserNotifyPermission === 'denied'}
                <span class="mt-1 block text-micro text-status-reopen">{t('settings.browserNotifyDenied')}</span>
              {:else if me.browserNotifyPermission === 'unsupported'}
                <span class="mt-1 block text-micro text-text-muted">{t('settings.browserNotifyUnsupported')}</span>
              {/if}
            </span>
            {#if me.browserNotifyPermission === 'default'}
              <button
                type="button"
                class="inline-flex h-control-sm flex-none items-center rounded-md border border-border-strong px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                onclick={() => void me.requestBrowserNotificationPermission()}
              >
                {t('settings.browserNotifyEnable')}
              </button>
            {/if}
          </div>
          <label class="mt-2 flex flex-col gap-1 border-t border-border-subtle pt-3">
            <span class="text-micro text-text-secondary">{t('settings.qaDashboardUrl')}</span>
            <input class={INPUT} bind:value={qaDashboardUrl} placeholder="https://qa.example.com" />
          </label>
        </div>
      {:else if tab === 'groups'}
        <div class="flex flex-col gap-5">
          <!-- Group labels + colors -->
          <div class="flex flex-col gap-1.5">
            <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupLabels')}
            </div>
            <div class="flex gap-1.5 text-micro text-text-muted">
              <span class="flex-1">{t('settings.groupKey')}</span>
              <span class="flex-1">{t('settings.label')}</span>
              <span class="w-16 flex-none">{t('settings.color')}</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each groupRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="cloud" />
                <input class="{INPUT} flex-1" bind:value={row.label} placeholder={t('settings.cloudPart')} />
                <input
                  type="color"
                  class="h-control w-16 flex-none rounded-md border border-border-strong bg-bg-base"
                  value={row.color || '#888888'}
                  oninput={(e) => (row.color = e.currentTarget.value)}
                  title={row.color || t('common.unspecified')}
                />
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => (groupRows = groupRows.filter((_, j) => j !== i))}
                  ><Icon name="x" size={13} /></button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() => (groupRows = [...groupRows, { key: '', label: '', color: '' }])}
              >{t('settings.addRow')}</button
            >
          </div>

          <!-- Product buckets -->
          <div class="flex flex-col gap-1.5">
            <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupToProduct')}
            </div>
            <div class="flex gap-1.5 text-micro text-text-muted">
              <span class="flex-1">{t('settings.groupKey')}</span>
              <span class="flex-1">{t('settings.productKey')}</span>
              <span class="flex-1">{t('settings.productLabel')}</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each productRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1 font-mono" bind:value={row.group} placeholder="cloud" />
                <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="CLOUD" />
                <input class="{INPUT} flex-1" bind:value={row.label} placeholder="Cloud" />
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => (productRows = productRows.filter((_, j) => j !== i))}
                  ><Icon name="x" size={13} /></button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() =>
                (productRows = [...productRows, { group: '', key: '', label: '' }])}
              >{t('settings.addRow')}</button
            >
          </div>

          <!-- Group classification rules -->
          <div class="flex flex-col gap-1.5">
            <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupRules')}
            </div>
            <p class="text-micro leading-relaxed text-text-muted">
              {t('settings.rulesTopDown')} <span class="text-text-secondary">{t('settings.rulesFirstWins')}</span>{t('settings.rulesDetail')}
            </p>
            <div class="flex gap-1.5 text-micro text-text-muted">
              <span class="w-24 flex-none">{t('common.group')}</span>
              <span class="flex-1">{t('settings.projectsCol')}</span>
              <span class="flex-1">{t('settings.label')}</span>
              <span class="flex-1">{t('settings.componentsCol')}</span>
              <span class="w-6 flex-none"></span>
            </div>
            {#each ruleRows as row, i (i)}
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} w-24 flex-none font-mono" bind:value={row.group} placeholder="cloud" />
                <input class="{INPUT} flex-1" bind:value={row.projects} placeholder="NMA, NMB" />
                <input class="{INPUT} flex-1" bind:value={row.labels} placeholder="backend" />
                <input class="{INPUT} flex-1" bind:value={row.components} placeholder="api" />
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => (ruleRows = ruleRows.filter((_, j) => j !== i))}
                  ><Icon name="x" size={13} /></button
                >
              </div>
            {/each}
            <button
              type="button"
              class={ADD_BTN}
              onclick={() =>
                (ruleRows = [
                  ...ruleRows,
                  { group: '', projects: '', labels: '', components: '' },
                ])}>{t('settings.addRow')}</button
            >
          </div>
        </div>
      {:else if tab === 'members'}
        <div class="flex flex-col gap-1.5">
          <div class="flex gap-1.5 text-micro text-text-muted">
            <span class="flex-1">{t('settings.memberEmail')}</span>
            <span class="w-24 flex-none">{t('settings.memberName')}</span>
            <span class="w-20 flex-none">{t('common.group')}</span>
            <span class="flex-1">Jira accountId</span>
            <span class="w-12 flex-none"></span>
          </div>
          {#each memberRows as row, i (i)}
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center gap-1.5">
                <input class="{INPUT} flex-1" bind:value={row.email} placeholder="a@b.c" />
                <input class="{INPUT} w-24 flex-none" bind:value={row.name} />
                <input class="{INPUT} w-20 flex-none font-mono" bind:value={row.group} />
                <input class="{INPUT} flex-1 font-mono" bind:value={row.jira_account_id} />
                <button
                  type="button"
                  class="flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-text-primary"
                  title={t('common.detail')}
                  onclick={() => (openMember = openMember === i ? null : i)}
                  ><Icon
                    name="chevron-right"
                    size={13}
                    class="transition-transform {openMember === i ? 'rotate-90' : ''}"
                  /></button
                >
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => {
                    memberRows = memberRows.filter((_, j) => j !== i)
                    openMember = null
                  }}><Icon name="x" size={13} /></button
                >
              </div>
              {#if openMember === i}
                <div class="ml-2 grid grid-cols-2 gap-1.5 border-l border-border-subtle pl-3 pb-1">
                  <label class="flex flex-col gap-0.5">
                    <span class="text-micro text-text-muted">{t('settings.displayName')}</span>
                    <input class={INPUT} bind:value={row.display_name} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-micro text-text-muted">{t('settings.department')}</span>
                    <input class={INPUT} bind:value={row.department} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-micro text-text-muted">{t('settings.jobTitle')}</span>
                    <input class={INPUT} bind:value={row.job_role} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-micro text-text-muted">{t('settings.avatarUrl')}</span>
                    <input class={INPUT} bind:value={row.avatar_url} />
                  </label>
                </div>
              {/if}
            </div>
          {/each}
          <button
            type="button"
            class={ADD_BTN}
            onclick={() =>
              (memberRows = [
                ...memberRows,
                {
                  email: '',
                  name: '',
                  display_name: '',
                  group: '',
                  department: '',
                  job_role: '',
                  jira_account_id: '',
                  avatar_url: '',
                },
              ])}>{t('settings.addMember')}</button
          >
        </div>
      {:else}
        <div class="flex flex-col gap-5">
          {#if specsSupported}
            <div class="flex flex-col gap-1.5">
              <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
                {t('settings.discoveredFields')}
              </div>
              <p class="text-micro text-text-muted">{t('settings.discoveredFieldsHint')}</p>
              {#if specRows.length === 0}
                <p class="text-[12px] text-text-secondary">{t('settings.noDiscoveredFields')}</p>
              {:else}
                <div class="flex flex-col gap-1">
                  {#each specRows as spec, i (spec.alias)}
                    <div class="flex items-center gap-2">
                      <span class="w-40 truncate text-[12px] text-text-primary" title={spec.alias}>
                        {spec.label}
                        {#if spec.auto === false}
                          <span class="ml-1 text-micro text-accent">{t('settings.pinned')}</span>
                        {/if}
                      </span>
                      <span class="relative flex">
                        <select
                          class="{SELECT} w-24"
                          value={spec.role}
                          onchange={(e) => {
                            touchSpec(i)
                            specRows[i].role = e.currentTarget.value
                          }}
                        >
                          <option value="facet">{t('settings.roleFacet')}</option>
                          <option value="body">{t('settings.roleBody')}</option>
                          <option value="user">{t('settings.roleUser')}</option>
                          <option value="plain">{t('settings.rolePlain')}</option>
                        </select>
                        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
                      </span>
                      <span class="relative flex">
                        <select
                          class="{SELECT} w-32"
                          value={spec.kind ?? ''}
                          onchange={(e) => {
                            touchSpec(i)
                            specRows[i].kind = e.currentTarget.value || undefined
                          }}
                        >
                          <option value="">{t('settings.kindNone')}</option>
                          <option value="option">option</option>
                          <option value="multi_option">multi_option</option>
                          <option value="user">user</option>
                          <option value="version_array">version_array</option>
                        </select>
                        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
                      </span>
                      <button
                        type="button"
                        class="text-micro text-text-muted hover:text-status-reopen"
                        onclick={() => {
                          specsTouched = true
                          specRows = specRows.filter((_, j) => j !== i)
                        }}>{t('settings.removeField')}</button
                      >
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
          <div class="flex flex-col gap-1.5">
            <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.fieldMap')}
            </div>
            <KeyValueRows
              bind:rows={fieldMapRows}
              keyLabel={t('settings.alias')}
              valueLabel={t('settings.jiraFieldId')}
              keyPlaceholder="severity"
              valuePlaceholder="customfield_10050"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
              {t('settings.editableFields')}
            </div>
            <KeyValueRows
              bind:rows={editableRows}
              keyLabel={t('settings.alias')}
              valueLabel={t('settings.jiraFieldId')}
              keyPlaceholder="solution"
              valuePlaceholder="customfield_10092"
            />
          </div>
          <label class="flex flex-col gap-1">
            <span class="text-micro text-text-secondary">
              {t('settings.adfSearchFields')}
            </span>
            <input class="{INPUT} font-mono" bind:value={bodyFieldsText} placeholder="customfield_10101" />
          </label>
        </div>
      {/if}

      <!-- Advanced: raw JSON -->
        <details
          class="mt-5 border-t border-border-subtle pt-3"
          ontoggle={(e) => {
            if (e.currentTarget.open) refreshJson()
          }}
        >
          <summary class="cursor-pointer text-micro text-text-secondary hover:text-text-primary">
            {t('settings.advancedJson')}
          </summary>
          <textarea
            class="mt-2 h-56 w-full rounded-md border border-border-strong bg-bg-base p-2 font-mono text-micro text-text-primary outline-none focus:border-accent"
            spellcheck="false"
            value={jsonText}
            oninput={(e) => applyJson(e.currentTarget.value)}
          ></textarea>
          {#if jsonError}
            <p class="mt-1 text-micro text-status-reopen">{jsonError}</p>
          {:else}
            <p class="mt-1 text-micro text-text-muted">
              {t('settings.jsonHint')}
            </p>
          {/if}
        </details>
      {/if}
    </div>


    <!-- locale -->
    <div class="flex flex-none items-center gap-2 border-t border-border-subtle px-5 py-2">
      <label class="flex items-center gap-2 text-[12px] text-text-secondary">
        <span>{t('settings.locale')}</span>
        <span class="relative flex">
          <select
            class="{SELECT} w-auto"
            value={locale()}
            onchange={(e) => setLocale(e.currentTarget.value as Locale)}
          >
            <option value="en">{t('settings.localeEn')}</option>
            <option value="ko">{t('settings.localeKo')}</option>
          </select>
          <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
        </span>
      </label>
    </div>

    <!-- Footer -->
    <div class="flex flex-none items-center justify-between gap-2 border-t border-border-subtle px-5 py-3">
      <span class="min-w-0 flex-1 truncate text-[12px] text-status-reopen">{error ?? ''}</span>
      <button
        type="button"
        onclick={onclose}
        class="inline-flex h-control items-center rounded-md px-3 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.close')}
      </button>
      <button
        type="button"
        onclick={save}
        disabled={loading || saving || !!jsonError}
        class="inline-flex h-control items-center rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
      >
        {saving ? t('common.saving') : t('common.save')}
      </button>
    </div>
  </div>
</div>
