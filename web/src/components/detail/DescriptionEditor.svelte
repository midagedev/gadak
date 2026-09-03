<script lang="ts">
  /*
   * Detail description: rendered from the ADF the server presents, edited as
   * markdown (docs/decisions/0012, GDK-1385). The server owns both halves:
   * `md` is the markdown the editor opens with — the text as typed for a
   * plain body, an escaped serialization for a rich one — and `loss` names
   * what a markdown save would drop (panel, media, mention, …). When loss
   * is non-empty the editor still opens, but a banner names it and the save
   * button says so — silent destruction is the thing this file exists to
   * prevent. Preview renders the draft through POST preview/ so the web
   * keeps no markdown parser of its own.
   */
  import { t } from '../../lib/i18n'
  import type { AdfNode, DetailAttachment } from '../../lib/types'
  import { previewMarkdown } from '../../lib/api'
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
    md = null,
    loss = [],
  }: {
    issueKey: string
    node?: AdfNode | null
    attachments?: DetailAttachment[]
    fallback?: string | null
    /** Markdown source from the server (description_md). Older servers omit → fallback text. */
    md?: string | null
    /** Node/mark kinds a markdown save would drop (format_loss). */
    loss?: string[]
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let busy = $state(false)
  let previewing = $state(false)
  let previewNode: AdfNode | null = $state(null)
  let ta: HTMLTextAreaElement | null = $state(null)

  const lossFree = $derived(loss.length === 0)
  const canEdit = $derived(me.identified || isHostedDemo())
  // The hosted demo has no server to render a preview.
  const canPreview = $derived(!isHostedDemo())

  // No reset effect on issueKey (GDK-692): DetailPanel keys this component on
  // the issue, so a switch remounts it and every local above returns to its
  // initial value — and an in-flight commit's continuation writes to the old
  // instance instead of clobbering the new issue's draft.

  function source(): string {
    if (md != null && md !== '') return md
    return fallback ?? ''
  }

  async function start() {
    if (!(await write.ensureWritableFor(issueKey))) return
    draft = source()
    previewing = false
    previewNode = null
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
    previewing = false
    previewNode = null
    draft = ''
  }

  async function showPreview() {
    if (!canPreview) return
    previewing = true
    try {
      previewNode = (await previewMarkdown(draft)).adf
    } catch {
      previewNode = null
    }
  }

  function showWrite() {
    previewing = false
    queueMicrotask(() => ta?.focus())
  }

  async function commit() {
    const next = draft
    // Unchanged and nothing to lose: nothing to send. With loss, an unchanged
    // save still PUTs — the explicit label is the confirmation to drop it.
    if (lossFree && next.trim() === source().trim()) {
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
  data-description-loss={lossFree ? 'none' : loss.join(',')}
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
      {#if !lossFree}
        <p
          class="flex items-start gap-2 rounded-md border border-status-stale/40 bg-status-stale/10 px-3 py-1.5 text-body text-status-stale"
          role="alert"
          data-testid="description-format-warn"
        >
          <Icon name="warning" size={14} class="mt-0.5 flex-none" />
          <span>{t('write.descriptionFormatWarn', { loss: loss.join(', ') })}</span>
        </p>
      {/if}
      {#if canPreview}
        <div class="flex items-center gap-1" role="tablist" aria-label={t('write.editDescription')}>
          <button
            type="button"
            role="tab"
            aria-selected={!previewing}
            data-testid="description-tab-write"
            onclick={showWrite}
            class="inline-flex h-control-sm items-center rounded-md px-2 text-body transition-colors {previewing
              ? 'text-text-muted hover:bg-bg-hover hover:text-text-primary'
              : 'bg-bg-hover text-text-primary'}"
          >
            {t('write.tabWrite')}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={previewing}
            data-testid="description-tab-preview"
            onclick={() => void showPreview()}
            class="inline-flex h-control-sm items-center rounded-md px-2 text-body transition-colors {previewing
              ? 'bg-bg-hover text-text-primary'
              : 'text-text-muted hover:bg-bg-hover hover:text-text-primary'}"
          >
            {t('write.tabPreview')}
          </button>
          <span class="ml-auto truncate text-micro text-text-muted">{t('write.markdownHint')}</span>
        </div>
      {/if}
      {#if previewing}
        <div
          class="min-h-[8rem] rounded-md border border-border-subtle bg-bg-base px-2.5 py-1.5"
          data-testid="description-preview"
        >
          <AdfContent node={previewNode} {issueKey} {attachments} emptyLabel={t('detail.noDescription')} />
        </div>
      {:else}
        <textarea
          bind:this={ta}
          bind:value={draft}
          rows="8"
          disabled={busy}
          data-testid="description-editor-input"
          aria-label={t('write.editDescription')}
          class="w-full resize-y rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 font-mono text-body text-text-primary outline-none focus:border-accent disabled:opacity-50"
        ></textarea>
      {/if}
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
          {busy ? t('common.saving') : lossFree ? t('common.save') : t('write.saveAsPlain')}
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
