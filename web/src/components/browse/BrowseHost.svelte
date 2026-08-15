<script lang="ts">
  /*
   * Where the browser pane lives in the shell — and the one place that decides
   * whether it is on screen at all.
   *
   * Rendered only inside the desktop app (App.svelte gates it on isDesktop), so
   * `gadak serve` mounts none of this and ships no path into the /desktop routes.
   *
   * The pane and the re-entry pill are alternatives: tabs stay open when the
   * pane is put away, and the pill is how they are found again. Nothing else is
   * drawn — with no tabs this component contributes no DOM, which is what keeps
   * it safe to sit inside the layout grid.
   */
  import { untrack } from 'svelte'
  import { browse, type BrowseKind } from '../../lib/browse.svelte'
  import { panel, type PanelKind } from '../../stores/panel.svelte'
  import { t } from '../../lib/i18n'
  import BrowsePane from './BrowsePane.svelte'
  import Icon from '../ui/Icon.svelte'

  /*
   * Opening something in the right panel is a request to see Gadak's copy of it,
   * and the pane covers exactly that surface — leaving it up would make a click
   * on a list row look like it did nothing. The pane steps aside; the tabs stay.
   *
   * Only on a *change*, never on mount: the pane is opened by a link click that
   * leaves the panel target alone, and a run at mount would close it on sight.
   *
   * The new target rides along as `reveal`: what the pane uncovers is what the
   * return resync must make current, not whatever the tab opened on (GDK-79).
   */
  const REVEAL_KIND: Record<PanelKind, BrowseKind | undefined> = {
    issue: 'issue',
    doc: 'page',
    person: undefined,
  }
  let lastTarget = untrack(() => panel.target)
  $effect(() => {
    const target = panel.target
    untrack(() => {
      if (target === lastTarget) return
      lastTarget = target
      if (target === null) return
      const kind = REVEAL_KIND[target.kind]
      browse.hidePane(kind ? { kind, key: target.key } : undefined)
    })
  })
</script>

{#if browse.paneOpen}
  <BrowsePane />
{:else if browse.pillVisible}
  <button
    type="button"
    onclick={() => browse.showPane()}
    title={t('browse.resumeHint', { n: browse.tabs.length })}
    data-testid="browse-reentry"
    class="browse-reentry anim-enter flex h-control items-center gap-2 rounded-lg border border-border-strong bg-bg-elevated pr-2 pl-2.5 text-body text-text-secondary shadow-overlay transition-colors hover:bg-bg-hover hover:text-text-primary"
  >
    <Icon name="globe" size={14} class="text-accent-text" />
    {t('browse.resume')}
    <span
      class="min-w-[18px] rounded bg-bg-active px-1 text-center text-micro text-text-primary"
      >{browse.tabs.length}</span
    >
  </button>
{/if}
