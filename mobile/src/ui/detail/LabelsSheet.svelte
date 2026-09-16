<script lang="ts">
  import Sheet from '../Sheet.svelte'
  import CurrentTick from './CurrentTick.svelte'
  import { t, fieldLabel } from '../../lib/i18n'
  import { errorMessage } from '../../lib/api'
  import { setLabels } from '../../lib/writes'
  import { knownLabels, sameLabels, splitLabelInput } from '../../lib/labels'
  import { app } from '../../lib/store.svelte'
  import type { IssueLite, WriteField } from '../../lib/types'

  /*
   * The labels picker (GDK-1925), extracted from Detail.svelte. The set
   * being edited is this sheet's own: construction (the screen mounts it
   * inside its `{#if}`) is the reset point, so opening the sheet starts
   * from what the issue carries — the initializer, not an open handler.
   *
   * The one departure from the pick dialect is the refusal. The screen's
   * refuseWrite drops an open sheet so its sentence lands on the status row
   * underneath, which is right for a picker whose rows are the server's —
   * reopening asks the server again. This sheet holds something the person
   * composed (a set they toggled), and eating that on a refusal is the
   * defect GDK-1863 closed for words. So the latch is taken (writes are
   * off from here on, every control recedes) and the sheet stays standing
   * with its set and the same sentence in its own error line.
   */
  let {
    issueKey,
    current,
    writesOff,
    onwritten,
    onrefused,
    onclose,
  }: {
    issueKey: string
    /** The labels the issue carries — the draft's starting point. */
    current: string[]
    writesOff: boolean
    /** The wrapper's answer — the screen latches `written`, syncs, and
     *  since GDK-1964 names the field so the save can announce itself. */
    onwritten: (next: IssueLite, field: WriteField) => void
    onrefused: (err: unknown, keepSheetOpen?: boolean) => boolean
    onclose: () => void
  } = $props()

  /** The set being edited; the row's own set until a toggle moves it. */
  // svelte-ignore state_referenced_locally
  // Capturing the initial value IS the contract: the screen mounts this
  // sheet inside its `{#if}`, so a fresh `current` arrives as a fresh
  // sheet, not as an edit under the person's thumb.
  let labelDraft = $state([...current])
  let labelInput = $state('')
  /** Labels typed in this sitting. Kept apart from the draft so one that is
   *  added and then turned back off stays on screen as an unchecked row
   *  rather than vanishing from a list it was never in. */
  let labelAdded = $state<string[]>([])
  let labelsSaving = $state(false)
  let labelsError = $state<string | null>(null)
  /** Every label this workspace uses, plus any this row carries that the
   *  snapshot has not caught up with, plus whatever was just typed in. The
   *  rows are read off the snapshot the phone is holding (lib/labels.ts) —
   *  the serve has no labels catalog to ask for, and the desk's own dialog
   *  reads the same source. */
  const labelChoices = $derived(
    [
      ...new Set([...knownLabels(app.issues), ...current, ...labelDraft, ...labelAdded]),
    ].sort((a, b) => a.localeCompare(b)),
  )
  const labelsArmed = $derived(!sameLabels(labelDraft, current))

  function toggleLabel(label: string) {
    labelDraft = labelDraft.includes(label)
      ? labelDraft.filter((l) => l !== label)
      : [...labelDraft, label]
  }

  /** Whitespace separates labels rather than sitting inside one: Jira
   *  refuses a label with a space and the server only trims (lib/labels.ts). */
  function addTypedLabel() {
    const typed = splitLabelInput(labelInput)
    if (typed.length === 0) return
    labelDraft = [...new Set([...labelDraft, ...typed])]
    labelAdded = [...new Set([...labelAdded, ...typed])]
    labelInput = ''
  }

  async function saveLabels() {
    if (writesOff || labelsSaving || !labelsArmed) return
    labelsSaving = true
    labelsError = null
    try {
      const res = await setLabels(issueKey, labelDraft)
      onwritten(res.issue, 'labels')
    } catch (err) {
      labelsError = errorMessage(err)
      onrefused(err, true)
    } finally {
      labelsSaving = false
    }
  }
</script>

<Sheet title={fieldLabel('labels')} {onclose}>
  <div class="pick-list">
    <!-- The free line below adds a label that is not there yet; it sits
         under the rows because it is the exception, not the way in. -->
    {#each labelChoices as l (l)}
      {@const on = labelDraft.includes(l)}
      <button
        class="t-row"
        class:current={on}
        aria-pressed={on}
        disabled={labelsSaving}
        onclick={() => toggleLabel(l)}
      >
        <span class="t-text"><span class="t-name">{l}</span></span>
        {#if on}<CurrentTick />{/if}
      </button>
    {/each}
    {#if labelChoices.length === 0}
      <p class="none">{t('common.none')}</p>
    {/if}
    <div class="sheet-foot">
    <div class="label-add">
      <input
        bind:value={labelInput}
        placeholder={t('write.addLabelOptional')}
        aria-label={t('write.addLabelOptional')}
        enterkeyhint="done"
        autocapitalize="none"
        autocorrect="off"
        spellcheck="false"
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            addTypedLabel()
          }
        }}
      />
      <button
        class="ghost"
        aria-label={t('write.addLabelOptional')}
        disabled={splitLabelInput(labelInput).length === 0}
        onclick={addTypedLabel}>+</button
      >
    </div>
    <div class="sheet-actions">
      <button
        class="save"
        class:armed={labelsArmed && !writesOff}
        disabled={writesOff || labelsSaving || !labelsArmed}
        onclick={() => void saveLabels()}
      >
        {t('common.save')}
      </button>
    </div>
    {#if labelsError}
      <p class="field-err">{labelsError}</p>
    {/if}
    </div>
  </div>
</Sheet>

<style>
  /* also in Detail.svelte — same tokens, GDK-1925 */
  .none {
    margin: 4px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  /* also in Detail.svelte — same tokens, GDK-1925 */
  .save {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 6px;
    font-weight: 600;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
  }
  /* also in Detail.svelte — same tokens, GDK-1925 */
  .ghost {
    flex: none;
    min-height: var(--spacing-control);
    padding: 0 12px;
    border-radius: 6px;
    color: var(--color-accent-text);
  }
  /* also in Detail.svelte — same tokens, GDK-1925 */
  .field-err {
    flex: 1 0 100%;
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .pick-list {
    overflow-y: auto;
    padding: 4px 8px 8px;
    display: flex;
    flex-direction: column;
  }
  /* also in the other pick sheets — same tokens, GDK-1925 */
  .t-row {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 10px;
    min-height: var(--spacing-control);
    padding: 6px 8px;
    border-radius: 6px;
    text-align: left;
  }
  .t-row:active:not(:disabled) {
    background: var(--color-bg-hover);
  }
  .t-row:disabled {
    opacity: 0.5;
  }
  .t-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .t-name {
    color: var(--color-text-primary);
    font-weight: 600;
  }
  /* The free line and Save stay in frame while the rows scroll above them:
     the 2026-09-14 vision pass opened the sheet on a workspace with twelve
     labels and found the way to add one, and the Save, below the fold.
     Sticky inside .pick-list's own scroll, on the sheet's panel colour. */
  .sheet-foot {
    position: sticky;
    bottom: 0;
    background: var(--color-bg-panel);
    padding-bottom: 8px;
    border-top: 1px solid var(--color-border-subtle);
  }
  /* In a sheet the actions sit on the 16px gutter the inputs above them
     use (.due-edit / .label-add pad 8 inside .pick-list's 8) — the same
     vision pass measured the Due sheet's Save box starting 8px left of the
     date input's edge. */
  .sheet-actions {
    display: flex;
    gap: 8px;
    padding: 8px 8px 0;
  }
  /* The free label line under the picker's rows. Same input dialect as the
     assignee sheet's search field, with the add beside it. */
  .label-add {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 8px 0;
  }
  .label-add input {
    flex: 1 1 auto;
    min-width: 0;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: var(--text-body);
  }
</style>
