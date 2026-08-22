<script lang="ts">
  /*
   * Keyboard cheat sheet (`?`). Every row below maps to a handler that actually
   * exists — App.svelte owns the whole list/detail set (⌘K / , / ? / j / k / ↵ / o / x /
   * s / a / l / p / c / Esc), SearchBox (/), CommandPalette (↑↓/Enter/Esc),
   * CommentComposer (⌘↩). Do not document a key that no handler listens for.
   */
  import { t } from '../../lib/i18n'
  import { trapFocus } from '../../lib/focus-trap'
  import DialogShell from '../ui/DialogShell.svelte'

  let { onclose }: { onclose: () => void } = $props()

  const mod = typeof navigator !== 'undefined' && /Mac|iP(hone|ad)/.test(navigator.platform)
    ? '⌘'
    : 'Ctrl'

  const sections: { title: string; rows: [string, string][] }[] = [
    {
      title: t('shortcuts.sectionGlobal'),
      rows: [
        [`${mod} K`, t('shortcuts.palette')],
        [',', t('shortcuts.settings')],
        // `c` resolves three ways in keymap.svelte.ts (detail open → focus
        // comment, list cursor → comment, otherwise new issue) — the caption
        // keeps this row honest about being the fallback case.
        ['c', t('shortcuts.newIssueContext')],
        ['?', t('shortcuts.help')],
        ['Esc', t('browse.back')],
      ],
    },
    {
      title: t('shortcuts.sectionList'),
      rows: [
        ['j', t('shortcuts.moveDown')],
        ['k', t('shortcuts.moveUp')],
        ['↵', t('shortcuts.openIssue')],
        ['o', t('detail.openJira')],
        ['x', t('shortcuts.selectRow')],
        ['s', t('shortcuts.listStatus')],
        ['p', t('shortcuts.listPriority')],
        ['a', t('shortcuts.listAssignee')],
        ['l', t('shortcuts.listLabels')],
        ['c', t('shortcuts.listComment')],
        ['Esc', t('shortcuts.clearSelection')],
      ],
    },
    {
      title: t('shortcuts.sectionDetail'),
      rows: [
        ['o', t('doc.openSource')],
        ['s', t('shortcuts.focusStatus')],
        ['p', t('shortcuts.focusPriority')],
        ['a', t('shortcuts.focusAssignee')],
        ['l', t('shortcuts.focusLabels')],
        ['c', t('shortcuts.focusComment')],
      ],
    },
    {
      title: t('shortcuts.sectionSearch'),
      rows: [
        ['/', t('shortcuts.focusSearch')],
        ['↑ ↓', t('shortcuts.suggestions')],
        ['↵', t('shortcuts.applySearch')],
        ['Esc', t('shortcuts.clearSearch')],
      ],
    },
    {
      title: t('shortcuts.sectionPalette'),
      rows: [
        ['↑ ↓', t('shortcuts.paletteMove')],
        ['↵', t('shortcuts.paletteRun')],
        ['Esc', t('shortcuts.paletteClose')],
      ],
    },
    {
      title: t('shortcuts.sectionCompose'),
      rows: [[`${mod} ↵`, t('shortcuts.submitComment')]],
    },
  ]

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
