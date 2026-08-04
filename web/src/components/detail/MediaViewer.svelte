<script lang="ts">
  import { t } from '../../lib/i18n'
  import type { DetailAttachment } from '../../lib/types'

  let {
    attachment,
    onClose,
  }: {
    attachment: DetailAttachment
    onClose: () => void
  } = $props()

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') onClose()
  }

  function onBackdrop(event: MouseEvent) {
    if (event.target === event.currentTarget) onClose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-[70] flex flex-col bg-black/90"
  role="dialog"
  aria-modal="true"
  aria-label={attachment.filename}
  tabindex="-1"
  onclick={onBackdrop}
  onkeydown={onKeydown}
>
  <header class="flex h-12 flex-none items-center justify-between gap-4 border-b border-white/10 px-4">
    <p class="min-w-0 truncate text-[13px] font-medium text-white/90">{attachment.filename}</p>
    <div class="flex flex-none items-center gap-1">
      <a
        href={attachment.content_url}
        target="_blank"
        rel="noopener noreferrer"
        class="flex h-8 w-8 items-center justify-center text-[18px] text-white/60 transition-colors hover:text-white"
        aria-label={t('common.openInNewTab')}
        title={t('common.openInNewTab')}
      >
        ↗
      </a>
      <button
        type="button"
        onclick={onClose}
        class="flex h-8 w-8 items-center justify-center text-[24px] leading-none text-white/60 transition-colors hover:text-white"
        aria-label={t('detail.mediaClose')}
        title={t('common.close')}
      >
        ×
      </button>
    </div>
  </header>
  <div class="flex min-h-0 flex-1 items-center justify-center p-4 sm:p-8">
    {#if attachment.is_image}
      <img
        src={attachment.content_url}
        alt={attachment.filename}
        class="max-h-full max-w-full object-contain"
      />
    {:else if attachment.is_video}
      <video
        src={attachment.content_url}
        controls
        autoplay
        playsinline
        class="max-h-full max-w-full"
      >
        <track kind="captions" />
      </video>
    {/if}
  </div>
</div>
