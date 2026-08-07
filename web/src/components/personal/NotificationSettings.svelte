<script lang="ts">
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { push } from '../../stores/push.svelte'

  let open = $state(false)

  function close() {
    open = false
  }

  async function togglePush() {
    if (push.state === 'subscribed') await push.disable()
    else await push.enable()
  }

  // Quiet hours: enabled when both quiet_start and quiet_end are set.
  const quietEnabled = $derived(
    !!push.notificationConfig?.preferences.quiet_start &&
      !!push.notificationConfig?.preferences.quiet_end,
  )

  function toggleQuiet(on: boolean) {
    void push.updatePreferences(
      on ? { quiet_start: '22:00', quiet_end: '07:00' } : { quiet_start: null, quiet_end: null },
    )
  }

  function setQuiet(field: 'quiet_start' | 'quiet_end', value: string) {
    void push.updatePreferences({ [field]: value || null })
  }
</script>

<svelte:window onclick={close} />

<div
  class="relative"
  role="presentation"
  onclick={(event) => event.stopPropagation()}
  onkeydown={(event) => event.stopPropagation()}
>
  <button
    type="button"
    onclick={() => (open = !open)}
    class="relative flex h-control-sm w-control-sm items-center justify-center rounded-md transition-colors {push.state ===
    'subscribed'
      ? 'text-accent-text hover:bg-bg-hover'
      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
    title={t('notif.title')}
    aria-label={t('notif.title')}
    aria-expanded={open}
  >
    {#if push.state === 'subscribed'}
      <Icon name="bell" size={15} />
      <span class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-status-done"></span>
    {:else}
      <Icon name="bell-off" size={15} />
    {/if}
  </button>

  {#if open}
    <div
      class="absolute right-0 top-9 z-40 w-[276px] rounded-lg border border-border-strong bg-bg-panel p-3 shadow-overlay"
    >
      <div class="mb-3 flex items-center gap-2">
        <Icon name="bell" size={15} class="text-text-secondary" />
        <span class="text-[12px] font-semibold text-text-primary">{t('notif.webPush')}</span>
        <span class="flex-1"></span>
        <button
          type="button"
          class="flex h-control-sm w-control-sm items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
          onclick={close}
          aria-label={t('common.close')}
          title={t('common.close')}
        >
          <Icon name="x" size={14} />
        </button>
      </div>

      {#if push.state === 'unsupported'}
        <p class="text-micro text-text-muted">{t('notif.unsupported')}</p>
      {:else if push.state === 'unavailable'}
        <p class="text-micro text-text-muted">{t('notif.serverNotReady')}</p>
      {:else if push.state === 'denied'}
        <p class="text-micro text-status-reopen">{t('notif.blocked')}</p>
      {:else}
        <button
          type="button"
          disabled={push.state === 'loading'}
          onclick={togglePush}
          class="mb-3 flex h-control w-full items-center justify-center gap-1.5 rounded-md border text-[12px] font-medium transition-colors disabled:opacity-50 {push.state ===
          'subscribed'
            ? 'border-status-done/40 bg-status-done/10 text-status-done hover:bg-status-done/15'
            : 'border-border-strong text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
        >
          {#if push.state === 'subscribed'}
            <Icon name="check" size={14} />
            {t('notif.enabledHere')}
          {:else}
            <Icon name="bell" size={14} />
            {t('notif.enableHere')}
          {/if}
        </button>

        {#if push.notificationConfig}
          <div class="space-y-2 border-t border-border-subtle pt-2.5">
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-micro text-text-secondary">
              <input
                type="checkbox"
                checked={push.notificationConfig.preferences.notify_mentions}
                onchange={(event) =>
                  push.updatePreferences({
                    notify_mentions: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.mention')}
            </label>
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-micro text-text-secondary">
              <input
                type="checkbox"
                checked={push.notificationConfig.preferences.notify_assigned}
                onchange={(event) =>
                  push.updatePreferences({
                    notify_assigned: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.newAssignee')}
            </label>
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-micro text-text-secondary">
              <input
                type="checkbox"
                checked={push.notificationConfig.preferences.notify_watched}
                onchange={(event) =>
                  push.updatePreferences({
                    notify_watched: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.watchChange')}
            </label>
            <label
              class="flex min-h-6 cursor-pointer items-center gap-2 border-t border-border-subtle pt-2 text-micro text-text-secondary"
            >
              <input
                type="checkbox"
                checked={push.notificationConfig.preferences.show_preview}
                onchange={(event) =>
                  push.updatePreferences({ show_preview: event.currentTarget.checked })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.lockScreen')}
            </label>
          </div>

          <div class="mt-2 space-y-2 border-t border-border-subtle pt-2.5">
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-micro text-text-secondary">
              <input
                type="checkbox"
                checked={quietEnabled}
                onchange={(event) => toggleQuiet(event.currentTarget.checked)}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.quietHours')}
            </label>
            {#if quietEnabled}
              <div class="flex items-center gap-2 pl-6 text-micro text-text-muted">
                <input
                  type="time"
                  value={push.notificationConfig.preferences.quiet_start ?? ''}
                  onchange={(event) => setQuiet('quiet_start', event.currentTarget.value)}
                  class="h-control-sm rounded border border-border-strong bg-bg-elevated px-1.5 text-micro text-text-primary"
                  aria-label={t('notif.quietStart')}
                />
                <span>~</span>
                <input
                  type="time"
                  value={push.notificationConfig.preferences.quiet_end ?? ''}
                  onchange={(event) => setQuiet('quiet_end', event.currentTarget.value)}
                  class="h-control-sm rounded border border-border-strong bg-bg-elevated px-1.5 text-micro text-text-primary"
                  aria-label={t('notif.quietEnd')}
                />
              </div>
              <p class="pl-6 text-micro text-text-muted">{t('notif.quietHint')}</p>
            {/if}
          </div>
        {/if}
      {/if}

      {#if push.error}
        <p class="mt-2 text-micro text-status-reopen">{push.error}</p>
      {/if}
    </div>
  {/if}
</div>
