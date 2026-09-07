<script lang="ts">
  /*
   * Full-screen image viewer for an ADF attachment (GDK-1503).
   *
   * The desktop opens its mediaViewer store; the phone has no such store and
   * does not want one — the only caller is AdfBody, and the image it shows is
   * the object URL that body already fetched, so this component owns no bytes
   * and no fetch. Sheet.svelte is the wrong host: it is a bottom sheet capped
   * at 70% with a grab handle, and a photograph wants the whole screen. What
   * is reused is Sheet's scrim pattern and, more importantly, its contract
   * with the system back gesture (lib/back.ts): the layer registers the same
   * onclose its Close button and its scrim call, so one back closes the
   * viewer instead of the detail screen underneath.
   *
   * Zoom is the platform's: `touch-action: pinch-zoom` hands the gesture to
   * WKWebView instead of reimplementing a transform matrix in JS. That is
   * also why the image is not draggable — a pinch inside the layer scales the
   * page region, and a tap on the scrim leaves.
   */
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n'
  import { systemBack } from '../lib/back'

  let {
    name,
    src,
    onclose,
  }: { name: string; src: string; onclose: () => void } = $props()

  $effect(() => {
    return systemBack.registerSheet(onclose)
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="viewer"
  role="dialog"
  aria-modal="true"
  aria-label={name}
  data-testid="attachment-viewer"
  tabindex="-1"
  transition:fade={{ duration: 120 }}
  onclick={onclose}
>
  <div class="bar">
    <span class="name">{name}</span>
    <button type="button" class="close" onclick={onclose}>{t('detail.viewerClose')}</button>
  </div>
  <!-- The stage swallows its own taps so a pinch or a mis-tap on the photo
       does not dismiss; only the ground around it and Close do. -->
  <div class="stage" onclick={(e) => e.stopPropagation()}>
    <img {src} alt={name} />
  </div>
</div>

<style>
  /* Fixed, not absolute: the body this layer is rendered from is a
     scrolling section, so an absolute inset:0 would size to the scrolled
     content. AdfBody's copied pill already takes the same road. */
  .viewer {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    flex-direction: column;
    /* The one place the phone paints a color the theme does not own, and
       deliberately: a viewer's plate is a darkroom, not a surface — a
       photograph must read the same on either ground. The desktop viewer
       makes the identical call (web/src/components/detail/MediaViewer.svelte
       :37 `bg-black/90`, :48 `text-white/90`), which is what this and the
       bar below match. No hex appears here; app.css keeps every *theme*
       color.

       Opaque, not the desktop's /90: the desk sits on a dark app while the
       phone's light theme is cream. At 0.97 the page behind measured 1–7 of
       255 on mobile/e2e/.shots/attach-image-viewer.png — and a blind vision
       judge read the description paragraphs word for word through it
       (GDK-1503 vision round). Seven levels of near-black is legible text on
       a phone panel; a plate that must hide the page is solid. */
    background: rgb(0 0 0);
  }
  .bar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: max(var(--safe-top), 8px) 16px 8px;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-micro);
    /* On the viewer's own plate, not the theme's (see .viewer). */
    color: rgb(255 255 255 / 0.82);
  }
  .close {
    flex: none;
    padding: 4px 8px;
    font-size: var(--text-body);
    color: rgb(255 255 255 / 0.92);
  }
  .stage {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 8px calc(max(var(--safe-bottom), 8px) + 8px);
    /* The platform's pinch, not a JS transform. */
    touch-action: pinch-zoom;
  }
  img {
    display: block;
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
</style>
