<script lang="ts">
  /* How often the mirror refreshes, and how old is "stale". */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { INPUT, SELECT, SELECT_CHEVRON, ADD_BTN } from './controls'
  import { RECONCILE_PRESETS, SYNC_PRESETS, type SettingsDraft } from './draft'

  let {
    draft = $bindable(),
    defaultSyncSec,
    defaultReconcileSec,
    onOpenJiraKey,
  }: {
    draft: SettingsDraft
    defaultSyncSec: number
    defaultReconcileSec: number
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
          class="{SELECT} w-auto max-w-[12rem]"
          onchange={(e) => {
            draft.syncPreset = Number(e.currentTarget.value)
            if (draft.syncPreset !== -1) draft.syncCustomText = ''
          }}
        >
          {#each SYNC_PRESETS as p (p.value)}
            <option value={p.value} selected={p.value === draft.syncPreset}>
              {t(p.labelKey)}{p.value === 0 ? ` (${defaultSyncSec}s)` : ''}
            </option>
          {/each}
        </select>
        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
      </span>
      {#if draft.syncPreset === -1}
        <input
          class="{INPUT} w-28"
          type="number"
          min="15"
          step="1"
          bind:value={draft.syncCustomText}
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
            draft.reconcilePreset = Number(e.currentTarget.value)
            if (draft.reconcilePreset !== -1) draft.reconcileCustomText = ''
          }}
        >
          {#each RECONCILE_PRESETS as p (p.value)}
            <option value={p.value} selected={p.value === draft.reconcilePreset}>
              {t(p.labelKey)}{p.value === 0 ? ` (${defaultReconcileSec}s)` : ''}
            </option>
          {/each}
        </select>
        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
      </span>
      {#if draft.reconcilePreset === -1}
        <input
          class="{INPUT} w-28"
          type="number"
          min="300"
          step="1"
          bind:value={draft.reconcileCustomText}
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
    <input class={INPUT} type="number" min="1" bind:value={draft.staleText} />
    <span class="text-micro text-text-muted">
      {t('settings.staleHint')}
    </span>
  </label>
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
</div>
