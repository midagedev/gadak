<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import { t } from '../lib/i18n'
  import { systemBack } from '../lib/back'
  import { keyboardInset } from '../lib/keyboard'

  // Bottom sheet: scrim + rising panel, thumb territory. The bottom inset
  // is a property of where the sheet sits (app.css: .detail-layer .sheet),
  // not a class the caller remembers to pass.
  //
  // `tall` is for sheets whose content is a writing surface (the
  // description editor): the panel claims most of the screen instead of
  // the picker's 70%. keyboardInset rides every sheet now — in WKWebView
  // the software keyboard overlays the layout viewport, so an input near
  // the panel's bottom would sit under it without the translate. It is a
  // no-op without a VisualViewport (headless capture, desktop browsers).
  let {
    title,
    onclose,
    tall = false,
    children,
  }: { title: string; onclose: () => void; tall?: boolean; children: Snippet } = $props()

  $effect(() => {
    return systemBack.registerSheet(onclose)
  })
</script>

<button
  type="button"
  class="scrim"
  transition:fade={{ duration: 150 }}
  onclick={onclose}
  aria-label={t('common.cancel')}
></button>
<div
  class="sheet"
  class:tall
  role="dialog"
  aria-modal="true"
  aria-label={title}
  transition:fly={{ y: 320, duration: 240 }}
  use:keyboardInset
>
  <div class="grab" aria-hidden="true"></div>
  <div class="head">
    <h2>{title}</h2>
    <button class="cancel" onclick={onclose}>{t('common.cancel')}</button>
  </div>
  {@render children()}
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    display: block;
    width: 100%;
    background: var(--color-scrim);
    z-index: 30;
    border-radius: 0;
    appearance: none;
    -webkit-appearance: none;
  }
  .sheet {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 31;
    background: var(--color-bg-panel);
    border-radius: 12px 12px 0 0;
    box-shadow: var(--shadow-overlay);
    max-height: 70%;
    display: flex;
    flex-direction: column;
  }
  .sheet.tall {
    max-height: 92%;
  }
  .grab {
    flex: none;
    width: 36px;
    height: 4px;
    border-radius: 9999px;
    background: var(--color-border-strong);
    margin: 8px auto 0;
  }
  .head {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 16px 4px;
  }
  h2 {
    margin: 0;
    font-size: var(--text-title);
    line-height: var(--text-title--line-height);
    font-family: var(--font-display);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .cancel {
    padding: 0 8px;
    color: var(--color-accent-text);
    font-size: var(--text-body);
  }
</style>
