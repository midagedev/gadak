<script lang="ts">
  /*
   * Read-only instance facts, above the tab content on every tab: which mirror
   * am I editing, where does it live, how much is in it.
   */
  import { t } from '../../lib/i18n'
  import { surface } from '../../lib/config'
  import type { SettingsRuntime } from '../../lib/api'
  import { COPY_BTN } from './controls'

  const onDesktop = surface() === 'desktop'

  let { runtime }: { runtime: SettingsRuntime } = $props()

  // Hidden until there is traffic to report: a row that only ever says zero
  // teaches nothing, and zero here means "nothing flushed yet", not "headroom".
  const apiUsage = $derived(
    runtime.apiUsage && runtime.apiUsage.last_7_days.requests > 0 ? runtime.apiUsage : null,
  )
  let copiedKey = $state<string | null>(null)

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
</script>

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
          <button type="button" class={COPY_BTN} onclick={() => copyText('db', runtime.dbPath)}>
            {copiedKey === 'db' ? t('settings.copied') : t('settings.copy')}
          </button>
          <button
            type="button"
            class={COPY_BTN}
            title={t(onDesktop ? 'settings.copySqliteDesktop' : 'settings.copySqlite')}
            onclick={() => copyText('sqlite', `sqlite3 ${runtime.dbPath}`)}
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
            onclick={() => copyText('cfg', runtime.configPath)}
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
  </dl>
</section>
