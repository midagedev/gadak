<script lang="ts">
  /*
   * Personal Jira API token settings dialog (write).
   *  - Copy + Atlassian API-token issue link.
   *  - jira_email (default me.email) + api_token → server validates via /myself on save.
   *  - When already set: display_name / token_hint / verified_at + delete.
   *  Modal pattern: Esc / backdrop click closes.
   */
  import { t } from '../../lib/i18n'
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { absoluteTime } from '../detail/format'

  const API_TOKEN_URL = 'https://id.atlassian.com/manage-profile/security/api-tokens'

  let jiraEmail = $state(write.jiraEmail || me.email || '')
  let apiToken = $state('')
  let error = $state<string | null>(null)
  let busy = $state(false)
  let emailEl: HTMLInputElement | null = $state(null)

  $effect(() => {
    emailEl?.focus()
  })

  async function submit(e: Event) {
    e.preventDefault()
    if (busy) return
    error = null
    busy = true
    const res = await write.saveCredential(jiraEmail.trim(), apiToken.trim())
    busy = false
    if (res.ok) {
      apiToken = ''
    } else {
      error = res.error ?? t('write.credSaveFailed')
    }
  }

  async function remove() {
    if (busy) return
    busy = true
    await write.deleteCredential()
    busy = false
    apiToken = ''
  }

  function close() {
    write.closeSettings()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close()
  }}
>
  <div
    class="anim-enter w-full max-w-sm rounded-lg border border-border-strong bg-bg-panel p-5 shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('jiraSettings.title')}
  >
    <h2 class="mb-1 text-[14px] font-semibold text-text-primary">{t('jiraSettings.heading')}</h2>
    <p class="mb-4 text-[12px] leading-relaxed text-text-muted">
      <!-- intro3 already ends in "Atlassian"; the line break below is the space
           before the link, so no literal belongs here. -->
      {t('jiraSettings.intro1')} <span class="text-text-secondary">{t('jiraSettings.intro2')}</span>{t('jiraSettings.intro3')}
      <a
        href={API_TOKEN_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="text-accent-text hover:underline">{t('jiraSettings.intro4')}</a
      >{t('jiraSettings.intro5')}
    </p>

    {#if write.configured}
      <!-- Configured summary -->
      <div class="mb-4 rounded-md border border-border-subtle bg-bg-elevated px-3 py-2.5 text-[12px]">
        <div class="flex items-center justify-between gap-2">
          <span class="text-text-secondary">{write.displayName || write.jiraEmail}</span>
          <span class="rounded bg-status-done/15 px-1.5 py-0.5 text-[11px] font-medium text-status-done"
            >{t('jiraSettings.connected')}</span
          >
        </div>
        <div class="mt-1 flex flex-col gap-0.5 text-[11px] text-text-muted">
          <span>{write.jiraEmail}</span>
          {#if write.tokenHint}<span class="font-mono">{t('jiraSettings.tokenDots', { hint: write.tokenHint })}</span>{/if}
          {#if write.verifiedAt}<span>{t('jiraSettings.verified', { when: absoluteTime(write.verifiedAt) })}</span>{/if}
        </div>
      </div>
    {/if}

    <form onsubmit={submit} class="flex flex-col gap-3">
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">{t('jiraSettings.email')}</span>
        <input
          bind:this={emailEl}
          bind:value={jiraEmail}
          type="email"
          autocomplete="username"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
          placeholder="you@example.com"
        />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">
          {t('jiraSettings.intro4')} {#if write.configured}<span class="text-text-muted">{t('jiraSettings.tokenReplace')}</span>{/if}
        </span>
        <input
          bind:value={apiToken}
          type="password"
          autocomplete="off"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 font-mono text-[13px] text-text-primary outline-none focus:border-accent"
          placeholder="ATATT3x…"
        />
      </label>

      {#if error}
        <p class="text-[12px] text-status-reopen">{error}</p>
      {/if}

      <div class="mt-1 flex items-center justify-between gap-2">
        {#if write.configured}
          <button
            type="button"
            onclick={remove}
            disabled={busy}
            class="rounded-md px-3 py-1.5 text-[12px] text-status-reopen transition-colors hover:bg-status-reopen/10 disabled:opacity-50"
          >
            {t('common.delete')}
          </button>
        {:else}
          <span></span>
        {/if}
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={close}
            class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
          >
            {t('common.close')}
          </button>
          <button
            type="submit"
            disabled={busy}
            class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {busy ? t('common.verifying') : write.configured ? t('jiraSettings.replaceToken') : t('common.save')}
          </button>
        </div>
      </div>
    </form>
  </div>
</div>
