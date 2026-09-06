<script lang="ts">
  /* How often the mirror refreshes, and how old is "stale". */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import { config, isLocalOriginWorkspace, surface } from '../../lib/config'
  import { copyText } from '../../lib/copy-text'
  import { upgradeCta } from '../../lib/upgrade-cta'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import Icon from '../ui/Icon.svelte'
  import type { SettingsRuntime } from '../../lib/api'
  import { INPUT, INPUT_BARE, SELECT_BARE, SELECT_CHEVRON, ADD_BTN, COPY_BTN } from './controls'
  import { RECONCILE_PRESETS, SYNC_PRESETS, type SettingsDraft } from './draft'
  import RuntimeMirror from './RuntimeMirror.svelte'

  const onDesktop = surface() === 'desktop'

  type UpdateDoc = {
    latest?: string
    release_url?: string
    newer?: boolean
    last_user_check_at?: string
    last_user_status?: string
  }

  const cta = $derived(upgradeCta(config().os))
  let copiedCmd = $state(false)
  let lastUserAt = ''

  async function pullUpdate(): Promise<void> {
    try {
      const res = await fetch(config().apiBase + 'update/', { credentials: 'same-origin' })
      if (!res.ok) return
      const data = (await res.json()) as UpdateDoc
      // Apply only when newer — never clear a delta-injected banner.
      if (data.newer && data.latest) {
        issues.applyUpdateInfo(data.latest, data.release_url ?? '')
      }
      if (!data.last_user_check_at || data.last_user_check_at === lastUserAt) return
      const age = Date.now() - Date.parse(data.last_user_check_at)
      if (!Number.isFinite(age) || age < 0 || age >= 15_000) return
      lastUserAt = data.last_user_check_at
      if (data.last_user_status === 'current') write.toast(t('settings.updateCurrent'), 'success')
      else if (data.last_user_status === 'error') write.toast(t('settings.updateFailed'), 'error')
      else if (data.last_user_status === 'dev') write.toast(t('settings.updateDev'), 'info')
    } catch {
      /* snapshot is advisory */
    }
  }

  onMount(() => {
    void pullUpdate()
    const id = setInterval(() => void pullUpdate(), 2000)
    return () => clearInterval(id)
  })

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
  <!-- GDK-1148: the dialog behind this button edits a SITE credential —
       email + API token. A local-origin workspace has none to edit (it writes
       through its in-process origin), so the entry point advertises a
       concept that does not exist there.

       The predicate is deliberately NOT originWritable: that is true of a
       connected workspace WITH a credential too, and hiding the button
       there would take away the way to rotate a token that does exist. A
       paired workspace is the residual — its credential lives in
       remote-origin.json, so this button is wrong there as well, and the
       client cannot yet tell paired from connected. GDK-1152 is where that
       gap closes; widening this branch by guessing is what put a regression
       here in the first place. -->
  {#if !isLocalOriginWorkspace()}
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

  <section
    class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
    data-testid="settings-update"
  >
    <div class="mb-2 text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.updateTitle')}
    </div>
    {#if issues.latestVersion}
      <p class="text-micro text-text-primary">
        {t('sidebar.updateAvailable', { version: issues.latestVersion })}
      </p>
      {#if issues.releaseUrl}
        <a
          href={issues.releaseUrl}
          target="_blank"
          rel="noreferrer"
          class="mt-1 inline-block text-micro text-accent-text hover:underline"
          data-testid="settings-update-link"
        >
          {t('settings.updateReleaseNotes')}
        </a>
      {/if}
    {/if}
    <!-- Command comes from upgradeCta — the single owner. A new package
         path is a row there, not another os === branch here. -->
    {#if cta.command}
      <div class="mt-2 flex flex-wrap items-center gap-1.5">
        <span class="font-mono text-micro text-text-primary" data-testid="settings-update-brew"
          >{cta.command}</span
        >
        <button type="button" class={COPY_BTN} onclick={() => void copyCmd()}>
          {copiedCmd ? t('settings.copied') : t('settings.copy')}
        </button>
      </div>
    {/if}
  </section>

  <!-- Read-only facts about the mirror these intervals drive: last pull,
       watermark, size, last error. Under the controls, because the controls are
       the subject of the tab and this is the reference for them. (GDK-188) -->
  {#if runtime}
    <RuntimeMirror {runtime} />
  {/if}
</div>
