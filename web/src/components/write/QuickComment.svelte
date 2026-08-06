<script lang="ts">
  /*
   * Quick comment (list triage `c`). Opening the detail panel to leave one line
   * costs the list its place, which is the whole thing the triage flow is trying
   * to keep — so the composer comes to the row instead, in the palette's dialog
   * chrome. The composer itself is unchanged, so the credential gate, drafts,
   * mentions and attachments all behave exactly as they do in the panel.
   */
  import { tick } from 'svelte'
  import { t } from '../../lib/i18n'
  import { trapFocus } from '../../lib/focus-trap'
  import { issues } from '../../stores/issues.svelte'
  import CommentComposer from './CommentComposer.svelte'

  let { issueKey, onclose }: { issueKey: string; onclose: () => void } = $props()

  let rootEl = $state<HTMLDivElement | null>(null)

  const issue = $derived(issues.pool.get(issueKey))

  // Land in the textarea: the key that opened this said "I want to type".
  $effect(() => {
    void issueKey
    void tick().then(() => {
      rootEl?.querySelector<HTMLTextAreaElement>('[data-testid="comment-composer"]')?.focus()
    })
  })

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      onclose()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-start justify-center bg-black/60 p-4 pt-[14vh] backdrop-blur-sm"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    bind:this={rootEl}
    use:trapFocus
    class="anim-pop flex w-full max-w-lg flex-col overflow-hidden rounded-lg border border-border-strong bg-bg-panel shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('triage.commentOn', { key: issueKey })}
    data-testid="quick-comment"
  >
    <div class="flex flex-none items-center gap-2 border-b border-border-subtle px-4 py-2.5">
      <span class="flex-none font-mono text-[12px] text-accent-text">{issueKey}</span>
      <span class="min-w-0 flex-1 truncate text-[13px] text-text-primary" title={issue?.summary}>
        {issue?.summary ?? ''}
      </span>
      <button
        type="button"
        class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-[13px] text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={onclose}
        aria-label={t('common.closeEsc')}
        title={t('common.closeEsc')}
      >
        ✕
      </button>
    </div>

    <div class="px-4 pb-3 pt-1">
      <CommentComposer {issueKey} onsubmitted={onclose} />
    </div>
  </div>
</div>
