<script lang="ts">
  /*
   * Devices (desktop only) — GDK-1047, the UI half of phone pairing.
   *
   * Mint a pairing offer, show it as the same QR the terminal draws, and
   * revoke devices that are gone. The mint and revoke run through
   * /desktop/pairing — the desktop app's route over internal/pairflow —
   * so this tab and `gadak pairing` operate the identical token store.
   *
   * The offer is a credential, and this screen is credential UI: the QR is
   * the intended transport (scan with the phone), the offer STRING stays
   * masked behind an explicit reveal for the paste-into-terminal case, and
   * none of it is persisted — component state only, gone when the dialog
   * closes. The mask is not security theater around the QR: a QR and its
   * plaintext are the same bytes; the reveal exists so a shoulder-surfing
   * thumbnail or a screenshot of the tab does not carry the credential.
   *
   * Scope is deliberately two choices. A terminal token opens a shell on
   * this machine and is minted from the terminal, not from a form; the
   * server refuses it too (bad_scope), and this select never offers it.
   */
  import { onMount } from 'svelte'
  import { t, type MessageKey } from '../../lib/i18n'
  import { copyText } from '../../lib/copy-text'
  import LoadingState from '../ui/LoadingState.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { ADD_BTN, COPY_BTN, INPUT, SELECT, SELECT_CHEVRON } from './controls'

  interface DeviceRow {
    label: string
    scope: string
    expires_at: string
    hash_prefix: string
    state: string
  }

  /** The one place the freshly minted credential lives. Never persisted. */
  interface Minted {
    offer: string
    label: string
    endpoint: string
    expires_at: string
    hash_prefix: string
    loopback_warning: boolean
    qr_png: string
  }

  let devices = $state<DeviceRow[]>([])
  let advertised = $state('')
  /** not_configured | paired_away | null — why the mint form is disabled. */
  let unavailable = $state<string | null>(null)
  let loading = $state(true)
  /** Device list reads the local app over loopback; inside the grace this
   *  surface paints nothing (GDK-1481). */
  const loadingGrace = createSkeletonGrace(() => loading && devices.length === 0 && !unavailable)
  let loadError = $state<string | null>(null)

  let label = $state('')
  let scope = $state<'serve' | 'origin'>('serve')
  let endpoint = $state('')
  let ttl = $state('')
  let minting = $state(false)
  let mintError = $state<string | null>(null)
  let minted = $state<Minted | null>(null)
  let revealed = $state(false)
  let offerCopied = $state(false)

  /** Server scope values → row copy. local-routing is the _home row. */
  function scopeLabel(scope: string): string {
    if (scope === 'local-routing') return t('settings.devicesScopeLocalRouting')
    if (scope === 'origin') return t('settings.devicesScopeOrigin')
    return t('settings.devicesScopeServe')
  }

  /** active | expired | revoked <ts> → one word; the timestamp stays. */
  function stateLabel(state: string): string {
    if (state.startsWith('revoked')) return t('settings.devicesStateRevoked')
    if (state === 'expired') return t('settings.devicesStateExpired')
    return t('settings.devicesStateActive')
  }

  const MINT_ERROR: Record<string, MessageKey> = {
    label_required: 'settings.devicesErrLabelRequired',
    reserved_label: 'settings.devicesErrReservedLabel',
    bad_scope: 'settings.devicesErrBadScope',
    bad_endpoint: 'settings.devicesErrBadEndpoint',
    bad_ttl: 'settings.devicesErrBadTtl',
    no_serve: 'settings.devicesErrNoServe',
    label_exists: 'settings.devicesErrLabelExists',
    not_configured: 'settings.devicesUnavailableNotConfigured',
    paired_away: 'settings.devicesUnavailablePairedAway',
    mint_failed: 'settings.devicesErrFailed',
  }

  async function load(): Promise<void> {
    loading = true
    try {
      const res = await fetch('/desktop/pairing')
      if (!res.ok) throw new Error(String(res.status))
      const doc = (await res.json()) as {
        devices: DeviceRow[]
        advertised_endpoint?: string
        unavailable?: string
      }
      devices = doc.devices ?? []
      advertised = doc.advertised_endpoint ?? ''
      unavailable = doc.unavailable ?? null
      // Prefill once: the advertised endpoint is what the phone would
      // reach; the user edits it when a tailnet URL is the better answer.
      if (endpoint === '' && advertised !== '') endpoint = advertised
      loadError = null
    } catch {
      // The route lives on the desktop app's mux only. Reaching this in
      // the desktop app means the server is older or broken, not that the
      // device list is empty — so say "could not read", never draw zero
      // devices. Rows already on screen stay: the error is a banner.
      loadError = t('settings.devicesLoadFailed')
    }
    loading = false
  }

  onMount(load)

  async function mint(): Promise<void> {
    if (minting || unavailable !== null) return
    minting = true
    mintError = null
    try {
      const res = await fetch('/desktop/pairing/mint', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label: label.trim(), scope, ttl: ttl.trim(), endpoint: endpoint.trim() }),
      })
      if (!res.ok) {
        const doc = (await res.json().catch(() => ({}))) as { error?: string }
        mintError = t(MINT_ERROR[doc.error ?? ''] ?? 'settings.devicesErrFailed')
        return
      }
      minted = (await res.json()) as Minted
      revealed = false
      offerCopied = false
      // The new token is in the store now; the list should say so.
      await load()
    } catch {
      mintError = t('settings.devicesErrFailed')
    } finally {
      minting = false
    }
  }

  async function revoke(row: DeviceRow): Promise<void> {
    try {
      const res = await fetch('/desktop/pairing/revoke', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ selector: row.label }),
      })
      if (!res.ok) {
        const doc = (await res.json().catch(() => ({}))) as { error?: string }
        // not_found / already_revoked both mean "the list knows better" —
        // reload rather than argue with a stale row.
        if (doc.error !== 'not_found' && doc.error !== 'already_revoked') {
          mintError = t('settings.devicesErrFailed')
          return
        }
      }
      await load()
    } catch {
      mintError = t('settings.devicesErrFailed')
    }
  }

  async function copyOffer(): Promise<void> {
    if (!minted) return
    // copy-text.ts owns the desktop-vs-web transport (GDK-178); "copied"
    // shows only for a write that actually happened.
    if (await copyText(minted.offer)) {
      offerCopied = true
      setTimeout(() => {
        offerCopied = false
      }, 1500)
    }
  }
</script>

<div class="flex flex-col gap-2.5" data-testid="devices-tab">
  <p class="text-micro leading-relaxed text-text-muted">{t('settings.devicesIntro')}</p>

  {#if loadError}
    <div
      class="flex flex-wrap items-center gap-2 rounded border border-border-subtle bg-bg-elevated px-2 py-1.5"
      data-testid="devices-error"
    >
      <span class="min-w-0 flex-1 text-micro text-status-reopen">{loadError}</span>
      <button type="button" class={COPY_BTN} disabled={loading} onclick={() => void load()}>
        {t('settings.integrationRecheck')}
      </button>
    </div>
  {/if}

  {#if unavailable}
    <p
      class="rounded border border-border-subtle bg-bg-elevated px-2 py-1.5 text-micro leading-relaxed text-text-secondary"
      data-testid="devices-unavailable"
    >
      {t(unavailable === 'paired_away'
        ? 'settings.devicesUnavailablePairedAway'
        : 'settings.devicesUnavailableNotConfigured')}
    </p>
  {/if}

  {#if loading && devices.length === 0 && !unavailable}
    <div class="h-full" data-skeleton={loadingGrace.attr}>
      {#if loadingGrace.visible}<LoadingState label={t('settings.devicesLoading')} />{/if}
    </div>
  {:else if devices.length === 0}
    {#if !loadError && !unavailable}
      <p class="py-6 text-center text-text-muted">{t('settings.devicesEmpty')}</p>
    {/if}
  {:else}
    <table class="w-full text-micro" data-testid="devices-list">
      <thead>
        <tr class="text-left text-text-muted">
          <th class="py-1 pr-2 font-medium">{t('settings.devicesColLabel')}</th>
          <th class="py-1 pr-2 font-medium">{t('settings.devicesColScope')}</th>
          <th class="py-1 pr-2 font-medium">{t('settings.devicesColExpires')}</th>
          <th class="py-1 pr-2 font-medium">{t('settings.devicesColState')}</th>
          <th class="py-1 font-medium" aria-label={t('settings.devicesRevoke')}></th>
        </tr>
      </thead>
      <tbody>
        {#each devices as row (row.hash_prefix + row.label)}
          {@const isHome = row.scope === 'local-routing'}
          {@const revoked = row.state.startsWith('revoked')}
          <tr class="border-t border-border-subtle align-top" data-testid="devices-row-{row.label}">
            <td class="py-1.5 pr-2">
              <span class="font-mono text-text-primary">{row.label}</span>
              {#if isHome}
                <p class="mt-0.5 text-micro leading-relaxed text-text-muted">
                  {t('settings.devicesHomeRowHint')}
                </p>
              {/if}
            </td>
            <td class="py-1.5 pr-2 text-text-secondary">{scopeLabel(row.scope)}</td>
            <!-- nowrap: the vision pass caught every row's date wrapping
                 mid-value (2026-11- / 25) at the dialog's table width — a
                 credential table's expiry must read as one token. -->
            <td class="whitespace-nowrap py-1.5 pr-2 font-mono text-text-secondary">{row.expires_at}</td>
            <td class="py-1.5 pr-2 {revoked ? 'text-status-reopen' : 'text-text-secondary'}">
              {stateLabel(row.state)}
            </td>
            <td class="py-1.5 text-right">
              {#if !isHome}
                <button
                  type="button"
                  class={COPY_BTN}
                  disabled={revoked}
                  onclick={() => void revoke(row)}
                  data-testid="devices-revoke-{row.label}"
                >
                  {t('settings.devicesRevoke')}
                </button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#if minted}
    <!-- The mint result: QR first (the intended transport), offer string
         masked behind an explicit reveal, everything in component state —
         closing the dialog drops the credential from memory. -->
    <section
      class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
      data-testid="devices-minted"
    >
      <p class="text-micro leading-relaxed text-text-primary">
        {t('settings.devicesMinted', { label: minted.label, expires: minted.expires_at })}
      </p>
      {#if minted.loopback_warning}
        <p class="mt-1 text-micro leading-relaxed text-status-reopen" data-testid="devices-loopback-warning">
          {t('settings.devicesLoopbackWarning')}
        </p>
      {/if}
      <div class="mt-2 flex flex-col items-center gap-2">
        <img
          src={minted.qr_png}
          alt={t('settings.devicesQrAlt')}
          class="h-56 w-56 rounded border border-border-subtle bg-white p-1"
          data-testid="devices-qr"
        />
        <div
          class="flex w-full max-w-md items-center gap-1.5 rounded border border-border-subtle bg-bg-base px-2 py-1"
          data-testid="devices-offer"
        >
          <span class="sr-only">{t('settings.devicesOfferLabel')}</span>
          <code
            class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-micro text-text-secondary"
            >{revealed ? minted.offer : `${minted.offer.slice(0, 6)}…${minted.offer.slice(-4)}`}</code
          >
          <button
            type="button"
            class="{COPY_BTN} flex-none"
            onclick={() => {
              revealed = !revealed
              offerCopied = false
            }}
            aria-pressed={revealed}
            data-testid="devices-offer-reveal"
          >
            {t(revealed ? 'settings.devicesOfferHide' : 'settings.devicesOfferShow')}
          </button>
          <button
            type="button"
            class="{COPY_BTN} flex-none"
            onclick={() => void copyOffer()}
            aria-label={t('settings.devicesCopyOffer')}
            data-testid="devices-offer-copy"
          >
            {offerCopied ? t('settings.copied') : t('settings.copy')}
          </button>
        </div>
      </div>
    </section>
  {/if}

  {#if unavailable === null}
    <form
      class="mt-1 flex flex-wrap items-end gap-2"
      onsubmit={(e) => {
        e.preventDefault()
        void mint()
      }}
      data-testid="devices-form"
    >
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('settings.devicesLabelLabel')}</span>
        <input
          class={INPUT}
          bind:value={label}
          data-testid="devices-label-input"
          autocomplete="off"
          spellcheck="false"
        />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('settings.devicesScopeLabel')}</span>
        <select
          class="{SELECT} {SELECT_CHEVRON}"
          bind:value={scope}
          data-testid="devices-scope-select"
        >
          <option value="serve">{t('settings.devicesScopeServe')}</option>
          <option value="origin">{t('settings.devicesScopeOrigin')}</option>
        </select>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('settings.devicesEndpointLabel')}</span>
        <input
          class={INPUT}
          bind:value={endpoint}
          placeholder="http://127.0.0.1:7877"
          data-testid="devices-endpoint-input"
          autocomplete="off"
          spellcheck="false"
        />
        <span class="text-micro text-text-muted">{t('settings.devicesEndpointHint')}</span>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('settings.devicesTtlLabel')}</span>
        <input
          class={INPUT}
          bind:value={ttl}
          placeholder="90d"
          data-testid="devices-ttl-input"
          autocomplete="off"
          spellcheck="false"
        />
      </label>
      <button
        type="submit"
        class={ADD_BTN}
        disabled={minting}
        data-testid="devices-mint-button"
      >
        {t(minting ? 'settings.devicesMintBusy' : 'settings.devicesMint')}
      </button>
    </form>
  {/if}

  {#if mintError}
    <p class="text-micro text-status-reopen" data-testid="devices-mint-error">{mintError}</p>
  {/if}
</div>
