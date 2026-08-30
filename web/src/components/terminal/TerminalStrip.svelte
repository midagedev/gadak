<script lang="ts">
  /*
   * The session strip (GDK-1153 / GDK-1163). One row per live shell; the
   * name on the row is the issue the session was claimed for (GDK-1158),
   * and clicking a row is what the pane below attaches to.
   *
   * This is a selector, not a window manager. There is still exactly one
   * <TerminalPane> mount and one socket — the strip only moves the id that
   * mount is pointed at, through the single owner in
   * lib/terminal/sessions.svelte.ts. Tiling is a second design for a
   * problem nobody has yet.
   *
   * It renders nothing at exactly one session: that name is already in the
   * pane's rail, and a chooser with one choice is furniture. At zero it is
   * not an empty table either — the row list becomes the one action worth
   * offering there, which is the same restart the status line offers, sent
   * through the same data sink (GDK-991).
   */
  import { onMount } from 'svelte'
  import { relativeTime, t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'
  import { stripRows, stripShowsRows, type TerminalSessionState } from '../../lib/terminal/strip'

  let {
    offerStart = false,
    onstart,
  }: {
    /** The pane's own verdict that a restart can succeed here. The empty
     *  state rides it so a healthy boot — where the roster is briefly empty
     *  because the create has not landed — never flashes a start button. */
    offerStart?: boolean
    onstart: () => void
  } = $props()

  onMount(() => terminalSessions.watch())

  // Recomputed on every roster poll; Date.now() is read there rather than
  // ticked separately, so "running" is at most one poll stale and the strip
  // has no clock of its own to keep in step.
  const rows = $derived(stripRows(terminalSessions.list, terminalSessions.selectedId, Date.now()))
  const showEmpty = $derived(rows.length === 0 && offerStart)
  const visible = $derived(stripShowsRows(rows.length) && (rows.length > 0 || showEmpty))

  // Existing tokens only — this strip introduces no colour. reopen is the
  // app's "something wants you" ink, inprogress its "work is happening"
  // ink, new its resting ink; a ghost is a border, not a status.
  const DOT: Record<TerminalSessionState, string> = {
    needs: 'bg-status-reopen',
    running: 'bg-status-inprogress',
    quiet: 'bg-status-new',
    ghost: 'bg-border-strong',
  }
  const STATE_KEY = {
    needs: 'terminal.strip.state.needs',
    running: 'terminal.strip.state.running',
    quiet: 'terminal.strip.state.quiet',
    ghost: 'terminal.strip.state.ghost',
  } as const

  function stateLabel(state: TerminalSessionState): string {
    return t(STATE_KEY[state])
  }
</script>

{#if visible}
  <div
    class="max-h-24 flex-none overflow-x-hidden overflow-y-auto border-b border-border-subtle bg-bg-panel"
    role="group"
    aria-label={t('terminal.strip.list')}
    data-testid="terminal-strip"
    data-count={rows.length}
  >
    {#if showEmpty}
      <button
        type="button"
        class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
        data-testid="terminal-strip-start"
        onclick={onstart}
      >
        <!-- The plus the rail already uses, not a status dot: this row is a
             verb, and a dot in the slot where the other rows carry state
             would read as a state this row does not have. -->
        <Icon name="plus" size={12} class="flex-none" />
        <span class="truncate">{t('terminal.strip.start')}</span>
      </button>
    {:else}
      {#each rows as row (row.id)}
        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2 px-3 py-1 text-left hover:bg-bg-hover"
          class:bg-bg-active={row.selected}
          aria-current={row.selected ? 'true' : undefined}
          aria-label={t('terminal.strip.show', { name: row.label })}
          title={stateLabel(row.state)}
          data-testid="terminal-strip-row"
          data-session-id={row.id}
          data-state={row.state}
          data-selected={row.selected ? 'true' : 'false'}
          onclick={() => terminalSessions.select(row.id)}
        >
          <span class="h-1.5 w-1.5 flex-none rounded-full {DOT[row.state]}"></span>
          <span
            class="truncate text-body"
            class:text-text-primary={row.selected}
            class:text-text-secondary={!row.selected}
            class:font-medium={row.namedByIssue}
            data-testid="terminal-strip-name">{row.label}</span
          >
          <span class="ml-auto flex-none truncate text-micro text-text-muted">
            {stateLabel(row.state)}{row.since ? ` · ${relativeTime(row.since, 'compact')}` : ''}
          </span>
        </button>
      {/each}
    {/if}
  </div>
{/if}
