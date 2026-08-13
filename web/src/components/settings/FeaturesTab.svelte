<script lang="ts">
  /* Optional surfaces, plus the two settings that belong to no other tab. */
  import { t } from '../../lib/i18n'
  import type { GadakFeatures } from '../../lib/config'
  import { me } from '../../stores/me.svelte'
  import { INPUT } from './controls'
  import type { SettingsDraft } from './draft'

  let { draft = $bindable() }: { draft: SettingsDraft } = $props()

  const FEATURES: [keyof GadakFeatures, string, string][] = [
    ['feed', t('settings.featureFeed'), t('settings.featureFeedDesc')],
    ['push', t('settings.featurePush'), t('settings.featurePushDesc')],
    ['deploy', t('settings.featureDeploy'), t('settings.featureDeployDesc')],
    ['qa', t('settings.featureQa'), t('settings.featureQaDesc')],
    ['teamGroups', t('settings.featureTeams'), t('settings.featureTeamsDesc')],
  ]
</script>

<div class="flex flex-col gap-2.5">
  {#each FEATURES as [key, label, hint] (key)}
    <label class="flex cursor-pointer items-start gap-2.5">
      <input
        type="checkbox"
        class="mt-0.5 flex-none accent-[var(--color-accent,#3b82f6)]"
        bind:checked={draft.features[key]}
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
    <input class={INPUT} bind:value={draft.qaDashboardUrl} placeholder="https://qa.example.com" />
  </label>
</div>
