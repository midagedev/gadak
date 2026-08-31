<script lang="ts">
  /*
   * The session strip (GDK-1153 / GDK-1163). One tab per live shell; the
   * name on the tab is the issue the session was claimed for (GDK-1158),
   * and clicking a tab is what the pane below attaches to.
   *
   * GDK-1194 turned it on its side: the terminal is a bottom dock now, and
   * the axis it has to spare is horizontal — a vertical roster inside a
   * 40%-tall dock spent the pane's scarce dimension on chrome. GDK-1199
   * then folded the dock's two chrome rows into one: this strip is the tab
   * half of that row (TerminalPane owns the row, the icon and the +/×
   * buttons), so it paints no border or panel of its own. Same testids,
   * same single owner; the DOM order is still the roster order, so a
   * keyboard walks the tabs the way it walked the rows.
   *
   * This is a selector, not a window manager. There is still exactly one
   * <TerminalPane> mount and one socket — the strip only moves the id that
   * mount is pointed at, through the single owner in
   * lib/terminal/sessions.svelte.ts. Tiling is a second design for a
   * problem nobody has yet.
   *
   * A single session is still a tab: the row is resident chrome now
   * (GDK-1199), so showing the one name costs nothing — the tab took over
   * the naming job the rail carried under GDK-1153. At zero it is not an
   * empty table either — the tab slot becomes the one action worth offering
   * there, which is the same restart the status line offers, sent through
   * the same data sink (GDK-991). Each tab carries an × on hover (GDK-1200):
   * the explicit end of that session, no confirmation, the same DELETE the
   * server has kept since GDK-922.
   */
  import { onMount } from 'svelte'
  import { relativeTime, t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'
  import { stripRows, type TerminalSessionState } from '../../lib/terminal/strip'

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

<div
  class="flex min-w-0 flex-1 items-stretch self-stretch overflow-x-auto overflow-y-hidden"
  role="group"
  aria-label={t('terminal.strip.list')}
  data-testid="terminal-strip"
  data-count={rows.length}
>
  {#if showEmpty}
    <button
      type="button"
      class="flex flex-none cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary"
      data-testid="terminal-strip-start"
      onclick={onstart}
    >
      <!-- The plus the chrome row already uses, not a status dot: this row
           is a verb, and a dot in the slot where the other rows carry state
           would read as a state this row does not have. -->
      <Icon name="plus" size={12} class="flex-none" />
      <span class="truncate">{t('terminal.strip.start')}</span>
    </button>
  {:else}
    {#each rows as row (row.id)}
      <button
        type="button"
        class="group flex max-w-48 min-w-0 flex-none cursor-pointer items-center gap-1.5 border-r border-b-2 border-r-border-subtle px-2.5 py-1 text-left hover:bg-bg-hover"
        class:bg-bg-active={row.selected}
        class:border-b-text-secondary={row.selected}
        class:border-b-transparent={!row.selected}
        aria-current={row.selected ? 'true' : undefined}
        aria-label={t('terminal.strip.show', { name: row.label })}
        title="{row.label} · {stateLabel(row.state)}{row.since
          ? ` · ${relativeTime(row.since, 'compact')}`
          : ''}"
        data-testid="terminal-strip-row"
        data-session-id={row.id}
        data-state={row.state}
        data-selected={row.selected ? 'true' : 'false'}
        data-issue-key={row.namedByIssue ? row.label : undefined}
        onclick={() => terminalSessions.select(row.id)}
        {@attach (el) => {
          // The selected tab has to be on screen to be a selection you can
          // see; with many shells the row scrolls sideways past the edge.
          if (row.selected) el.scrollIntoView({ block: 'nearest', inline: 'nearest' })
        }}
      >
        <span class="h-1.5 w-1.5 flex-none rounded-full {DOT[row.state]}"></span>
        <span
          class="truncate text-body"
          class:text-text-primary={row.selected}
          class:text-text-secondary={!row.selected}
          class:font-medium={row.namedByIssue}
          data-testid="terminal-strip-name">{row.label}</span
        >
        <!-- A <button> here would be a button inside the tab's own <button>
             (invalid DOM, and Firefox drops the inner one), so this carries
             the role and the keys itself — the BoardCard.svelte precedent.
             Both handlers stop propagation: ending a session is not
             selecting its tab. On a ghost the same DELETE is just tidying a
             shell that already has nobody (GDK-1200). -->
        <span
          role="button"
          tabindex="0"
          data-testid="terminal-strip-kill"
          aria-label={t('terminal.strip.kill', { name: row.label })}
          title={t('terminal.strip.kill', { name: row.label })}
          class="flex h-4 w-4 flex-none items-center justify-center rounded text-text-muted opacity-0 transition-opacity group-hover:opacity-100 hover:bg-bg-hover hover:text-text-primary focus-visible:opacity-100"
          onclick={(e) => {
            e.stopPropagation()
            void terminalSessions.kill(row.id)
          }}
          onkeydown={(e) => {
            if (e.key !== 'Enter' && e.key !== ' ') return
            e.preventDefault()
            e.stopPropagation()
            void terminalSessions.kill(row.id)
          }}
        >
          <Icon name="x" size={10} />
        </span>
      </button>
    {/each}
  {/if}
</div>
