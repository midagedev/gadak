<script lang="ts">
  import { t } from '../../lib/i18n'
  /*
   * Auth gate — shown only when there's no cache and we got 401 (render-before-auth).
   * With a cache the app still renders; auth is rechecked quietly in the background.
   */
  let { onRetry }: { onRetry?: () => void } = $props()
</script>

<div class="flex h-screen flex-col items-center justify-center gap-4 bg-bg-base px-6 text-center">
  <div class="flex items-center gap-2">
    <span class="inline-block h-2.5 w-2.5 rounded-[3px] bg-accent"></span>
    <span class="text-title font-semibold text-text-primary">gadak</span>
  </div>
  <p class="max-w-sm text-body text-text-secondary">
    {t('app.authGate')}
    <code class="text-text-primary">gadak serve</code>
    {t('app.authGateHint')}
  </p>
  <div class="flex gap-2">
    {#if onRetry}
      <button
        onclick={onRetry}
        class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('app.authRetry')}
      </button>
    {/if}
  </div>
</div>
