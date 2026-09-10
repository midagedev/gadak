<script lang="ts">
  /*
   * Keyboard cheat sheet (`?`). Rows come from lib/commands.ts — the same
   * registry keymap dispatches and the palette lists. Help-only rows
   * (Tab, search/palette arrows, ⌘↵) document local handlers.
   */
  import { t } from '../../lib/i18n'
  import { trapFocus } from '../../lib/focus-trap'
  import { helpSections, keyContext } from '../../lib/commands'
  import { modifierSymbol } from '../../lib/unified-search'
  import { config, originTrackerName } from '../../lib/config'
  import { ORIGIN_GADAK } from '../../lib/workspace'
  import { browse } from '../../lib/browse.svelte'
  import { ESC_TIER, isEscapeKey, onEscape } from '../../lib/dom-actions'
  import DialogShell from '../ui/DialogShell.svelte'

  let { onclose }: { onclose: () => void } = $props()

  // The origin rows name the tracker ("Open the issue in {tracker}"); the
  // placeholder is inert on every other row.
  const params = { tracker: originTrackerName() }
  // The same context the resolver sees, with the two facts this sheet reads:
  // whether the browse pane is open (Esc ← back is its shortcut) and whether
  // this origin has pages the `o` chord can open. The registry's `when`
  // gates own the hiding now — GDK-1589 replaced the dialog's own
  // labelKey.endsWith('OpenJira') string filter (the GDK-1313 workaround).
  const ctx = $derived(
    keyContext({
      browsePaneOpen: browse.paneOpen,
      originOpenable: config().originType !== ORIGIN_GADAK,
    }),
  )
  const sections = $derived(
    helpSections(modifierSymbol(), ctx)
      .map((section) => ({
        title: t(section.titleKey),
        rows: section.rows.map((row) => [row.kbd, t(row.labelKey, params)] as [string, string]),
      }))
      .filter((section) => section.rows.length > 0),
  )

  // Dialog-tier claim on the Esc stack, acting-and-spending so the surface
  // under it keeps its own Esc (GDK-1565).
  function onKeydown(e: KeyboardEvent) {
    if (isEscapeKey(e)) {
      e.preventDefault()
      onclose()
    }
  }
</script>

<DialogShell
  title={t('shortcuts.title')}
  ariaLabel={t('shortcuts.title')}
  data-testid="shortcuts-dialog"
  {onclose}
  trap={trapFocus}
  panelClass="anim-pop max-h-[80vh] max-w-lg"
  headerClass="flex flex-none flex-col border-b border-border-subtle px-4 py-3"
>
  <div
    class="scroll-region min-h-0 flex-1 px-4 py-3"
    use:onEscape={{ handler: onKeydown, priority: ESC_TIER.dialog, label: 'shortcuts' }}
  >
    {#each sections as section (section.title)}
      <div class="mb-3 last:mb-0">
        <div class="mb-1 section-label">
          {section.title}
        </div>
        <dl class="flex flex-col">
          {#each section.rows as [keys, label] (label + keys)}
            <div class="flex items-center gap-3 border-b border-border-subtle/60 py-1.5 last:border-0">
              <dt class="w-24 flex-none">
                <kbd
                  class="rounded border border-border-strong bg-bg-elevated px-1.5 py-0.5 font-mono text-micro text-text-secondary"
                >
                  {keys}
                </kbd>
              </dt>
              <dd class="min-w-0 flex-1 truncate text-body text-text-secondary" title={label}>
                {label}
              </dd>
            </div>
          {/each}
        </dl>
      </div>
    {/each}
  </div>
</DialogShell>
