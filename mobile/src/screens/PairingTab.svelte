<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { t } from '../lib/i18n'
  import { app, sync, unpair } from '../lib/store.svelte'
  import { relTime, hasIdentity } from '../lib/domain'

  // Rarely visited, always honest: what am I paired to, how fresh is the
  // snapshot, who does the serve think I am. The one destructive rarity —
  // Unpair — uses the house two-step arm (UX_PRINCIPLES §7): first tap
  // arms, second tap within 3s fires, no modal.
  let armed = $state(false)
  let armTimer: ReturnType<typeof setTimeout> | null = null

  // Dev-only viewport telemetry: the phone has no console, and vertical
  // geometry bugs (§4.2) need numbers, not guesses.
  const DEV = import.meta.env.DEV
  let viewportProbe = $state('')
  $effect(() => {
    if (!DEV) return
    const update = () => {
      const vv = window.visualViewport
      viewportProbe =
        `inner ${window.innerWidth}x${window.innerHeight}` +
        (vv ? ` · vv ${Math.round(vv.width)}x${Math.round(vv.height)} @${Math.round(vv.offsetTop)}` : '') +
        ` · screen ${screen.width}x${screen.height}` +
        ` · safe-t ${getComputedStyle(document.documentElement).getPropertyValue('--probe-safe-top') || '?'}` +
        ` · safe-b ${getComputedStyle(document.documentElement).getPropertyValue('--probe-safe-bottom') || '?'}`
    }
    update()
    const id = setInterval(update, 1000)
    return () => clearInterval(id)
  })

  function onUnpair() {
    if (!armed) {
      armed = true
      if (armTimer) clearTimeout(armTimer)
      armTimer = setTimeout(() => (armed = false), 3000)
      return
    }
    if (armTimer) clearTimeout(armTimer)
    void unpair()
  }

  function host(endpoint: string): string {
    if (endpoint === '') return 'this machine (dev proxy)'
    try {
      return new URL(endpoint).host
    } catch {
      return endpoint
    }
  }

  function expiry(iso: string): string {
    if (!iso) return ''
    const t = new Date(iso)
    if (isNaN(t.getTime())) return ''
    return t.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
  }
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <h1 class="type-subject">Pairing</h1>
    </div>
  {/snippet}

  <div class="page">
    {#if app.meta}
      <section>
        <h3>Paired server</h3>
        <p class="big">{app.meta.label || host(app.meta.endpoint)}</p>
        <p class="sub mono">{host(app.meta.endpoint)}</p>
        {#if expiry(app.meta.expires_at)}
          <p class="sub">Offer expires {expiry(app.meta.expires_at)}</p>
        {/if}
      </section>

      <section>
        <h3>Mirror</h3>
        <p class="line">
          <span>{app.issues.length} issues</span>
          <span class="quiet">
            {#if app.offline}
              offline — last sync {app.lastSyncAt ? relTime(app.lastSyncAt.toISOString(), app.now) : 'never'}
            {:else if app.syncing}
              syncing…
            {:else if app.lastSyncAt}
              synced {relTime(app.lastSyncAt.toISOString(), app.now)}
            {:else}
              not synced yet
            {/if}
          </span>
        </p>
        <button class="act" onclick={() => void sync()} disabled={app.syncing}>{t('sidebar.syncNow')}</button>
      </section>

      <section>
        <h3>Identity</h3>
        {#if hasIdentity(app.me)}
          <p class="big">{app.me?.name || app.me?.email}</p>
          {#if app.me?.email && app.me?.name}
            <p class="sub">{app.me.email}</p>
          {/if}
          <p class="sub">{t('personal.myAssignee')} filters to this identity.</p>
        {:else}
          <p class="line"><span class="quiet">No identity — the serve runs standalone, so {t('doc.issues')} opens on {t('view.allOpen.name')}.</span></p>
        {/if}
      </section>

      <section>
        <button class="unpair" class:armed onclick={onUnpair}>
          {armed ? 'Tap again to unpair' : 'Unpair this phone'}
        </button>
        <p class="sub center">Unpairing forgets the server and deletes the token from the Keychain.</p>
      </section>
    {/if}

    <p class="ver">gadak mobile 0.1.0</p>
    {#if DEV}
      <p class="probe">DEV {viewportProbe}</p>
    {/if}
  </div>
</Screen>

<style>
  .head {
    display: flex;
    align-items: baseline;
    padding: 12px 0 10px;
  }
  h1 {
    margin: 0;
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
  }
  .page {
    padding: 4px 16px 24px;
    display: flex;
    flex-direction: column;
  }
  section {
    padding: 4px 0 12px;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  h3 {
    margin: 0 0 6px;
    padding: 10px 0 4px;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .big {
    margin: 0;
    font-weight: 600;
  }
  .sub {
    margin: 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .mono {
    font-family: var(--font-mono);
  }
  .line {
    margin: 0;
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .quiet {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .act {
    align-self: flex-start;
    margin-top: 4px;
    margin-left: -4px;
    padding: 0 4px;
    color: var(--color-accent-text);
    font-size: var(--text-micro);
    font-weight: 600;
  }
  .act:disabled {
    opacity: 0.45;
  }
  .unpair {
    width: 100%;
    margin-top: 8px;
    border-radius: 6px;
    border: 1px solid var(--color-border-strong);
    color: var(--color-text-primary);
    font-weight: 600;
    background: transparent;
  }
  .unpair.armed {
    background: var(--color-text-primary);
    border-color: var(--color-text-primary);
    color: var(--color-bg-base);
  }
  .center {
    text-align: center;
    margin-top: 6px;
  }
  .ver {
    margin: 16px 0 0;
    text-align: center;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
  }
  .probe {
    margin: 12px 0 0;
    padding-top: 10px;
    border-top: 1px solid var(--color-border-subtle);
    text-align: left;
    font-size: var(--text-micro);
    font-family: var(--font-mono);
    color: var(--color-text-muted);
    overflow-wrap: anywhere;
  }
</style>
