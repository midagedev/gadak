<script lang="ts">
  import Sheet from '../Sheet.svelte'
  import { t, fieldLabel } from '../../lib/i18n'
  import { errorMessage } from '../../lib/api'
  import { setDuedate } from '../../lib/writes'
  import type { IssueLite, WriteField } from '../../lib/types'

  /*
   * The due-date picker (GDK-1925), extracted from Detail.svelte. The same
   * picker dialect as labels: construction is the reset point (the screen
   * mounts the sheet inside its `{#if}`), the draft starts from what the
   * issue carries, and a refusal keeps the sheet standing — the date was
   * composed, not picked off the server's rows (GDK-1863's bargain).
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
    /** The issue's duedate as YYYY-MM-DD ('' when it carries none). */
    current: string
    writesOff: boolean
    /** The wrapper's answer — the screen latches `written`, syncs, and
     *  since GDK-1964 names the field so the save can announce itself. */
    onwritten: (next: IssueLite, field: WriteField) => void
    onrefused: (err: unknown, keepSheetOpen?: boolean) => boolean
    onclose: () => void
  } = $props()

  /** `<input type="date">` speaks YYYY-MM-DD and so does the server. */
  // svelte-ignore state_referenced_locally
  // Capturing the initial value IS the contract — the screen mounts this
  // sheet inside its `{#if}`, so a fresh `current` is a fresh sheet.
  let dueDraft = $state(current)
  let dueSaving = $state(false)
  let dueError = $state<string | null>(null)
  const dueArmed = $derived(dueDraft !== '' && dueDraft !== current)

  async function writeDue(value: string | null) {
    if (writesOff || dueSaving) return
    dueSaving = true
    dueError = null
    try {
      const res = await setDuedate(issueKey, value)
      onwritten(res.issue, 'due')
    } catch (err) {
      dueError = errorMessage(err)
      onrefused(err, true)
    } finally {
      dueSaving = false
    }
  }
</script>

<Sheet title={fieldLabel('due')} {onclose}>
  <div class="pick-list">
    <!-- The platform's own date control, not a calendar of our own: it
         speaks YYYY-MM-DD, which is exactly what the server takes, and
         on the phone it is the wheel the person already knows. -->
    <div class="due-edit">
      <input type="date" bind:value={dueDraft} aria-label={fieldLabel('due')} />
    </div>
    <div class="sheet-actions">
      <button
        class="save"
        class:armed={dueArmed && !writesOff}
        disabled={writesOff || dueSaving || !dueArmed}
        onclick={() => void writeDue(dueDraft)}
      >
        {t('common.save')}
      </button>
      {#if current}
        <button class="ghost" disabled={writesOff || dueSaving} onclick={() => void writeDue(null)}>
          {t('common.none')}
        </button>
      {/if}
    </div>
    {#if dueError}
      <p class="field-err">{dueError}</p>
    {/if}
  </div>
</Sheet>

<style>
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
  /* In a sheet the actions sit on the 16px gutter the inputs above them
     use (.due-edit / .label-add pad 8 inside .pick-list's 8) — the
     2026-09-14 vision pass measured this sheet's Save box starting 8px
     left of the date input's edge. */
  .sheet-actions {
    display: flex;
    gap: 8px;
    padding: 8px 8px 0;
  }
  .due-edit {
    padding: 8px 8px 0;
  }
  .due-edit input {
    width: 100%;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: var(--text-body);
    color: var(--color-text-primary);
  }
</style>
