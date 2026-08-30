<script lang="ts">
  /*
   * Detail description: read as ADF, edit as the plain-text slice the
   * PUT /description/ endpoint accepts. A pencil (header icon-button grammar)
   * opens a textarea; Esc cancels via onEscape; save calls write.setDescription
   * with no optimistic ADF. Complex docs (not just paragraph/text/hardBreak)
   * still open, but a warning banner and an explicit save label name the
   * format loss — silent destruction is the thing this file exists to prevent.
   */
  import { t } from '../../lib/i18n'
  import type { AdfNode, DetailAttachment } from '../../lib/types'
  import { adfToPlainText, isSimpleAdf } from '../../lib/adf'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { isHostedDemo } from '../../lib/config'
  import { onEscape } from '../../lib/dom-actions'
  import Icon from '../ui/Icon.svelte'
  import AdfContent from './AdfContent.svelte'

  let {
    issueKey,
    node = null,
    attachments = [],
    fallback = null,
  }: {
    issueKey: string
    node?: AdfNode | null
    attachments?: DetailAttachment[]
    fallback?: string | null
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let busy = $state(false)
  let ta: HTMLTextAreaElement | null = $state(null)

  const simple = $derived(isSimpleAdf(node))
  const canEdit = $derived(me.identified || isHostedDemo())

  $effect(() => {
    void issueKey
    editing = false
    draft = ''
    busy = false
  })

  async function start() {
    if (!(await write.ensureWritableFor(issueKey))) return
    draft = adfToPlainText(node)
    if (!draft && fallback) draft = fallback
    editing = true
    queueMicrotask(() => {
      if (!ta) return
      ta.focus()
      ta.setSelectionRange(0, 0)
      ta.scrollTop = 0
    })
  }

  function cancel() {
    editing = false
    draft = ''
  }

  async function commit() {
    const current = adfToPlainText(node)
    const next = draft
    // Unchanged simple doc: nothing to send. Complex + unchanged still PUTs —
    // the explicit "save as plain text" is the confirmation to drop formatting.
    if (simple && next.trim() === current.trim()) {
      cancel()
      return
    }
    busy = true
    const ok = await write.setDescription(issueKey, next)
    busy = false
    if (ok) cancel()
  }

  function onEditorKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      cancel()
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void commit()
    }
  }
</script>

<div
  class="text-body text-text-secondary"
  data-testid="description-editor"
  data-description-editing={editing ? 'true' : 'false'}
  data-description-simple={simple ? 'true' : 'false'}
>
  {#if editing}
    <!-- Esc on the form (textarea or buttons) must preventDefault before
         DetailPanel's window listener closes the panel. -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form
      class="flex flex-col gap-2"
      onsubmit={(e) => {
        e.preventDefault()
        void commit()
      }}
      onkeydown={onEditorKeydown}
      use:onEscape={(e) => {
        e.preventDefault()
        cancel()
      }}
    >
      {#if !simple}
        <p
          class="flex items-start gap-2 rounded-md border border-status-stale/40 bg-status-stale/10 px-3 py-1.5 text-body text-status-stale"
          role="alert"
          data-testid="description-format-warn"
        >
          <Icon name="warning" size={14} class="mt-0.5 flex-none" />
          <span>{t('write.descriptionFormatWarn')}</span>
        </p>
      {/if}
      <textarea
        bind:this={ta}
        bind:value={draft}
        rows="8"
        disabled={busy}
        data-testid="description-editor-input"
        aria-label={t('write.editDescription')}
        class="w-full resize-y rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-body text-text-primary outline-none focus:border-accent disabled:opacity-50"
      ></textarea>
      <div class="flex items-center justify-end gap-2">
        <button
          type="button"
          onclick={cancel}
          disabled={busy}
          data-testid="description-cancel"
          class="inline-flex h-control items-center rounded-md px-3 text-body text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-50"
        >
          {t('common.cancel')}
        </button>
        <button
          type="submit"
          disabled={busy}
          data-testid="description-save"
          class="inline-flex h-control items-center rounded-md bg-accent px-3 text-body font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
        >
          {busy ? t('common.saving') : simple ? t('common.save') : t('write.saveAsPlain')}
        </button>
      </div>
    </form>
  {:else}
    <div class="flex items-start gap-1">
      <div class="min-w-0 flex-1">
        <!-- commands: the body is the one place a ▶ is offered (GDK-1162).
             Comments and body-role custom fields do not get one — a button
             that puts someone else's line at your prompt belongs where the
             issue states its own reproduction, not in a thread. -->
        <AdfContent
          {node}
          issueKey={issueKey}
          {attachments}
          {fallback}
          emptyLabel={t('detail.noDescription')}
          commands
        />
      </div>
      {#if canEdit}
        <button
          type="button"
          onclick={() => void start()}
          data-testid="description-edit"
          class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          aria-label={t('write.editDescription')}
          title={t('write.editDescription')}
        >
          <Icon name="pen" size={14} />
        </button>
      {/if}
    </div>
  {/if}
</div>
