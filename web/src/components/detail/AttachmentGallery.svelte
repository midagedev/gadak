<script lang="ts">
  import { t } from '../../lib/i18n'
  import type { DetailAttachment } from '../../lib/types'
  import { mediaViewer } from '../../stores/media-viewer.svelte'

  let { attachments }: { attachments: DetailAttachment[] } = $props()

  function formatBytes(size: number): string {
    if (!size) return ''
    const units = ['B', 'KB', 'MB', 'GB']
    const index = Math.min(Math.floor(Math.log(size) / Math.log(1024)), units.length - 1)
    const value = size / 1024 ** index
    return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
  }
</script>

<div class="grid grid-cols-2 gap-2">
  {#each attachments as attachment (attachment.id)}
    {#if attachment.is_image}
      <button
        type="button"
        onclick={() => mediaViewer.open(attachment)}
        class="group relative aspect-[4/3] min-w-0 overflow-hidden rounded-md border border-border-subtle bg-bg-base text-left hover:border-border-strong"
        aria-label={t('detail.enlarge', { name: attachment.filename })}
      >
        <img
          src={attachment.content_url}
          alt={attachment.filename}
          loading="lazy"
          decoding="async"
          class="h-full w-full object-contain transition-transform duration-150 group-hover:scale-[1.02]"
        />
        <span class="absolute inset-x-0 bottom-0 truncate bg-black/70 px-2 py-1 text-[10px] text-white/90">
          {attachment.filename}
        </span>
      </button>
    {:else if attachment.is_video}
      <button
        type="button"
        onclick={() => mediaViewer.open(attachment)}
        class="group col-span-2 flex aspect-video min-w-0 flex-col items-center justify-center gap-2 rounded-md border border-border-subtle bg-bg-base px-3 text-center hover:border-border-strong hover:bg-bg-hover"
        aria-label={t('detail.play', { name: attachment.filename })}
      >
        <span class="flex h-9 w-9 items-center justify-center rounded-full border border-border-strong bg-bg-elevated text-[16px] text-text-primary group-hover:border-accent">
          ▶
        </span>
        <span class="w-full truncate text-[11px] text-text-secondary">{attachment.filename}</span>
        <span class="text-[10px] text-text-muted">{formatBytes(attachment.size)}</span>
      </button>
    {:else}
      <a
        href={attachment.content_url}
        target="_blank"
        rel="noopener noreferrer"
        class="col-span-2 flex min-w-0 items-center gap-2 rounded-md border border-border-subtle bg-bg-base px-3 py-2 hover:bg-bg-hover"
      >
        <span class="flex-none text-[14px]" aria-hidden="true">▤</span>
        <span class="min-w-0 flex-1 truncate text-[12px] text-text-secondary">{attachment.filename}</span>
        <span class="flex-none text-[10px] text-text-muted">{formatBytes(attachment.size)}</span>
      </a>
    {/if}
  {/each}
</div>
