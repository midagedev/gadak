<script lang="ts">
  import { t } from '../../lib/i18n'
  import { Bell, BellOff, Check, X } from '@lucide/svelte'
  import { me } from '../../stores/me.svelte'

  let open = $state(false)

  function close() {
    open = false
  }

  async function togglePush() {
    if (me.pushState === 'subscribed') await me.disablePush()
    else await me.enablePush()
  }

  // Quiet hours: enabled when both quiet_start and quiet_end are set.
  const quietEnabled = $derived(
    !!me.notificationConfig?.preferences.quiet_start &&
      !!me.notificationConfig?.preferences.quiet_end,
  )

  function toggleQuiet(on: boolean) {
    void me.updateNotificationPreferences(
      on ? { quiet_start: '22:00', quiet_end: '07:00' } : { quiet_start: null, quiet_end: null },
    )
  }

  function setQuiet(field: 'quiet_start' | 'quiet_end', value: string) {
    void me.updateNotificationPreferences({ [field]: value || null })
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
    class="relative flex h-7 w-7 items-center justify-center rounded-md transition-colors {me.pushState ===
    'subscribed'
      ? 'text-accent-text hover:bg-bg-hover'
      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
    title={t('notif.title')}
    aria-label={t('notif.title')}
    aria-expanded={open}
  >
    {#if me.pushState === 'subscribed'}
      <Bell size={15} strokeWidth={1.9} />
      <span class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-status-done"></span>
    {:else}
      <BellOff size={15} strokeWidth={1.8} />
    {/if}
  </button>

  {#if open}
    <div
      class="absolute right-0 top-9 z-40 w-[276px] rounded-lg border border-border-strong bg-bg-panel p-3 shadow-overlay"
    >
      <div class="mb-3 flex items-center gap-2">
        <Bell size={15} strokeWidth={1.8} class="text-text-secondary" />
        <span class="text-[12px] font-semibold text-text-primary">{t('notif.webPush')}</span>
        <span class="flex-1"></span>
        <button
          type="button"
          class="flex h-control-sm w-control-sm items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
          onclick={close}
          aria-label={t('common.close')}
          title={t('common.close')}
        >
          <X size={14} strokeWidth={1.8} />
        </button>
      </div>

      {#if me.pushState === 'unsupported'}
        <p class="text-[11px] text-text-muted">{t('notif.unsupported')}</p>
      {:else if me.pushState === 'unavailable'}
        <p class="text-[11px] text-text-muted">{t('notif.serverNotReady')}</p>
      {:else if me.pushState === 'denied'}
        <p class="text-[11px] text-status-reopen">{t('notif.blocked')}</p>
      {:else}
        <button
          type="button"
          disabled={me.pushState === 'loading'}
          onclick={togglePush}
          class="mb-3 flex h-control w-full items-center justify-center gap-1.5 rounded-md border text-[12px] font-medium transition-colors disabled:opacity-50 {me.pushState ===
          'subscribed'
            ? 'border-status-done/40 bg-status-done/10 text-status-done hover:bg-status-done/15'
            : 'border-border-strong text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
        >
          {#if me.pushState === 'subscribed'}
            <Check size={14} strokeWidth={2} />
            {t('notif.enabledHere')}
          {:else}
            <Bell size={14} strokeWidth={1.8} />
            {t('notif.enableHere')}
          {/if}
        </button>

        {#if me.notificationConfig}
          <div class="space-y-2 border-t border-border-subtle pt-2.5">
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[11px] text-text-secondary">
              <input
                type="checkbox"
                checked={me.notificationConfig.preferences.notify_mentions}
                onchange={(event) =>
                  me.updateNotificationPreferences({
                    notify_mentions: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.mention')}
            </label>
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[11px] text-text-secondary">
              <input
                type="checkbox"
                checked={me.notificationConfig.preferences.notify_assigned}
                onchange={(event) =>
                  me.updateNotificationPreferences({
                    notify_assigned: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.newAssignee')}
            </label>
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[11px] text-text-secondary">
              <input
                type="checkbox"
                checked={me.notificationConfig.preferences.notify_watched}
                onchange={(event) =>
                  me.updateNotificationPreferences({
                    notify_watched: event.currentTarget.checked,
                  })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.watchChange')}
            </label>
            <label
              class="flex min-h-6 cursor-pointer items-center gap-2 border-t border-border-subtle pt-2 text-[11px] text-text-secondary"
            >
              <input
                type="checkbox"
                checked={me.notificationConfig.preferences.show_preview}
                onchange={(event) =>
                  me.updateNotificationPreferences({ show_preview: event.currentTarget.checked })}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.lockScreen')}
            </label>
          </div>

          <div class="mt-2 space-y-2 border-t border-border-subtle pt-2.5">
            <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[11px] text-text-secondary">
              <input
                type="checkbox"
                checked={quietEnabled}
                onchange={(event) => toggleQuiet(event.currentTarget.checked)}
                class="h-3.5 w-3.5 accent-accent"
              />
              {t('notif.quietHours')}
            </label>
            {#if quietEnabled}
              <div class="flex items-center gap-2 pl-6 text-[11px] text-text-muted">
                <input
                  type="time"
                  value={me.notificationConfig.preferences.quiet_start ?? ''}
                  onchange={(event) => setQuiet('quiet_start', event.currentTarget.value)}
                  class="h-control-sm rounded border border-border-strong bg-bg-elevated px-1.5 text-[11px] text-text-primary"
                  aria-label={t('notif.quietStart')}
                />
                <span>~</span>
                <input
                  type="time"
                  value={me.notificationConfig.preferences.quiet_end ?? ''}
                  onchange={(event) => setQuiet('quiet_end', event.currentTarget.value)}
                  class="h-control-sm rounded border border-border-strong bg-bg-elevated px-1.5 text-[11px] text-text-primary"
                  aria-label={t('notif.quietEnd')}
                />
              </div>
              <p class="pl-6 text-micro text-text-muted">{t('notif.quietHint')}</p>
            {/if}
          </div>
        {/if}
      {/if}

      {#if me.pushError}
        <p class="mt-2 text-[11px] text-status-reopen">{me.pushError}</p>
      {/if}
    </div>
  {/if}
</div>
