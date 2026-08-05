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
  import type { ScrySettings, SettingsRuntime } from '../../lib/api'
  import type { ScryFeatures } from '../../lib/config'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import KeyValueRows from './KeyValueRows.svelte'
  import { trapFocus } from '../../lib/focus-trap'

  let { onclose }: { onclose: () => void } = $props()

  type Tab = 'sync' | 'features' | 'groups' | 'members' | 'fields'
  const TABS: [Tab, string][] = [
    ['sync', t('settings.tabSync')],
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
    'w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent'
  const ADD_BTN =
    'self-start rounded-md border border-border-strong px-2 py-1 text-[11px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
  const DEL_BTN =
    'w-6 flex-none text-[12px] text-text-muted transition-colors hover:text-status-reopen'
  const COPY_BTN =
    'rounded border border-border-strong px-1.5 py-0.5 text-[10px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'

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
    projectsText = joinCsv(s.projects)
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
      projects: splitCsv(projectsText),
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
    }
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
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    use:trapFocus
    class="anim-pop flex max-h-[88vh] w-full max-w-3xl flex-col rounded-lg border border-border-strong bg-bg-panel shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label={t('settings.title')}
  >
    <!-- Header + tabs -->
    <div class="flex-none border-b border-border-subtle px-5 pt-4">
      <h2 class="mb-0.5 text-[14px] font-semibold text-text-primary">{t('settings.title')}</h2>
      <p class="mb-3 text-[11px] text-text-muted">
        {t('settings.introBefore')} <span class="font-mono">~/.scry/config.json</span> {t('settings.introAfter')}
      </p>
      <div class="flex gap-1">
        {#each TABS as [id, label] (id)}
          <button
            type="button"
            class="-mb-px border-b-2 px-2.5 py-1.5 text-[12px] transition-colors {tab === id
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
            <div class="mb-2 text-[11px] font-medium uppercase tracking-wide text-text-muted">
              {t('settings.thisMirror')}
            </div>
            <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-[11px]">
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
          <label class="flex flex-col gap-1">
            <span class="text-[11px] text-text-secondary">{t('settings.projects')}</span>
            <input class={INPUT} bind:value={projectsText} placeholder="NMB, NMA" />
          </label>

          <div class="flex flex-col gap-1">
            <span class="text-[11px] text-text-secondary">{t('settings.syncInterval')}</span>
            <div class="flex flex-wrap items-center gap-2">
              <select
                class="{INPUT} w-auto max-w-[12rem]"
                value={String(syncPreset)}
                onchange={(e) => {
                  syncPreset = Number(e.currentTarget.value)
                  if (syncPreset !== -1) syncCustomText = ''
                }}
              >
                {#each SYNC_PRESETS as p (p.value)}
                  <option value={p.value}>
                    {p.label}{p.value === 0 ? ` (${defaultSyncSec}s)` : ''}
                  </option>
                {/each}
              </select>
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
                <span class="text-[11px] text-text-muted">{t('settings.intervalSeconds')}</span>
              {/if}
            </div>
            <span class="text-[11px] text-text-muted">{t('settings.syncIntervalHint')}</span>
          </div>

          <div class="flex flex-col gap-1">
            <span class="text-[11px] text-text-secondary">{t('settings.reconcileInterval')}</span>
            <div class="flex flex-wrap items-center gap-2">
              <select
                class="{INPUT} w-auto max-w-[12rem]"
                value={String(reconcilePreset)}
                onchange={(e) => {
                  reconcilePreset = Number(e.currentTarget.value)
                  if (reconcilePreset !== -1) reconcileCustomText = ''
                }}
              >
                {#each RECONCILE_PRESETS as p (p.value)}
                  <option value={p.value}>
                    {p.label}{p.value === 0 ? ` (${defaultReconcileSec}s)` : ''}
                  </option>
                {/each}
              </select>
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
                <span class="text-[11px] text-text-muted">{t('settings.intervalSeconds')}</span>
              {/if}
            </div>
            <span class="text-[11px] text-text-muted">{t('settings.reconcileIntervalHint')}</span>
          </div>
          <p class="text-[11px] leading-relaxed text-text-muted">{t('settings.intervalApplies')}</p>

          <label class="flex max-w-[200px] flex-col gap-1">
            <span class="text-[11px] text-text-secondary">{t('settings.staleHours')}</span>
            <input class={INPUT} type="number" min="1" bind:value={staleText} />
            <span class="text-[11px] text-text-muted">
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
            <p class="mt-1 text-[11px] text-text-muted">
              {t('settings.credsElsewhere')}
            </p>
          </div>
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
                <span class="block text-[11px] leading-relaxed text-text-muted">{hint}</span>
              </span>
            </label>
          {/each}
          <!-- In-tab browser Notification permission (not web push / VAPID). -->
          <div class="mt-1 flex items-start gap-2.5 border-t border-border-subtle pt-3">
            <span class="min-w-0 flex-1">
              <span class="text-text-primary">{t('settings.browserNotify')}</span>
              <span class="block text-[11px] leading-relaxed text-text-muted">
                {t('settings.browserNotifyDesc')}
              </span>
              {#if me.browserNotifyPermission === 'granted'}
                <span class="mt-1 block text-[11px] text-text-secondary">{t('settings.browserNotifyGranted')}</span>
              {:else if me.browserNotifyPermission === 'denied'}
                <span class="mt-1 block text-[11px] text-status-reopen">{t('settings.browserNotifyDenied')}</span>
              {:else if me.browserNotifyPermission === 'unsupported'}
                <span class="mt-1 block text-[11px] text-text-muted">{t('settings.browserNotifyUnsupported')}</span>
              {/if}
            </span>
            {#if me.browserNotifyPermission === 'default'}
              <button
                type="button"
                class="flex-none rounded-md border border-border-strong px-2 py-1 text-[11px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
                onclick={() => void me.requestBrowserNotificationPermission()}
              >
                {t('settings.browserNotifyEnable')}
              </button>
            {/if}
          </div>
          <label class="mt-2 flex flex-col gap-1 border-t border-border-subtle pt-3">
            <span class="text-[11px] text-text-secondary">{t('settings.qaDashboardUrl')}</span>
            <input class={INPUT} bind:value={qaDashboardUrl} placeholder="https://qa.example.com" />
          </label>
        </div>
      {:else if tab === 'groups'}
        <div class="flex flex-col gap-5">
          <!-- Group labels + colors -->
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupLabels')}
            </div>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
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
                  class="h-[26px] w-16 flex-none rounded-md border border-border-strong bg-bg-base"
                  value={row.color || '#888888'}
                  oninput={(e) => (row.color = e.currentTarget.value)}
                  title={row.color || t('common.unspecified')}
                />
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => (groupRows = groupRows.filter((_, j) => j !== i))}>✕</button
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
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupToProduct')}
            </div>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
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
                  onclick={() => (productRows = productRows.filter((_, j) => j !== i))}>✕</button
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
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
              {t('settings.groupRules')}
            </div>
            <p class="text-[11px] leading-relaxed text-text-muted">
              {t('settings.rulesTopDown')} <span class="text-text-secondary">{t('settings.rulesFirstWins')}</span>{t('settings.rulesDetail')}
            </p>
            <div class="flex gap-1.5 text-[11px] text-text-muted">
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
                  onclick={() => (ruleRows = ruleRows.filter((_, j) => j !== i))}>✕</button
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
          <div class="flex gap-1.5 text-[11px] text-text-muted">
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
                  class="w-6 flex-none text-[12px] text-text-muted transition-colors hover:text-text-primary"
                  title={t('common.detail')}
                  onclick={() => (openMember = openMember === i ? null : i)}
                  >{openMember === i ? '▾' : '▸'}</button
                >
                <button
                  type="button"
                  class={DEL_BTN}
                  title={t('settings.deleteRow')}
                  onclick={() => {
                    memberRows = memberRows.filter((_, j) => j !== i)
                    openMember = null
                  }}>✕</button
                >
              </div>
              {#if openMember === i}
                <div class="ml-2 grid grid-cols-2 gap-1.5 border-l border-border-subtle pl-3 pb-1">
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">{t('settings.displayName')}</span>
                    <input class={INPUT} bind:value={row.display_name} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">{t('settings.department')}</span>
                    <input class={INPUT} bind:value={row.department} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">{t('settings.jobTitle')}</span>
                    <input class={INPUT} bind:value={row.job_role} />
                  </label>
                  <label class="flex flex-col gap-0.5">
                    <span class="text-[11px] text-text-muted">{t('settings.avatarUrl')}</span>
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
          <div class="flex flex-col gap-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
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
            <div class="text-[11px] font-medium uppercase tracking-wide text-text-muted">
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
            <span class="text-[11px] text-text-secondary">
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
          <summary class="cursor-pointer text-[11px] text-text-secondary hover:text-text-primary">
            {t('settings.advancedJson')}
          </summary>
          <textarea
            class="mt-2 h-56 w-full rounded-md border border-border-strong bg-bg-base p-2 font-mono text-[11px] text-text-primary outline-none focus:border-accent"
            spellcheck="false"
            value={jsonText}
            oninput={(e) => applyJson(e.currentTarget.value)}
          ></textarea>
          {#if jsonError}
            <p class="mt-1 text-[11px] text-status-reopen">{jsonError}</p>
          {:else}
            <p class="mt-1 text-[11px] text-text-muted">
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
        <select
          class="rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent"
          value={locale()}
          onchange={(e) => setLocale(e.currentTarget.value as Locale)}
        >
          <option value="en">{t('settings.localeEn')}</option>
          <option value="ko">{t('settings.localeKo')}</option>
        </select>
      </label>
    </div>

    <!-- Footer -->
    <div class="flex flex-none items-center justify-between gap-2 border-t border-border-subtle px-5 py-3">
      <span class="min-w-0 flex-1 truncate text-[12px] text-status-reopen">{error ?? ''}</span>
      <button
        type="button"
        onclick={onclose}
        class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.close')}
      </button>
      <button
        type="button"
        onclick={save}
        disabled={loading || saving || !!jsonError}
        class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
      >
        {saving ? t('common.saving') : t('common.save')}
      </button>
    </div>
  </div>
</div>
