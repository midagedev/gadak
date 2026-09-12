<script lang="ts">
  /*
   * The session roster, in whichever of its two homes is up (GDK-1835).
   *
   * `dock` is the column on the left of the dock band, as wide as the app
   * sidebar above it, so the window's one vertical rule continues the
   * sidebar's edge and reads as two columns — navigation on the left,
   * content on the right — under one horizontal seam (GDK-1355).
   *
   * `sidebar` is what that column becomes when the pane is the full-screen
   * sheet. The sheet starts at the sidebar's right edge, so the sidebar is
   * still on screen and a second 160px rail beside it was two rails for one
   * job: at 899px that was 368px of chrome before the terminal got a column,
   * on a band whose whole point (GDK-1352) is that "a terminal needs columns
   * — a gadak one-liner runs ~120 characters". Here the rows are a block in
   * the sidebar itself and the sheet keeps every column right of it.
   *
   * One component for both because the rows, the new-shell verb and the way
   * out are the same in each; only the frame differs. The two verbs come
   * from `terminalChrome` rather than from a parent, because in the sidebar
   * home this is not rendered inside the pane that owns them.
   */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import TerminalStrip from './TerminalStrip.svelte'
  import { terminalChrome, TERMINAL_OVERLAY_ROSTER_PX } from '../../lib/terminal/pane.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'

  let { variant = 'dock' }: { variant?: 'dock' | 'sidebar' } = $props()

  const restartable = $derived(terminalChrome.restartable)
  /* Hidden only while the strip is showing its own start row (no sessions,
     restart on offer): that row is already the one verb worth having there,
     and two plus rows would ask the same question twice. With sessions still
     listed and the shown one exited, this stays — the status line restarts
     *that* shell, this makes another. */
  const showNew = $derived(!(restartable && terminalSessions.list.length === 0))
</script>

<!-- The header is the same two verbs in both homes: the way out (the pane
     swallows every keystroke on purpose, so a visible close has to exist)
     and the shape control. In the dock the terminal mark is the label; in
     the sidebar the block sits under sections that all carry a word, so it
     carries one too rather than a lone glyph among labelled neighbours. -->
{#snippet header()}
  <div
    class="flex h-7 flex-none items-center gap-2 {variant === 'sidebar'
      ? 'px-3'
      : 'pr-2 pl-4'}"
  >
    <Icon name="terminal" size={13} class="flex-none text-text-muted" />
    {#if variant === 'sidebar'}
      <span class="section-label min-w-0 flex-1 truncate">{t('sidebar.terminal')}</span>
    {:else}
      <span class="flex-1"></span>
    {/if}
    <button
      type="button"
      class="flex h-6 w-6 flex-none items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
      aria-label={terminalChrome.narrow ? t('terminal.shape.dock') : t('terminal.shape.full')}
      title={terminalChrome.narrow ? t('terminal.shape.dock') : t('terminal.shape.full')}
      aria-pressed={terminalChrome.narrow}
      data-testid="terminal-shape"
      onclick={() => terminalChrome.toggleMode()}
    >
      <Icon name={terminalChrome.narrow ? 'chevrons-down-up' : 'chevrons-up-down'} size={14} />
    </button>
    <button
      type="button"
      class="flex h-6 w-6 flex-none items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
      aria-label={t('terminal.close')}
      title="{t('terminal.close')} ({t('terminal.shortcut')})"
      data-testid="terminal-close"
      onclick={() => terminalChrome.toggle()}
    >
      <Icon name="x" size={14} />
    </button>
  </div>
{/snippet}

{#snippet rows()}
  <TerminalStrip offerStart={restartable} onstart={() => terminalChrome.restart?.()} />
  {#if showNew}
    <button
      type="button"
      class="flex h-7 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left text-body text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
      aria-label={t('terminal.strip.new')}
      title={t('terminal.strip.new')}
      data-testid="terminal-new"
      onclick={() => terminalChrome.newSession?.()}
    >
      <Icon name="plus" size={12} class="flex-none" />
      <span class="truncate">{t('terminal.strip.new')}</span>
    </button>
  {/if}
{/snippet}

{#if variant === 'sidebar'}
  <!-- No ground and no rule of its own: inside the sidebar this is one more
       block on the sidebar's own panel, and a second border there would draw
       a box around what is already in a box. -->
  <div class="flex-none pt-2" data-testid="terminal-chrome" data-variant="sidebar">
    {@render header()}
    <div class="px-2 pb-1">
      {@render rows()}
    </div>
  </div>
{:else}
  <div
    class="terminal-roster flex flex-none flex-col border-r border-border-strong bg-bg-panel"
    style:width={terminalChrome.narrow ? `${TERMINAL_OVERLAY_ROSTER_PX}px` : undefined}
    data-testid="terminal-chrome"
    data-variant="dock"
  >
    {@render header()}
    <div class="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-2 pb-1">
      {@render rows()}
    </div>
  </div>
{/if}
