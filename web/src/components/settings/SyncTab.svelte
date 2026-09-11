<script lang="ts">
  /* How often the mirror refreshes, and how old is "stale". */
  import { t } from '../../lib/i18n'
  import { config, credentialRequired, surface } from '../../lib/config'
  import { copyText } from '../../lib/copy-text'
  import { upgradeCta } from '../../lib/upgrade-cta'
  import Icon from '../ui/Icon.svelte'
  import type { SettingsRuntime } from '../../lib/api'
  import { INPUT, INPUT_BARE, SELECT_BARE, SELECT_CHEVRON, ADD_BTN, COPY_BTN } from './controls'
  import { RECONCILE_PRESETS, SYNC_PRESETS, type SettingsDraft } from './draft'
  import RuntimeMirror from './RuntimeMirror.svelte'

  const onDesktop = surface() === 'desktop'

  const cta = $derived(upgradeCta(config().os))
  let copiedCmd = $state(false)

  async function copyCmd(): Promise<void> {
    if (!cta.command) return
    if (await copyText(cta.command)) {
      copiedCmd = true
      setTimeout(() => {
        copiedCmd = false
      }, 1500)
    }
  }

  // `runtime` is null until the settings load lands (and on an older server
  // that sends no runtime block at all) — the mirror simply has nothing to
  // mirror then, which is not an error state to report.
  let {
    draft = $bindable(),
    defaultSyncSec,
    defaultReconcileSec,
    runtime = null,
    onOpenJiraKey,
  }: {
    draft: SettingsDraft
    defaultSyncSec: number
    defaultReconcileSec: number
    runtime?: SettingsRuntime | null
    onOpenJiraKey: () => void
  } = $props()
</script>

<div class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <span class="text-micro text-text-secondary">{t('settings.syncInterval')}</span>
    <div class="flex flex-wrap items-center gap-2">
      <!-- selected on the option, not value on the select: a plain
           value attribute applies before the #each options mount and
           never re-syncs, leaving the control visibly empty. -->
      <span class="relative flex">
        <select
          class="{SELECT_BARE} w-auto max-w-[12rem]"
          onchange={(e) => {
            draft.syncPreset = Number(e.currentTarget.value)
            if (draft.syncPreset !== -1) draft.syncCustomText = ''
          }}
        >
          {#each SYNC_PRESETS as p (p.value)}
            <option value={p.value} selected={p.value === draft.syncPreset}>
              {t(p.labelKey)}{p.value === 0 ? ` (${t('settings.intervalDefaultSeconds', { n: String(defaultSyncSec) })})` : ''}
            </option>
          {/each}
        </select>
        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
      </span>
      {#if draft.syncPreset === -1}
        <input
          class="{INPUT_BARE} w-28"
          type="text" inputmode="numeric"
          min="15"
          step="1"
          bind:value={draft.syncCustomText}
          placeholder={String(defaultSyncSec)}
          aria-label={t('settings.syncInterval')}
        />
        <span class="text-micro text-text-muted">{t('settings.intervalSeconds')}</span>
      {/if}
    </div>
    <span class="text-micro text-text-muted"
      >{onDesktop ? t('settings.syncIntervalHintDesktop') : t('settings.syncIntervalHint')}</span
    >
  </div>

  <div class="flex flex-col gap-1">
    <span class="text-micro text-text-secondary">{t('settings.reconcileInterval')}</span>
    <div class="flex flex-wrap items-center gap-2">
      <span class="relative flex">
        <select
          class="{SELECT_BARE} w-auto max-w-[12rem]"
          onchange={(e) => {
            draft.reconcilePreset = Number(e.currentTarget.value)
            if (draft.reconcilePreset !== -1) draft.reconcileCustomText = ''
          }}
        >
          {#each RECONCILE_PRESETS as p (p.value)}
            <option value={p.value} selected={p.value === draft.reconcilePreset}>
              {t(p.labelKey)}{p.value === 0 ? ` (${t('settings.intervalDefaultSeconds', { n: String(defaultReconcileSec) })})` : ''}
            </option>
          {/each}
        </select>
        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
      </span>
      {#if draft.reconcilePreset === -1}
        <input
          class="{INPUT_BARE} w-28"
          type="text" inputmode="numeric"
          min="300"
          step="1"
          bind:value={draft.reconcileCustomText}
          placeholder={String(defaultReconcileSec)}
          aria-label={t('settings.reconcileInterval')}
        />
        <span class="text-micro text-text-muted">{t('settings.intervalSeconds')}</span>
      {/if}
    </div>
    <span class="text-micro text-text-muted"
      >{onDesktop
        ? t('settings.reconcileIntervalHintDesktop')
        : t('settings.reconcileIntervalHint')}</span
    >
  </div>
  <p class="text-micro leading-relaxed text-text-muted">{t('settings.intervalApplies')}</p>

  <label class="flex max-w-[200px] flex-col gap-1">
    <span class="text-micro text-text-secondary">{t('settings.staleHours')}</span>
    <input
      class={INPUT}
      type="text"
      inputmode="numeric"
      min="1"
      placeholder={t('settings.staleLearnedPlaceholder')}
      bind:value={draft.staleText}
    />
    <span class="text-micro text-text-muted">
      {t('settings.staleHint')}
    </span>
  </label>
  <!-- GDK-1148/GDK-1152: the dialog behind this button edits a SITE
       credential — email + API token. The origin states whether this
       workspace has one (capabilities.credentialRequired): true on the
       Jira family reached as a site, false on built-in (writes through its
       in-process origin), paired (its credential lives in
       remote-origin.json on the home machine), and Linear (key is config,
       not a site token). The old workspace-kind branch showed the button to
       paired workspaces too — selling a token errand to a workspace that
       has no token to set here. -->
  {#if credentialRequired()}
  <div class="border-t border-border-subtle pt-3">
    <button
      type="button"
      class={ADD_BTN}
      onclick={onOpenJiraKey}
    >
      {t('settings.personalToken')}
    </button>
    <p class="mt-1 text-micro text-text-muted">
      {t('settings.credsElsewhere')}
    </p>
  </div>
  {/if}

  {#if cta.command}
    <section
      class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
      data-testid="settings-upgrade"
    >
      <div class="mb-2 section-label">
        {t('settings.upgradeTitle')}
      </div>
      <!-- Command comes from upgradeCta — the single owner. A new package
           path is a row there, not another os === branch here. -->
      <div class="flex flex-wrap items-center gap-1.5">
        <span class="font-mono text-micro text-text-primary" data-testid="settings-upgrade-cmd"
          >{cta.command}</span
        >
        <button type="button" class={COPY_BTN} onclick={() => void copyCmd()}>
          {copiedCmd ? t('detail.linkCopied') : t('settings.copy')}
        </button>
      </div>
    </section>
  {/if}

  <!-- Read-only facts about the mirror these intervals drive: last pull,
       watermark, size, last error. Under the controls, because the controls are
       the subject of the tab and this is the reference for them. (GDK-188) -->
  {#if runtime}
    <RuntimeMirror {runtime} />
  {/if}
</div>
