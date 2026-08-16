<script lang="ts">
  /*
   * Keyboard cheat sheet (`?`). Every row below maps to a handler that actually
   * exists — App.svelte owns the whole list/detail set (⌘K / ? / j / k / ↵ / x /
   * s / a / l / c / Esc), SearchBox (/), CommandPalette (↑↓/Enter/Esc),
   * CommentComposer (⌘↩). Do not document a key that no handler listens for.
   */
  import { t } from '../../lib/i18n'
  import { trapFocus } from '../../lib/focus-trap'
  import Icon from '../ui/Icon.svelte'

  let { onclose }: { onclose: () => void } = $props()

  const mod = typeof navigator !== 'undefined' && /Mac|iP(hone|ad)/.test(navigator.platform)
    ? '⌘'
    : 'Ctrl'

  const sections: { title: string; rows: [string, string][] }[] = [
    {
      title: t('shortcuts.sectionGlobal'),
      rows: [
        [`${mod} K`, t('shortcuts.palette')],
        // `c` resolves three ways in keymap.svelte.ts (detail open → focus
        // comment, list cursor → comment, otherwise new issue) — the caption
        // keeps this row honest about being the fallback case.
        ['c', t('shortcuts.newIssueContext')],
        ['?', t('shortcuts.help')],
      ],
    },
    {
      title: t('shortcuts.sectionList'),
      rows: [
        ['j', t('shortcuts.moveDown')],
        ['k', t('shortcuts.moveUp')],
        ['↵', t('shortcuts.openIssue')],
        ['x', t('shortcuts.selectRow')],
        ['s', t('shortcuts.listStatus')],
        ['a', t('shortcuts.listAssignee')],
        ['l', t('shortcuts.listLabels')],
        ['c', t('shortcuts.listComment')],
        ['Esc', t('shortcuts.clearSelection')],
      ],
    },
    {
      title: t('shortcuts.sectionDetail'),
      rows: [
        ['s', t('shortcuts.focusStatus')],
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

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-[#1c1812]/28 p-4 backdrop-blur-[2px]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    use:trapFocus
    class="anim-pop flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-border-strong bg-bg-panel shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('shortcuts.title')}
    data-testid="shortcuts-dialog"
  >
    <div class="flex flex-none items-center justify-between border-b border-border-subtle px-4 py-3">
      <h2 class="type-subject text-[18px] leading-snug text-text-primary">{t('shortcuts.title')}</h2>
      <button
        type="button"
        class="flex h-control-sm w-control-sm items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        onclick={onclose}
        aria-label={t('common.closeEsc')}
        title={t('common.closeEsc')}
      >
        <Icon name="x" size={14} />
      </button>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-4 py-3">
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
                <dd class="min-w-0 flex-1 truncate text-[12px] text-text-secondary" title={label}>
                  {label}
                </dd>
              </div>
            {/each}
          </dl>
        </div>
      {/each}
    </div>
  </div>
</div>
