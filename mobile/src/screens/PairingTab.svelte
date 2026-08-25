<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { t } from '../lib/i18n'
  import { app, sync, unpair, pairTerminal, unpairTerminal } from '../lib/store.svelte'
  import { relTime, hasIdentity } from '../lib/domain'
  import { decodeOffer, OfferError } from '../lib/offer'
  import { ApiError, errorMessage } from '../lib/api'

  // Rarely visited, always honest: what am I paired to, how fresh is the
  // snapshot, who does the serve think I am. The one destructive rarity —
  // Unpair — uses the house two-step arm (UX_PRINCIPLES §7): first tap
  // arms, second tap within 3s fires, no modal.
  let armed = $state(false)
  let armTimer: ReturnType<typeof setTimeout> | null = null
  let termArmed = $state(false)
  let termArmTimer: ReturnType<typeof setTimeout> | null = null
  let termOffer = $state('')
  let termBusy = $state(false)
  let termError = $state<string | null>(null)

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

  // Friendly copy per decoder refusal — same mapping as PairGate.svelte.
  function offerCopy(e: OfferError): string {
    const m = e.message
    if (m.includes('empty')) return 'Paste the offer line first.'
    if (m.includes('version')) return 'This offer is from a newer gadak. Update the app, then pair.'
    return 'That does not look like a pairing offer. Copy the whole line from `gadak pairing mint`.'
  }

  function terminalProbeCopy(err: unknown): string {
    if (err instanceof OfferError) return offerCopy(err)
    if (err instanceof ApiError && err.code === 'scope_rejected') {
      // A serve QR scanned into the terminal slot. Distinct from expired.
      return 'This offer is for the issue mirror, not the shell. Mint a terminal-scope offer on the desktop and pair again.'
    }
    if (err instanceof ApiError && err.code === 'pairing_rejected') {
      return 'This code is expired or revoked. Mint a new terminal offer on the desktop and pair again.'
    }
    return errorMessage(err)
  }

  async function submitTerminal() {
    if (termBusy) return
    termError = null
    termBusy = true
    try {
      const offer = decodeOffer(termOffer)
      await pairTerminal(offer)
      termOffer = ''
    } catch (err) {
      termError = terminalProbeCopy(err)
    } finally {
      termBusy = false
    }
  }

  async function pasteAndPairTerminal() {
    termError = null
    try {
      const text = (await navigator.clipboard.readText()).trim()
      if (text === '') {
        termError = 'Clipboard is empty. Copy the offer line first.'
        return
      }
      termOffer = text
    } catch {
      termError = 'Could not read the clipboard. Paste into the field instead.'
      return
    }
    await submitTerminal()
  }

  // Same scan block as PairGate.svelte — do not re-derive the plugin call.
  async function scanTerminal() {
    termError = null
    try {
      const { scan: scanQR, Format, cancel } = await import('@tauri-apps/plugin-barcode-scanner')
      const result = await scanQR({ windowed: false, formats: [Format.QRCode] })
      void cancel
      if (result?.content) {
        termOffer = result.content
        await submitTerminal()
      }
    } catch {
      termError = 'Could not open the camera. Paste the offer line instead.'
    }
  }

  function onUnpairTerminal() {
    if (!termArmed) {
      termArmed = true
      if (termArmTimer) clearTimeout(termArmTimer)
      termArmTimer = setTimeout(() => (termArmed = false), 3000)
      return
    }
    if (termArmTimer) clearTimeout(termArmTimer)
    void unpairTerminal()
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
        <h3>{t('sidebar.terminal')}</h3>
        {#if app.terminal}
          <p class="big">{app.terminal.label || host(app.terminal.endpoint)}</p>
          <p class="sub mono">{host(app.terminal.endpoint)}</p>
          <button class="unpair-shell" class:armed={termArmed} onclick={onUnpairTerminal}>
            {termArmed ? 'Tap again to unpair' : 'Unpair the shell'}
          </button>
        {:else}
          <label class="lbl" for="term-offer">Terminal offer</label>
          <textarea
            id="term-offer"
            bind:value={termOffer}
            rows="3"
            placeholder="Paste the terminal-scope offer line"
            autocapitalize="off"
            spellcheck="false"
          ></textarea>
          <p class="sub">
            On the desktop: <span class="mono">gadak pairing mint --scope terminal</span> prints one line.
          </p>
          {#if termError}
            <p class="error" role="alert">{termError}</p>
          {/if}
          {#if termOffer.trim() === ''}
            <button class="act" disabled={termBusy} onclick={() => void pasteAndPairTerminal()}>
              {termBusy ? 'Checking…' : 'Paste & pair'}
            </button>
          {:else}
            <button class="act" disabled={termBusy} onclick={() => void submitTerminal()}>
              {termBusy ? 'Checking…' : 'Pair'}
            </button>
          {/if}
          {#if !DEV}
            <button class="act" onclick={() => void scanTerminal()}>Scan</button>
          {/if}
        {/if}
      </section>

      <section>
        <button class="unpair" class:armed onclick={onUnpair}>
          {armed ? 'Tap again to unpair' : 'Unpair this phone'}
        </button>
        <p class="sub center">Unpairing forgets the server and deletes both its pairing token and the shell's from the Keychain.</p>
      </section>
    {/if}

    <p class="ver">gadak mobile 0.1.0</p>
    {#if DEV}
      <p class="probe" data-testid="viewport-probe" hidden>DEV {viewportProbe}</p>
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
    position: relative;
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
  .lbl {
    display: block;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 6px;
  }
  textarea {
    width: 100%;
    resize: none;
    padding: 12px;
    background: var(--color-bg-panel);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-family: var(--font-mono);
    font-size: var(--text-body);
    overflow-wrap: anywhere;
  }
  textarea:focus {
    outline: none;
    border-color: var(--color-border-strong);
  }
  textarea::placeholder {
    font-family: var(--font-sans);
    color: var(--color-text-muted);
  }
  .error {
    margin: 8px 0 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .unpair-shell {
    width: 100%;
    margin-top: 8px;
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
    font-weight: 500;
    background: transparent;
  }
  .unpair-shell.armed {
    background: var(--color-text-secondary);
    border-color: var(--color-text-secondary);
    color: var(--color-bg-base);
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
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }
</style>
