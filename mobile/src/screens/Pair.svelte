<script lang="ts">
  /*
   * Pair screen — skeleton (1차). Paste the one-line offer, decode, save.
   * QR scan and Keychain storage are later rounds; both are recorded in the
   * plugin gap map. The token is never displayed: the preview shows
   * endpoint, label, and expiry only (offer.go credential rule).
   */
  import { decodeOffer, isExpired, offerExpiry, OfferError, type Offer } from '../lib/offer'
  import { clearPairing, readPairing, savePairing } from '../lib/settings'
  import { t } from '../lib/i18n'

  let { onsaved } = $props<{ onsaved?: () => void }>()

  // 16KiB paste cap: the decoder handles more (tested), but the paste
  // surface bounds what a hostile paste can make the UI carry around.
  const MAX_PASTE = 16 * 1024

  let paste = $state('')
  let decoded: Offer | null = $state(null)
  let error: string | null = $state(null)
  let errorReason: string | null = $state(null)
  let saved = $state(false)
  let current = $state(readPairing())

  function ondecode(): void {
    saved = false
    error = null
    errorReason = null
    try {
      decoded = decodeOffer(paste)
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

  function onsave(): void {
    if (!decoded) return
    savePairing({ endpoint: decoded.endpoint, token: decoded.token, label: decoded.label })
    current = readPairing()
    saved = true
    onsaved?.()
  }

  function onunpair(): void {
    clearPairing()
    current = null
    saved = false
  }

  function expiresLabel(o: Offer): string {
    const d = offerExpiry(o)
    if (d === null) return t('pair.preview.noExpiry')
    return d.toLocaleDateString()
  }

  $effect(() => {
    // New paste invalidates the previous decode.
    void paste
    if (decoded !== null || error !== null) {
      decoded = null
      error = null
      errorReason = null
      saved = false
    }
  })
</script>

<section class="m-main scroll-region" data-testid="pair-screen">
  <h1 class="type-subject m-queue-title">{t('pair.title')}</h1>

  {#if current}
    <div class="m-pair-current" data-testid="pair-current">
      <p>{current.label ? t('pair.current', { label: current.label }) : t('pair.currentFallback')}</p>
      <p class="m-pair-endpoint">{current.endpoint}</p>
      <button class="m-button m-button-quiet" type="button" onclick={onunpair}>{t('pair.unpair')}</button>
    </div>
  {/if}

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
      data-testid="pair-paste"
    ></textarea>
    <button class="m-button" type="submit" disabled={paste.trim() === ''} data-testid="pair-decode">
      {t('pair.decode')}
    </button>
  </form>

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
      <button class="m-button" type="button" onclick={onsave} data-testid="pair-save">{t('pair.save')}</button>
      {#if saved}
        <p class="m-saved" data-testid="pair-saved">{t('pair.saved')}</p>
      {/if}
    </div>
  {/if}
</section>
