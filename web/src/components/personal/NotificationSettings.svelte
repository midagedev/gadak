<script lang="ts">
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

  // 조용 시간: quiet_start/quiet_end 둘 다 설정돼 있으면 사용 중.
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
    title="알림 설정"
    aria-label="알림 설정"
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
      class="absolute right-0 top-9 z-40 w-[276px] rounded-md border border-border-strong bg-bg-panel p-3 shadow-xl"
    >
      <div class="mb-3 flex items-center gap-2">
        <Bell size={15} strokeWidth={1.8} class="text-text-secondary" />
        <span class="text-[12px] font-semibold text-text-primary">웹 알림</span>
        <span class="flex-1"></span>
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
          onclick={close}
          aria-label="닫기"
          title="닫기"
        >
          <X size={14} strokeWidth={1.8} />
        </button>
      </div>

      {#if me.pushState === 'unsupported'}
        <p class="text-[11px] text-text-muted">이 브라우저는 웹 알림을 지원하지 않습니다.</p>
      {:else if me.pushState === 'unavailable'}
        <p class="text-[11px] text-text-muted">서버 알림 설정이 준비되지 않았습니다.</p>
      {:else if me.pushState === 'denied'}
        <p class="text-[11px] text-status-reopen">브라우저에서 알림이 차단되었습니다.</p>
      {:else}
        <button
          type="button"
          disabled={me.pushState === 'loading'}
          onclick={togglePush}
          class="mb-3 flex h-8 w-full items-center justify-center gap-1.5 rounded-md border text-[12px] font-medium transition-colors disabled:opacity-50 {me.pushState ===
          'subscribed'
            ? 'border-status-done/40 bg-status-done/10 text-status-done hover:bg-status-done/15'
            : 'border-border-strong text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
        >
          {#if me.pushState === 'subscribed'}
            <Check size={14} strokeWidth={2} />
            이 브라우저에서 켜짐
          {:else}
            <Bell size={14} strokeWidth={1.8} />
            이 브라우저에서 켜기
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
              멘션
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
              새 담당 이슈
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
              워치 이슈 변경
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
              잠금 화면에 내용 표시
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
              조용 시간 (이 시간대엔 알림 보류)
            </label>
            {#if quietEnabled}
              <div class="flex items-center gap-2 pl-6 text-[11px] text-text-muted">
                <input
                  type="time"
                  value={me.notificationConfig.preferences.quiet_start ?? ''}
                  onchange={(event) => setQuiet('quiet_start', event.currentTarget.value)}
                  class="rounded border border-border-strong bg-bg-elevated px-1.5 py-1 text-[11px] text-text-primary"
                  aria-label="조용 시간 시작"
                />
                <span>~</span>
                <input
                  type="time"
                  value={me.notificationConfig.preferences.quiet_end ?? ''}
                  onchange={(event) => setQuiet('quiet_end', event.currentTarget.value)}
                  class="rounded border border-border-strong bg-bg-elevated px-1.5 py-1 text-[11px] text-text-primary"
                  aria-label="조용 시간 종료"
                />
              </div>
              <p class="pl-6 text-[10px] text-text-muted">KST 기준 · 자정 걸침 가능</p>
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
