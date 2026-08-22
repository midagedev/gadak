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
  import DialogShell from '../ui/DialogShell.svelte'

  let { issueKey, onclose }: { issueKey: string; onclose: () => void } = $props()

  let composer = $state<ReturnType<typeof CommentComposer> | null>(null)

  const issue = $derived(issues.pool.get(issueKey))

  // Land in the textarea: the key that opened this said "I want to type".
  $effect(() => {
    void issueKey
    void tick().then(() => {
      composer?.composerEl()?.focus()
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

<DialogShell
  ariaLabel={t('triage.commentOn', { key: issueKey })}
  data-testid="quick-comment"
  {onclose}
  trap={trapFocus}
  panelClass="anim-pop max-w-lg"
  backdropClass="items-start p-4 pt-[14vh]"
  headerClass="flex flex-none flex-col border-b border-border-subtle px-4 py-2.5"
  titleRowClass="flex items-center gap-2"
>
  {#snippet titleLead()}
    <span class="flex-none font-mono text-micro text-accent-text">{issueKey}</span>
    <span class="min-w-0 flex-1 truncate text-body text-text-primary" title={issue?.summary}>
      {issue?.summary ?? ''}
    </span>
  {/snippet}
  <div class="px-4 pb-3 pt-1">
    <CommentComposer bind:this={composer} {issueKey} onsubmitted={onclose} />
  </div>
</DialogShell>
