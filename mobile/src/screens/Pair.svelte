<script lang="ts">
  /*
   * Pair screen — scan first, paste as the fallback (ux-report Q6). A
   * decode previews endpoint/label/expiry; an expired offer cannot be
   * saved; and saving proves the connection with one bootstrap. Success
   * is "paired"; failure keeps the saved pairing and says so as its own
   * state — never a silent rollback (Q6③). The token is never displayed:
   * the preview shows endpoint, label, and expiry only (offer.go
   * credential rule), and no error path quotes it.
   */
  import { Format, scan } from '@tauri-apps/plugin-barcode-scanner'
  import { ApiError, bootstrap, type ApiContext } from '../lib/api'
  import {
    connectFailure,
    connectFailureMessage,
    decodeOffer,
    isExpired,
    offerExpiry,
    offerFromLine,
    OfferError,
    type ConnectFailure,
    type Offer,
  } from '../lib/offer'
  import { clearPairing, readPairing, readQueueCache, savePairing } from '../lib/settings'
  import { detectLocale, t } from '../lib/i18n'

  // onsaved is App.svelte's tab-switch (A-nav renames it); onpaired is
  // this chunk's name for the same moment. Both fire only on a proven
  // pair — a saved-but-unconnected pairing keeps the user here to read why.
  // onunpaired fires after clearPairing() so the app can stop what only a
  // pairing may run (the feed poll — A-nav).
  let {
    onpaired,
    onsaved,
    onunpaired,
  } = $props<{ onpaired?: () => void; onsaved?: () => void; onunpaired?: () => void }>()

  // 16KiB paste cap: the decoder handles more (tested), but the paste
  // surface bounds what a hostile paste can make the UI carry around.
  const MAX_PASTE = 16 * 1024
  // The proof's door — orca's 25s (ux-report Q6): a half-dead Tailscale
  // must surface as a timeout, not hide behind a spinner.
  const CONNECT_TIMEOUT_MS = 25_000
  // DEV runs in a browser with no camera behind the plugin — paste stays
  // the primary surface there, exactly as before the scan round.
  const canScan = !import.meta.env.DEV

  let paste = $state('')
  let decoded: Offer | null = $state(null)
  let error: string | null = $state(null)
  let errorReason: string | null = $state(null)
  let saved = $state(false)
  let current = $state(readPairing())
  // Proof in flight / its failure verdict when the save stood but the
  // bootstrap did not answer.
  let proving = $state(false)
  let failedConnect: ConnectFailure | null = $state(null)
  let failedConnectRaw: string | null = $state(null)
  // Paste form: primary in DEV, one tap away ("Have a code instead?")
  // beside the scan button everywhere else.
  let pasteOpen = $state(!canScan)
  let scanNote: string | null = $state(null)
  let lastConnectedAt: string | null = $state(initialLastConnected())

  let savable = $derived(decoded !== null && !isExpired(decoded) && !proving)

  function initialLastConnected(): string | null {
    // The queue cache's syncedAt is the app's last successful connection;
    // it only counts when it is newer than this pairing was saved —
    // otherwise it dates the previous home (or a re-pair), and showing it
    // would claim a connection that never happened.
    const pairing = readPairing()
    const cache = readQueueCache()
    if (!pairing || pairing.savedAt === '' || !cache || cache.syncedAt === '') return null
    return cache.syncedAt > pairing.savedAt ? cache.syncedAt : null
  }

  /** One reader for both surfaces: the scanned line and the pasted line. */
  function ingest(line: string): void {
    saved = false
    failedConnect = null
    failedConnectRaw = null
    error = null
    errorReason = null
    try {
      decoded = decodeOffer(offerFromLine(line))
    } catch (err) {
      decoded = null
      if (err instanceof OfferError) {
        error = t('pair.error.invalid')
        // Reason text never contains the payload (tested), but it is the
        // decoder phrasing — shown small, not as the headline.
        errorReason = err.message
      } else {
        throw err
      }
    }
  }

  function ondecode(): void {
    ingest(paste)
  }

  async function onscan(): Promise<void> {
    scanNote = null
    let content: string
    try {
      const result = await scan({
        cameraDirection: 'back',
        formats: [Format.QRCode],
        windowed: false,
      })
      content = result.content
    } catch {
      // Denied, no camera, or the user closed the scanner — the paste
      // path is one tap away and that is where this lands.
      pasteOpen = true
      scanNote = t('pair.scan.unavailable')
      return
    }
    if (content.trim() === '') return
    ingest(content)
  }

  async function onsave(): Promise<void> {
    if (!decoded || proving) return
    // The expired gate lives here too, not only in the disabled button:
    // the button and the handler must agree (ux-report Q6② gap).
    if (isExpired(decoded)) return
    error = null
    errorReason = null
    failedConnect = null
    failedConnectRaw = null
    proving = true
    try {
      try {
        await savePairing({ endpoint: decoded.endpoint, token: decoded.token, label: decoded.label })
      } catch {
        // The secure-store write failed (settings.ts rejects) — the
        // pairing is NOT saved even though the meta half may be; tell the
        // user instead of claiming success.
        saved = false
        error = t('pair.error.save')
        return
      }
      current = readPairing()
      // The proof: one bootstrap with the just-saved credentials. 304
      // cannot happen (no etag sent) but would answer the same — the
      // gate spoke to us.
      const ctx: ApiContext = { endpoint: decoded.endpoint, token: decoded.token }
      try {
        await bootstrap(ctx, null, AbortSignal.timeout(CONNECT_TIMEOUT_MS))
      } catch (err) {
        // Saved stands; only the connection failed. Distinct state, its
        // own message per verdict (Q6④), callbacks stay quiet.
        failedConnect = connectFailure(err)
        failedConnectRaw =
          failedConnect.kind === 'http' && err instanceof ApiError ? err.message : null
        return
      }
      saved = true
      lastConnectedAt = new Date().toISOString()
      onpaired?.()
      onsaved?.()
    } finally {
      proving = false
    }
  }

  async function onunpair(): Promise<void> {
    await clearPairing()
    current = null
    saved = false
    failedConnect = null
    failedConnectRaw = null
    lastConnectedAt = null
    onunpaired?.()
  }

  function expiresLabel(o: Offer): string {
    const d = offerExpiry(o)
    if (d === null) return t('pair.preview.noExpiry')
    return d.toLocaleDateString()
  }

  /*
   * Compact relative time — platform copy (Intl.RelativeTimeFormat,
   * numeric auto), same ladder granularity as the web catalog's compact
   * relativeTime (minute→year, "just now" under a minute). Component-
   * local like Queue's timeLabel; a shared mobile util is a later
   * refactor's call, not this chunk's file list.
   */
  function relativeConnected(iso: string): string {
    const then = new Date(iso).getTime()
    if (Number.isNaN(then)) return ''
    const s = Math.floor((Date.now() - then) / 1000)
    if (s < 60) return t('pair.current.justNow')
    const rtf = new Intl.RelativeTimeFormat(detectLocale(), { numeric: 'auto' })
    const m = Math.floor(s / 60)
    if (m < 60) return rtf.format(-m, 'minute')
    const h = Math.floor(m / 60)
    if (h < 24) return rtf.format(-h, 'hour')
    const d = Math.floor(h / 24)
    if (d < 7) return rtf.format(-d, 'day')
    if (d < 30) return rtf.format(-Math.floor(d / 7), 'week')
    if (d < 365) return rtf.format(-Math.floor(d / 30), 'month')
    return rtf.format(-Math.floor(d / 365), 'year')
  }

  // Editing the paste text invalidates the previous decode — as an
  // explicit input handler, not an $effect: a rune effect that reads the
  // very state it clears (decoded/error) re-runs when ingest() sets them
  // and wipes the preview it was supposed to keep (Svelte 5 tracks every
  // read in the effect body).
  function onpasteinput(): void {
    if (decoded !== null || error !== null) {
      decoded = null
      error = null
      errorReason = null
      saved = false
      failedConnect = null
      failedConnectRaw = null
    }
  }
</script>

<section class="m-main scroll-region" data-testid="pair-screen">
  <h1 class="type-subject m-queue-title">{t('pair.title')}</h1>

  {#if current}
    <div class="m-pair-current" data-testid="pair-current">
      <p>{current.label ? t('pair.current', { label: current.label }) : t('pair.currentFallback')}</p>
      <p class="m-pair-endpoint">{current.endpoint}</p>
      {#if lastConnectedAt}
        <p class="m-pair-lastconn" data-testid="pair-last-connected">
          {t('pair.current.lastConnected', { time: relativeConnected(lastConnectedAt) })}
        </p>
      {/if}
      <button class="m-button m-button-quiet" type="button" onclick={onunpair}>{t('pair.unpair')}</button>
    </div>
  {/if}

  {#if canScan}
    <div class="m-pair-scan">
      <button
        class="m-button m-scan-button"
        type="button"
        onclick={() => void onscan()}
        disabled={proving}
        data-testid="pair-scan"
      >
        {t('pair.scan')}
      </button>
      {#if scanNote}
        <p class="m-scan-note" role="note" data-testid="pair-scan-note">{scanNote}</p>
      {/if}
      {#if !pasteOpen}
        <button class="m-linkbutton" type="button" onclick={() => (pasteOpen = true)} data-testid="pair-paste-instead">
          {t('pair.pasteInstead')}
        </button>
      {/if}
    </div>
  {/if}

  {#if pasteOpen}
    <form
      class="m-pair-form"
      onsubmit={(e) => {
        e.preventDefault()
        ondecode()
      }}
    >
      <label class="m-label" for="pair-paste">{t('pair.paste.label')}</label>
      <textarea
        id="pair-paste"
        class="m-textarea"
        rows="4"
        spellcheck="false"
        autocomplete="off"
        placeholder={t('pair.paste.placeholder')}
        maxlength={MAX_PASTE}
        bind:value={paste}
        oninput={onpasteinput}
        data-testid="pair-paste"
      ></textarea>
      <button class="m-button" type="submit" disabled={paste.trim() === ''} data-testid="pair-decode">
        {t('pair.decode')}
      </button>
    </form>
  {/if}

  {#if error}
    <p class="m-error" role="alert" data-testid="pair-error">
      {error}
      {#if errorReason}<span class="m-error-reason">{errorReason}</span>{/if}
    </p>
  {/if}

  {#if decoded}
    <div class="m-pair-preview" data-testid="pair-preview">
      <dl>
        <div>
          <dt>{t('pair.preview.endpoint')}</dt>
          <dd>{decoded.endpoint}</dd>
        </div>
        <div>
          <dt>{t('pair.preview.label')}</dt>
          <dd>{decoded.label || '—'}</dd>
        </div>
        <div>
          <dt>{t('pair.preview.expires')}</dt>
          <dd>
            {expiresLabel(decoded)}
            {#if isExpired(decoded)}
              <span class="m-expired">{t('pair.preview.expired')}</span>
            {/if}
          </dd>
        </div>
      </dl>
      {#if isExpired(decoded)}
        <p class="m-error" role="alert" data-testid="pair-expired-note">{t('pair.preview.expiredGuidance')}</p>
      {/if}
      <button class="m-button" type="button" onclick={onsave} disabled={!savable} data-testid="pair-save">
        {proving ? t('pair.connect.checking') : t('pair.save')}
      </button>
      {#if saved}
        <p class="m-saved" data-testid="pair-saved">{t('pair.saved')}</p>
      {/if}
      {#if failedConnect}
        <div class="m-connect-failed" role="alert" data-testid="pair-connect-failed">
          <p class="m-connect-headline">{t('pair.savedNotConnected')}</p>
          <p>{t(connectFailureMessage(failedConnect))}</p>
          {#if failedConnectRaw}<span class="m-error-reason">{failedConnectRaw}</span>{/if}
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .m-pair-scan {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin: 0.75rem 0 0;
  }

  /* The scan button is this chunk's primary action, so it gets the iOS
     44px minimum on top of m-button (app.css owns --spacing-control at
     32px and is not this chunk's file); min-height, not height, so
     Dynamic Type grows it instead of clipping. */
  .m-scan-button {
    min-height: 44px;
    height: auto;
  }

  /* The "Have a code instead?" reveal — quiet, but a real button: 44px
     minimum per the chunk's iOS contract, min-height so Dynamic Type can
     grow it (no fixed heights on touch targets this chunk adds). */
  .m-linkbutton {
    min-height: 44px;
    padding: 0 0.5rem;
    border: 0;
    background: none;
    color: var(--color-accent);
    font-family: var(--font-sans);
    font-size: var(--text-body);
    align-self: flex-start;
    cursor: pointer;
  }

  .m-scan-note {
    margin: 0;
    color: var(--color-text-muted);
    font-size: var(--text-micro);
  }

  .m-pair-lastconn {
    color: var(--color-text-secondary);
    font-size: var(--text-micro);
  }

  .m-connect-failed {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin: 0.5rem 0 0;
  }

  .m-connect-failed p {
    margin: 0;
  }

  .m-connect-headline {
    color: var(--color-status-reopen);
    font-size: var(--text-body);
  }
</style>
