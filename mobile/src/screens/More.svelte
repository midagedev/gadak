<script lang="ts">
  /*
   * More — the third tab (A-nav). Two things by design (ux-report Q8:
   * tabs are navigation, cap 3 — Queue / Search / 더보기): the pair
   * management entry (Pair lives one push away, "Pair는 온보딩이지 탭이
   * 아니다") and one line of app info. Anything else a settings screen
   * would grow is explicitly outside the MVP (ux-report 결론 요약 10).
   */
  import { getVersion } from '@tauri-apps/api/app'
  import type { Pairing } from '../lib/settings'
  import { t } from '../lib/i18n'

  let {
    pairing = null,
    onopenpair,
  }: { pairing?: Pairing | null; onopenpair?: () => void } = $props()

  let version = $state('')

  $effect(() => {
    // core:app:default allows getVersion in the packaged app; a plain
    // browser (vite dev) has none to read and the line stays plain — a
    // version nobody measured is never invented here.
    getVersion()
      .then((v) => (version = v))
      .catch(() => {})
  })
</script>

<section class="m-main scroll-region" data-testid="more-screen">
  <h1 class="type-subject m-queue-title">{t('nav.more')}</h1>

  <button class="more-row" type="button" onclick={() => onopenpair?.()} data-testid="more-pair">
    <span class="more-row-main">
      <span class="more-row-title">{t('pair.title')}</span>
      <span class="more-row-status" data-testid="more-pair-status">
        {pairing
          ? pairing.label !== ''
            ? t('pair.current', { label: pairing.label })
            : t('pair.currentFallback')
          : t('more.pair.unpaired')}
      </span>
    </span>
    <span class="more-chevron" aria-hidden="true">›</span>
  </button>

  <p class="more-about" data-testid="more-about">
    {version !== '' ? t('more.about', { version }) : t('more.aboutPlain')}
  </p>
</section>

<style>
  /* Scoped — app.css is frozen this round; tokens only, 44pt floor on the row. */
  .more-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    min-height: 44px;
    margin: 0.75rem 0 0;
    padding: 0.5rem 0.25rem;
    border: 0;
    border-bottom: 1px solid var(--color-border-subtle);
    background: transparent;
    font-family: var(--font-sans);
    text-align: left;
    cursor: pointer;
  }

  .more-row-main {
    display: flex;
    flex: 1 1 auto;
    min-width: 0;
    flex-direction: column;
    gap: 0.125rem;
  }

  .more-row-title {
    color: var(--color-text-primary);
    font-size: var(--text-body);
  }

  .more-row-status {
    color: var(--color-text-muted);
    font-size: var(--text-micro);
    overflow-wrap: anywhere;
  }

  .more-chevron {
    flex: none;
    color: var(--color-text-muted);
    font-size: var(--text-heading);
    line-height: 1;
  }

  .more-about {
    margin: 1rem 0.25rem 0;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }
</style>
