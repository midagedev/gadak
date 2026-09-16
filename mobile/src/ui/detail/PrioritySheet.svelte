<script lang="ts">
  import Sheet from '../Sheet.svelte'
  import CurrentTick from './CurrentTick.svelte'
  import { t } from '../../lib/i18n'
  import { errorMessage } from '../../lib/api'
  import { setPriority } from '../../lib/writes'
  import type { IssueLite, PriorityDoc, WriteField } from '../../lib/types'

  /*
   * The priority picker (GDK-1925), extracted from Detail.svelte. The
   * catalog is NOT this component's: the screen asks for it once and keeps
   * it for the screen's life, so a second open renders from what is already
   * in hand and no request goes out (the request-invariance contract in
   * ui/detail/detail-sheets.test.ts). The pick's per-row flight state is
   * the sheet's own — applyingId/rowError/failedRow used to be one trio
   * shared with the assignee sheet, but the two cannot be open at once, so
   * each sheet carrying its own loses nothing and keeps the coupling from
   * coming back.
   */
  let {
    issueKey,
    lite,
    writesOff,
    priorities,
    loading,
    error,
    onwritten,
    onrefused,
    onclose,
  }: {
    issueKey: string
    lite: IssueLite | undefined
    writesOff: boolean
    /** The screen's cached catalog — null until its one ask lands. */
    priorities: PriorityDoc[] | null
    loading: boolean
    error: string | null
    /** The wrapper's answer — the screen latches `written`, syncs, and
     *  since GDK-1964 names the field so the save can announce itself. */
    onwritten: (next: IssueLite, field: WriteField) => void
    onrefused: (err: unknown, keepSheetOpen?: boolean) => boolean
    onclose: () => void
  } = $props()

  /** In-flight row id while a pick crosses (null = idle). */
  let applyingId = $state<string | null>(null)
  let rowError = $state<string | null>(null)
  let failedRow = $state<string | null>(null)

  /** The clearing row is "current" only when the issue really carries no
   *  priority — an empty id is the mirror's "unknown", not "none"
   *  (types.ts pins that contract for priority_id). */
  const priorityNone = $derived(Boolean(lite) && !lite?.priority_id && !lite?.priority)

  async function pickPriority(priorityId: string | null) {
    if (writesOff || applyingId) return
    applyingId = priorityId ?? 'none'
    rowError = null
    failedRow = null
    try {
      const res = await setPriority(issueKey, priorityId)
      onwritten(res.issue, 'priority')
    } catch (err) {
      if (onrefused(err)) return
      rowError = errorMessage(err)
      failedRow = applyingId
    } finally {
      applyingId = null
    }
  }
</script>

<Sheet title={t('write.changePriority')} {onclose}>
  <div class="pick-list">
    {#if loading}
      <p class="none">{t('write.askingServer')}</p>
    {:else if error}
      <p class="error">{error}</p>
    {:else if priorities && lite}
      <button
        class="t-row"
        class:current={priorityNone}
        aria-current={priorityNone ? 'true' : undefined}
        disabled={applyingId !== null}
        onclick={() => void pickPriority(null)}
      >
        <span class="t-text">
          <span class="t-name">{t('common.none')}</span>
          {#if failedRow === 'none' && rowError}
            <span class="t-err">{rowError}</span>
          {/if}
        </span>
        {#if priorityNone}<CurrentTick />{/if}
      </button>
      {#each priorities as p (p.id)}
        {@const current = Boolean(lite.priority_id) && lite.priority_id === p.id}
        <button
          class="t-row"
          class:current
          aria-current={current ? 'true' : undefined}
          disabled={applyingId !== null}
          onclick={() => void pickPriority(p.id)}
        >
          <span class="t-text">
            <span class="t-name">{p.name}</span>
            {#if failedRow === p.id && rowError}
              <span class="t-err">{rowError}</span>
            {/if}
          </span>
          {#if current}<CurrentTick />{/if}
        </button>
      {/each}
      {#if priorities.length === 0}
        <p class="none">{t('write.noPriorities')}</p>
      {/if}
    {/if}
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
  .error {
    margin: 8px 0;
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
  .t-err {
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-status-reopen);
  }
</style>
