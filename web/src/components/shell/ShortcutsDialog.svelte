<script lang="ts">
  /*
   * Keyboard cheat sheet (`?`). Rows come from lib/commands.ts — the same
   * registry keymap dispatches and the palette lists. Help-only rows
   * (Tab, search/palette arrows, ⌘↵) document local handlers.
   */
  import { t } from '../../lib/i18n'
  import { trapFocus } from '../../lib/focus-trap'
  import { helpSections } from '../../lib/commands'
  import { modifierSymbol } from '../../lib/unified-search'
  import DialogShell from '../ui/DialogShell.svelte'

  let { onclose }: { onclose: () => void } = $props()

  const sections = helpSections(modifierSymbol()).map((section) => ({
    title: t(section.titleKey),
    rows: section.rows.map((row) => [row.kbd, t(row.labelKey)] as [string, string]),
  }))

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onclose()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<DialogShell
  title={t('shortcuts.title')}
  ariaLabel={t('shortcuts.title')}
  data-testid="shortcuts-dialog"
  {onclose}
  trap={trapFocus}
  panelClass="anim-pop max-h-[80vh] max-w-lg"
  headerClass="flex flex-none flex-col border-b border-border-subtle px-4 py-3"
>
  <div class="scroll-region min-h-0 flex-1 px-4 py-3">
    {#each sections as section (section.title)}
      <div class="mb-3 last:mb-0">
        <div class="mb-1 text-micro font-medium uppercase tracking-wide text-text-muted">
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
