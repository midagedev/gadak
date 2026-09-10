<script lang="ts">
  /*
   * Right detail panel frame ([foundation]).
   * Occupies width only when `open` (issue selected). Wide screens use a grid
   * track; narrow screens use an overlay.
   *
   * Selection contract: open state comes from [explore] selection store
   * (`selectedKey`) via App.svelte's `open` prop — this component only displays.
   *
   * `modal` (GDK-201/GDK-1585) is the same verdict as a prop: while the
   * panel covers the list it is a real dialog — role, aria-modal and the
   * focus trap render here instead of an external DOM walk. The trap stays
   * action-shaped: the element is always mounted (the grid track expects
   * it), so a bare use:trapFocus would trap a docked panel too — this
   * wrapper composes the same trapFocus the nine dialogs use and answers
   * the `modal` parameter Svelte re-invokes `update` with.
   */
  import type { ActionReturn } from 'svelte/action'
  import type { Snippet } from 'svelte'
  import { trapFocus } from '../../lib/focus-trap'

  let {
    open = false,
    modal = false,
    children,
  }: { open?: boolean; modal?: boolean; children?: Snippet } = $props()

  function trapWhileModal(node: HTMLElement, on: boolean): ActionReturn<boolean> {
    let trapped: ReturnType<typeof trapFocus> | null = on ? trapFocus(node) : null
    return {
      update(next: boolean) {
        if (next === (trapped !== null)) return
        if (next) trapped = trapFocus(node)
        else {
          trapped?.destroy()
          trapped = null
        }
      },
      destroy() {
        trapped?.destroy()
        trapped = null
      },
    }
  }
</script>

<aside
  class="issue-detail-panel h-full overflow-hidden bg-bg-panel"
  class:is-open={open}
  class:border-l={open}
  class:border-border-strong={open}
  aria-hidden={!open}
  role={modal ? 'dialog' : undefined}
  aria-modal={modal ? 'true' : undefined}
  data-testid="issue-detail-panel"
  use:trapWhileModal={modal}
>
  <div class="h-full w-full min-w-0 overflow-y-auto">
    {@render children?.()}
  </div>
</aside>
