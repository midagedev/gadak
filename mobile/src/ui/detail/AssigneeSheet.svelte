<script lang="ts">
  import { onDestroy } from 'svelte'
  import Sheet from '../Sheet.svelte'
  import CurrentTick from './CurrentTick.svelte'
  import { t } from '../../lib/i18n'
  import { errorMessage } from '../../lib/api'
  import { searchUsers, setAssignee } from '../../lib/writes'
  import { app } from '../../lib/store.svelte'
  import type { IssueLite, UserDoc, WriteField } from '../../lib/types'

  /*
   * The assignee picker (GDK-1925), extracted from Detail.svelte. The
   * screen keeps the two verdicts that are not the sheet's to give —
   * writability (writesOff) and the write's result (onwritten latches
   * `written` and syncs) — and this component owns everything that only
   * means something while the sheet is standing: the search field, its
   * debounce and its abort. Mounting inside the screen's `{#if}` makes
   * every open start from an empty field, and the teardown below makes
   * closing it cancel whatever was still owed — the screen used to keep a
   * fired-and-forgotten debounce alive after the sheet closed.
   */
  let {
    issueKey,
    lite,
    writesOff,
    onwritten,
    onrefused,
    onclose,
  }: {
    issueKey: string
    lite: IssueLite | undefined
    writesOff: boolean
    /** The wrapper's answer — the screen latches `written`, syncs, and
     *  since GDK-1964 names the field so the save can announce itself. */
    onwritten: (next: IssueLite, field: WriteField) => void
    /** The screen's refuseWrite: true means the refusal was handled. */
    onrefused: (err: unknown, keepSheetOpen?: boolean) => boolean
    onclose: () => void
  } = $props()

  let userQuery = $state('')
  let users = $state<UserDoc[]>([])
  let searching = $state(false)
  /** Search debounce handle + staleness guard: results land only for the
   *  newest keystroke, and the previous request is aborted, not just ignored. */
  let searchTimer: ReturnType<typeof setTimeout> | null = null
  let searchAbort: AbortController | null = null
  let searchSeq = 0
  /** In-flight row id while a pick crosses (null = idle). */
  let applyingId = $state<string | null>(null)
  let rowError = $state<string | null>(null)
  let failedRow = $state<string | null>(null)

  /** The clearing row is "current" only when the issue really carries no
   *  assignee — an empty id is the mirror's "unknown", not "none". */
  const unassignedNow = $derived(!lite?.assignee_id)

  /** Rows beyond the clearing one: me, the current assignee, then what the
   *  search brought — deduped by account id. */
  const assigneeCandidates = $derived.by<
    Array<{ id: string; accountId: string | null; label: string; sub: string | null }>
  >(() => {
    const out: Array<{ id: string; accountId: string | null; label: string; sub: string | null }> = []
    const seen = new Set<string>()
    const me = app.me
    const meId = me?.account_id ?? null
    if (me && meId) {
      seen.add(meId)
      out.push({
        id: 'me',
        accountId: meId,
        label: me.name || me.email || t('common.me'),
        sub: t('common.me'),
      })
    }
    const currentId = lite?.assignee_id ?? null
    if (currentId && !seen.has(currentId)) {
      seen.add(currentId)
      out.push({
        id: 'current',
        accountId: currentId,
        label: lite?.assignee ?? currentId,
        sub: null,
      })
    }
    for (const u of users) {
      if (seen.has(u.account_id)) continue
      seen.add(u.account_id)
      out.push({
        id: 'user-' + u.account_id,
        accountId: u.account_id,
        label: u.display_name,
        sub: u.email,
      })
    }
    return out
  })

  onDestroy(() => {
    searchSeq++
    if (searchTimer) clearTimeout(searchTimer)
    searchTimer = null
    searchAbort?.abort()
    searchAbort = null
  })

  function onUserQuery(next: string) {
    userQuery = next
    if (searchTimer) clearTimeout(searchTimer)
    if (next.trim().length < 2) {
      searchSeq++
      searchAbort?.abort()
      searchAbort = null
      users = []
      searching = false
      return
    }
    const seq = ++searchSeq
    searching = true
    searchTimer = setTimeout(() => void runUserSearch(next, seq), 250)
  }

  async function runUserSearch(q: string, seq: number) {
    searchAbort?.abort()
    const ctl = new AbortController()
    searchAbort = ctl
    try {
      const res = await searchUsers(issueKey, q.trim(), { signal: ctl.signal })
      if (seq !== searchSeq) return
      users = res.users
    } catch (err) {
      if (seq !== searchSeq || ctl.signal.aborted) return
      // The fixed rows stay usable; only the search band reports.
      users = []
      rowError = errorMessage(err)
    } finally {
      if (seq === searchSeq) searching = false
    }
  }

  async function pickAssignee(accountId: string | null) {
    if (writesOff || applyingId) return
    applyingId = accountId ?? 'unassigned'
    rowError = null
    failedRow = null
    try {
      const res = await setAssignee(issueKey, accountId)
      onwritten(res.issue, 'assignee')
    } catch (err) {
      if (onrefused(err)) return
      rowError = errorMessage(err)
      failedRow = applyingId
    } finally {
      applyingId = null
    }
  }
</script>

<Sheet title={t('write.pickAssignee')} {onclose}>
  <div class="pick-list">
    <!-- The field sits above what it filters: the rows below it are the
         result of what is typed here, and a filter under its own results
         reads as an afterthought. Row order stays Unassigned → me →
         current → results. -->
    <div class="search">
      <input
        value={userQuery}
        placeholder={t('write.searchNameEmail')}
        inputmode="search"
        oninput={(e) => onUserQuery(e.currentTarget.value)}
      />
      {#if searching}
        <p class="none">{t('common.searching')}</p>
      {:else if userQuery.trim().length >= 2 && users.length === 0}
        <p class="none">{t('write.userNotFound')}</p>
      {/if}
    </div>
    <button
      class="t-row"
      class:current={unassignedNow}
      aria-current={unassignedNow ? 'true' : undefined}
      disabled={applyingId !== null}
      onclick={() => void pickAssignee(null)}
    >
      <span class="t-text">
        <span class="t-name">{t('common.unassigned')}</span>
        {#if failedRow === 'unassigned' && rowError}
          <span class="t-err">{rowError}</span>
        {/if}
      </span>
      {#if unassignedNow}<CurrentTick />{/if}
    </button>
    {#each assigneeCandidates as c (c.id)}
      {@const current = Boolean(lite?.assignee_id) && lite?.assignee_id === c.accountId}
      <button
        class="t-row"
        class:current
        aria-current={current ? 'true' : undefined}
        disabled={applyingId !== null}
        onclick={() => void pickAssignee(c.accountId)}
      >
        <span class="t-text">
          <span class="t-name">{c.label}</span>
          {#if c.sub}
            <span class="t-to">{c.sub}</span>
          {/if}
          {#if failedRow === c.id && rowError}
            <span class="t-err">{rowError}</span>
          {/if}
        </span>
        {#if current}<CurrentTick />{/if}
      </button>
    {/each}
  </div>
</Sheet>

<style>
  /* also in Detail.svelte — same tokens, GDK-1925 */
  .none {
    margin: 4px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .pick-list {
    overflow-y: auto;
    padding: 4px 8px 8px;
    display: flex;
    flex-direction: column;
  }
  .search {
    border-bottom: 1px solid var(--color-border-subtle);
    padding: 4px 8px 8px;
    margin-bottom: 4px;
  }
  .search input {
    width: 100%;
    min-height: var(--spacing-control);
    padding: 0 12px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
  }
  .search .none {
    padding: 6px 2px 0;
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
  .t-to {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .t-err {
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-status-reopen);
  }
</style>
