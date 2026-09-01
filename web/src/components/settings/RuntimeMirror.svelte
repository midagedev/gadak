<script lang="ts">
  /*
   * Read-only instance facts, at the foot of the Sync tab: which mirror am I
   * editing, where does it live, how much is in it, when did it last pull.
   *
   * One place, not one per tab (GDK-188). Repeated above every tab's content it
   * pushed each tab's own subject down to say the same things again, and none of
   * those things were about members, fields or groups. They are all about the
   * state of the sync, which is the tab that now owns it.
   */
  import { t } from '../../lib/i18n'
  import { copyText } from '../../lib/copy-text'
  import { config, surface } from '../../lib/config'
  import { isLocalOrigin, STANDALONE_INIT_COMMAND } from '../../lib/workspace'
  import type { SettingsRuntime } from '../../lib/api'
  import { COPY_BTN } from './controls'

  const onDesktop = surface() === 'desktop'
  const localOrigin = isLocalOrigin(config())

  let { runtime }: { runtime: SettingsRuntime } = $props()

  // Hidden until there is traffic to report: a row that only ever says zero
  // teaches nothing, and zero here means "nothing flushed yet", not "headroom".
  const apiUsage = $derived(
    runtime.apiUsage && runtime.apiUsage.last_7_days.requests > 0 ? runtime.apiUsage : null,
  )
  let copiedKey = $state<string | null>(null)

  async function copyValue(key: string, text: string) {
    if (!text) return
    // copy-text.ts owns the desktop-vs-web transport (GDK-178); the copied
    // state only ever shows on a write that actually happened.
    if (await copyText(text)) {
      copiedKey = key
      setTimeout(() => {
        if (copiedKey === key) copiedKey = null
      }, 1500)
    }
  }
</script>

<!-- No outer margin: the tab is a gap-4 column and owns the spacing. -->
<section
  class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
  aria-label={t('settings.thisMirror')}
  data-testid="runtime-mirror"
>
  <div class="mb-2 text-micro font-medium uppercase tracking-wide text-text-muted">
    {t('settings.thisMirror')}
  </div>
  <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-micro">
    <dt class="text-text-muted">{t('settings.runtimeProfile')}</dt>
    <dd class="min-w-0">
      <div class="flex min-w-0 flex-wrap items-center gap-1.5">
        <span class="font-mono text-text-primary">{runtime.profile}</span>
        {#if localOrigin}
          <!-- Same status-pill classes as IntegrationsTab's install-state chip.
               data-kind carries the stored kind value ("standalone"), not the
               GDK-1278 vocabulary — it is a DOM contract e2e reads. -->
          <span
            class="inline-flex items-center gap-1.5 rounded-full border border-border-subtle px-1.5 py-0.5 text-micro text-text-secondary"
            data-testid="workspace-kind"
            data-kind="standalone"
            title={t('settings.workspaceLocalOriginHint')}
            aria-label={t('settings.workspaceLocalOriginHint')}
          >
            {t('settings.workspaceLocalOrigin')}
          </span>
        {/if}
      </div>
      {#if runtime.profile && runtime.profile !== 'default'}
        <div class="mt-0.5 font-mono text-text-muted" data-testid="runtime-cli-flag">
          {t('settings.runtimeCli', { name: runtime.profile })}
        </div>
      {/if}
    </dd>

    <dt class="text-text-muted">{t('settings.runtimeDb')}</dt>
    <dd class="min-w-0">
      <div class="flex flex-wrap items-center gap-1.5">
        <span class="break-all font-mono text-text-primary">{runtime.dbPath || t('settings.none')}</span>
        {#if runtime.dbPath}
          <button type="button" class={COPY_BTN} onclick={() => copyValue('db', runtime.dbPath)}>
            {copiedKey === 'db' ? t('settings.copied') : t('settings.copy')}
          </button>
          <button
            type="button"
            class={COPY_BTN}
            title={t(onDesktop ? 'settings.copySqliteDesktop' : 'settings.copySqlite')}
            onclick={() => copyValue('sqlite', `sqlite3 ${runtime.dbPath}`)}
          >
            {copiedKey === 'sqlite'
              ? t('settings.copied')
              : onDesktop
                ? t('settings.copySqliteLabelDesktop')
                : 'sqlite3'}
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
            onclick={() => copyValue('cfg', runtime.configPath)}
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
    <dd class="font-mono text-text-primary">{runtime.gadakVersion}</dd>

    <dt class="text-text-muted">{t('settings.localOriginHow')}</dt>
    <dd class="min-w-0">
      <div class="flex flex-wrap items-center gap-1.5">
        <code
          class="break-all font-mono text-text-primary"
          data-testid="local-origin-init-command"
        >{STANDALONE_INIT_COMMAND}</code>
        <button
          type="button"
          class={COPY_BTN}
          data-testid="local-origin-init-copy"
          onclick={() => copyValue('local-origin-init', STANDALONE_INIT_COMMAND)}
        >
          {copiedKey === 'local-origin-init' ? t('settings.copied') : t('settings.copy')}
        </button>
      </div>
      <div class="mt-0.5 text-text-muted">{t('settings.workspaceLocalOriginHint')}</div>
      <div class="mt-0.5 text-text-muted">{t('settings.localOriginCommandHint')}</div>
    </dd>
  </dl>
</section>
