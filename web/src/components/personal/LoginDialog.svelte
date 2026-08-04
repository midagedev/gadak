<script lang="ts">
  /*
   * 로그인 미니 다이얼로그 ([personal]). 이메일/비밀번호 → me.login().
   *  모달 오버레이. Esc/배경 클릭으로 닫기. 성공 시 onClose.
   */
  import { t } from '../../lib/i18n'
  import { me } from '../../stores/me.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let email = $state('')
  let password = $state('')
  let error = $state<string | null>(null)
  let busy = $state(false)
  let emailEl: HTMLInputElement | null = $state(null)

  $effect(() => {
    // 열릴 때 이메일 입력에 포커스
    emailEl?.focus()
  })

  async function submit(e: Event) {
    e.preventDefault()
    if (busy) return
    error = null
    busy = true
    const res = await me.login(email, password)
    busy = false
    if (res.ok) {
      onClose()
    } else {
      error = res.error ?? t('login.failed')
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- 배경 -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onClose()
  }}
>
  <!-- 다이얼로그 -->
  <div
    class="anim-enter w-full max-w-xs rounded-lg border border-border-strong bg-bg-panel p-5 shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label={t('login.title')}
  >
    <h2 class="mb-1 text-[14px] font-semibold text-text-primary">{t('login.heading')}</h2>
    <p class="mb-4 text-[12px] text-text-muted">
      {t('login.hint')}
    </p>

    <form onsubmit={submit} class="flex flex-col gap-3">
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">{t('common.email')}</span>
        <input
          bind:this={emailEl}
          bind:value={email}
          type="email"
          autocomplete="username"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
          placeholder="you@example.com"
        />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">{t('common.password')}</span>
        <input
          bind:value={password}
          type="password"
          autocomplete="current-password"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
        />
      </label>

      {#if error}
        <p class="text-[12px] text-status-reopen">{error}</p>
      {/if}

      <div class="mt-1 flex items-center justify-end gap-2">
        <button
          type="button"
          onclick={onClose}
          class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
        >
          {t('common.cancel')}
        </button>
        <button
          type="submit"
          disabled={busy}
          class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
        >
          {busy ? t('login.submitting') : t('common.login')}
        </button>
      </div>
    </form>
  </div>
</div>
