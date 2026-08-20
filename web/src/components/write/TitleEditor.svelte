<script lang="ts">
  /*
   * Detail-header title. Idle is the same subject line as before; click turns
   * it into a field that looks like the heading. Enter saves, Esc restores.
   * Jira's summary is a single line (255).
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { onEscape } from '../../lib/dom-actions'

  let { issue }: { issue: IssueLite } = $props()

  let editing = $state(false)
  let draft = $state('')
  let busy = $state(false)
  let ignoreBlur = $state(false)
  let inputEl: HTMLInputElement | null = $state(null)

  const canEdit = $derived(me.identified)

  async function start() {
    if (!(await write.ensureWritableFor(issue.issue_key))) return
    ignoreBlur = false
    draft = issue.summary
    editing = true
    queueMicrotask(() => {
      inputEl?.focus()
      inputEl?.select()
    })
  }

  function cancel() {
    ignoreBlur = true
    editing = false
    draft = ''
  }

  async function commit() {
    const next = draft.trim()
    if (!next) {
      write.toast(t('write.titleRequired'), 'info')
      return
    }
    if (next === issue.summary) {
      cancel()
      return
    }
    busy = true
    const ok = await write.setSummary(issue.issue_key, next)
    busy = false
    if (ok) cancel()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      void commit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancel()
    }
  }
</script>

<h2 class="type-subject mb-3 text-heading text-text-primary">
  {#if editing}
    <input
      bind:this={inputEl}
      bind:value={draft}
      use:onEscape={(e) => {
        e.preventDefault()
        cancel()
      }}
      onkeydown={onKeydown}
      onblur={() => {
        if (!busy && !ignoreBlur) void commit()
      }}
      type="text"
      maxlength="255"
      disabled={busy}
      data-testid="title-editor-input"
      aria-label={t('write.editTitle')}
      class="w-full bg-transparent text-heading text-text-primary outline-none ring-1 ring-accent/40 rounded-sm px-0.5 -mx-0.5 disabled:opacity-50"
    />
  {:else}
    <button
      type="button"
      data-testid="title-editor"
      onclick={() => void start()}
      class="group w-full rounded-sm px-0.5 -mx-0.5 text-left transition-colors hover:bg-bg-hover/60"
      title={canEdit ? t('write.editTitle') : issue.summary}
    >
      {issue.summary}
    </button>
  {/if}
</h2>
