<script lang="ts">
  import Queue from './screens/Queue.svelte'
  import Pair from './screens/Pair.svelte'
  import { t } from './lib/i18n'

  type Screen = 'queue' | 'pair'
  let screen = $state<Screen>('queue')
</script>

<div class="m-shell">
  <header class="m-header">
    <span class="type-subject wordmark">gadak</span>
  </header>
  {#if screen === 'queue'}
    <Queue onpair={() => (screen = 'pair')} />
  {:else}
    <Pair onsaved={() => (screen = 'queue')} />
  {/if}
  <nav class="m-tabbar">
    <button
      class="m-tab"
      class:m-tab-active={screen === 'queue'}
      type="button"
      onclick={() => (screen = 'queue')}
      aria-current={screen === 'queue' ? 'page' : undefined}
    >
      {t('nav.queue')}
    </button>
    <button
      class="m-tab"
      class:m-tab-active={screen === 'pair'}
      type="button"
      onclick={() => (screen = 'pair')}
      aria-current={screen === 'pair' ? 'page' : undefined}
    >
      {t('nav.pair')}
    </button>
  </nav>
</div>
