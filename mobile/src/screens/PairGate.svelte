<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { t } from '../lib/i18n'
  import { app, enterDemo, pair } from '../lib/store.svelte'
  import { decodeOffer, OfferError, OfferScopeError } from '../lib/offer'
  import { errorMessage } from '../lib/api'

  // The unpaired app IS this screen — pairing is the front door, not a
  // setting. Paste-first: the dev webview has no camera, and a one-line
  // offer is exactly what `gadak pairing mint` prints. QR rides the
  // packaged build only.
  const IS_DEV = import.meta.env.DEV

  let offerLine = $state('')
  let busy = $state(false)
  let error = $state<string | null>(null)

  // Friendly copy per decoder refusal — the decoder's own messages are for
  // tests and logs-with-no-token, not for the screen. Same app.hosts.*
  // keys the PairingTab roster flow reads (GDK-1704).
  function offerCopy(e: OfferError): string {
    const m = e.message
    if (m.includes('empty')) return t('app.hosts.errEmpty')
    if (m.includes('version')) return t('app.hosts.errVersion')
    return t('app.hosts.errBad')
  }

  async function submit() {
    if (busy) return
    error = null
    busy = true
    try {
      const offer = decodeOffer(offerLine)
      await pair(offer)
    } catch (err) {
      // OfferScopeError: decoded fine, carries no mirror token — its
      // message is the user's sentence (authored in lib/offer.ts).
      error =
        err instanceof OfferScopeError
          ? err.message
          : err instanceof OfferError
            ? offerCopy(err)
            : errorMessage(err)
    } finally {
      busy = false
    }
  }

  // One-thumb path: `gadak pairing mint | pbcopy` on the desktop, one tap
  // here. clipboard.readText shows the system paste pill — the OS asks the
  // user, not us — and the long-press paste dance disappears.
  async function pasteAndPair() {
    error = null
    try {
      const text = (await navigator.clipboard.readText()).trim()
      if (text === '') {
        error = t('app.hosts.errClipboardEmpty')
        return
      }
      offerLine = text
    } catch {
      error = t('app.hosts.errClipboardFail')
      return
    }
    await submit()
  }

  // Dev diagnostic: whether the shell's Keychain answers at all. Presence
  // only — the value never reaches the DOM.
  let devProbe = $state('')
  if (IS_DEV) {
    void (async () => {
      const hasTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
      if (!hasTauri) {
        devProbe = 'dev: no tauri bridge'
        return
      }
      try {
        const { invoke } = await import('@tauri-apps/api/core')
        const t = await invoke<string | null>('token_get')
        devProbe = `dev: keychain token ${t ? 'present' : 'absent'}`
      } catch (e) {
        devProbe = `dev: token_get failed: ${String(e).slice(0, 120)}`
      }
    })()
  }

  async function scan() {
    error = null
    try {
      const { scan: scanQR, Format, cancel } = await import('@tauri-apps/plugin-barcode-scanner')
      const result = await scanQR({ windowed: false, formats: [Format.QRCode] })
      void cancel
      if (result?.content) {
        offerLine = result.content
        await submit()
      }
    } catch {
      error = t('app.hosts.errCamera')
    }
  }
</script>

<Screen>
  <div class="safe-top safe-bottom frame">
    <div class="gate">
    <div class="brand">
      <h1 class="type-subject">gadak</h1>
      <p class="tag">{t('app.gate.tagline')}</p>
    </div>

    {#if app.rejected}
      <p class="rejected">{t('app.gate.rejected')}</p>
    {/if}

    <label class="lbl" for="offer">{t('app.hosts.offerLabel')}</label>
    <textarea
      id="offer"
      bind:value={offerLine}
      rows="4"
      placeholder={t('app.hosts.offerPlaceholder')}
      autocapitalize="off"
      spellcheck="false"
    ></textarea>
    <p class="hint">
      {t('app.gate.desktopLead')} <span class="cmd">gadak pairing mint</span>
      {t('app.gate.desktopTail')} {t('app.gate.offerSecret')}
    </p>

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    {#if offerLine.trim() === ''}
      <button class="pair" disabled={busy} onclick={() => void pasteAndPair()}>
        {busy ? t('app.hosts.checking') : t('app.hosts.pastePair')}
      </button>
    {:else}
      <button class="pair" disabled={busy} onclick={() => void submit()}>
        {busy ? t('app.hosts.checking') : t('app.hosts.pair')}
      </button>
    {/if}
    {#if !IS_DEV}
      <button class="qr" onclick={() => void scan()}>{t('app.hosts.scan')}</button>
    {/if}
    <!-- Third door (GDK-1051): the bundled read-only sample workspace, no
         pairing at all. -->
    <button class="demo" disabled={busy} onclick={() => void enterDemo()}>
      {t('app.demoEnter')}
    </button>
    {#if devProbe}
      <p class="hint">{devProbe}</p>
    {/if}
    </div>
  </div>
</Screen>

<style>
  .frame {
    min-height: 100%;
    display: flex;
    flex-direction: column;
  }
  .gate {
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: 24px;
  }
  .brand {
    margin-bottom: 32px;
  }
  h1 {
    margin: 0;
    font-size: 40px;
    line-height: 1.1;
  }
  .tag {
    margin: 6px 0 0;
    color: var(--color-text-muted);
  }
  .rejected {
    margin: 0 0 16px;
    padding: 12px;
    border-radius: 6px;
    background: var(--color-lozenge-red);
    color: var(--color-text-primary);
    font-size: var(--text-micro);
  }
  .lbl {
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 6px;
  }
  textarea {
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
  .hint {
    margin: 8px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .cmd {
    font-family: var(--font-mono);
  }
  .error {
    margin: 12px 0 0;
    font-size: var(--text-micro);
    color: var(--color-status-reopen);
  }
  .pair {
    margin-top: 20px;
    min-height: var(--spacing-control);
    border-radius: 6px;
    background: var(--color-accent);
    color: var(--color-bg-base);
    font-weight: 600;
  }
  .pair:disabled {
    opacity: 0.45;
  }
  .qr,
  .demo {
    margin-top: 8px;
    min-height: var(--spacing-control);
    color: var(--color-accent-text);
  }
</style>
