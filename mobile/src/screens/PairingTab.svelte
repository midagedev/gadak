<script lang="ts">
  import Screen from '../ui/Screen.svelte'
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
        <div class="card">
          <p class="big">{app.meta.label || host(app.meta.endpoint)}</p>
          <p class="sub mono">{host(app.meta.endpoint)}</p>
          {#if expiry(app.meta.expires_at)}
            <p class="sub">Offer expires {expiry(app.meta.expires_at)}</p>
          {/if}
        </div>
      </section>

      <section>
        <h3>Mirror</h3>
        <div class="card">
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
          <button class="act" onclick={() => void sync()} disabled={app.syncing}>Sync now</button>
        </div>
      </section>

      <section>
        <h3>Identity</h3>
        <div class="card">
          {#if hasIdentity(app.me)}
            <p class="big">{app.me?.name || app.me?.email}</p>
            {#if app.me?.email && app.me?.name}
              <p class="sub">{app.me.email}</p>
            {/if}
            <p class="sub">"Mine" in the queue filters to this identity.</p>
          {:else}
            <p class="line"><span class="quiet">No identity — the serve runs standalone, so the queue shows all open issues.</span></p>
          {/if}
        </div>
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
      <p class="ver">{viewportProbe}</p>
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
    gap: 20px;
  }
  h3 {
    margin: 0 0 6px;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .card {
    background: var(--color-bg-panel);
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 4px;
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
    margin-top: 6px;
    min-height: var(--spacing-control-sm);
    padding: 0 12px;
    border-radius: 6px;
    border: 1px solid var(--color-border-strong);
    color: var(--color-accent-text);
    font-size: var(--text-micro);
    font-weight: 600;
  }
  .act:disabled {
    opacity: 0.45;
  }
  .unpair {
    width: 100%;
    min-height: var(--spacing-control);
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    color: var(--color-status-reopen);
    font-weight: 600;
    background: var(--color-bg-panel);
  }
  .unpair.armed {
    background: var(--color-status-reopen);
    border-color: var(--color-status-reopen);
    color: var(--color-bg-base);
  }
  .center {
    text-align: center;
    margin-top: 6px;
  }
  .ver {
    margin: 8px 0 0;
    text-align: center;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
  }
</style>
