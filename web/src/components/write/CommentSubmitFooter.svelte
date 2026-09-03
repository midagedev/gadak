<script lang="ts">
  /*
   * Shared submit row for issue and page comment composers (GDK-650).
   * Owns the kbd chip (write.commentShortcut + modifierSymbol) and the
   * posting/submit button. The leading snippet is issue-only attach; the
   * page origin has no attach/mention, so DocumentPanel leaves it empty.
   */
  import type { Snippet } from 'svelte'
  import { t } from '../../lib/i18n'
  import { modifierSymbol } from '../../lib/unified-search'
  import Icon from '../ui/Icon.svelte'

  let {
    busy,
    disabled,
    onclick,
    buttonType = 'button',
    submitTestId,
    leading,
    previewing = false,
    onpreview,
  }: {
    busy: boolean
    disabled: boolean
    onclick?: () => void
    buttonType?: 'button' | 'submit'
    submitTestId?: string
    leading?: Snippet
    /** Write/Preview toggle (GDK-1385); absent when the composer cannot render one. */
    previewing?: boolean
    onpreview?: () => void
  } = $props()
</script>

<div class="flex items-center justify-end gap-2">
  {@render leading?.()}
  {#if onpreview}
    <button
      type="button"
      onclick={onpreview}
      aria-pressed={previewing}
      data-testid="comment-preview-toggle"
      class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong px-2 text-body text-text-secondary transition-colors hover:text-text-primary {previewing
        ? 'bg-bg-hover text-text-primary'
        : ''}"
      title={previewing ? t('write.tabWrite') : t('write.tabPreview')}
    >
      <Icon name={previewing ? 'pen' : 'eye'} size={13} />
      {previewing ? t('write.tabWrite') : t('write.tabPreview')}
    </button>
  {/if}
  <kbd
    data-testid="comment-shortcut"
    class="rounded border border-border-subtle px-1 text-micro text-text-muted"
  >{t('write.commentShortcut', { mod: modifierSymbol() })}</kbd>
  <button
    type={buttonType}
    {onclick}
    {disabled}
    data-testid={submitTestId}
    class="inline-flex h-control items-center rounded-md bg-accent px-3 text-body font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-40"
  >
    {busy ? t('write.commentPosting') : t('write.commentButton')}
  </button>
</div>
