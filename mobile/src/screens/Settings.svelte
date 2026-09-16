<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import { t } from '../lib/i18n'
  import {
    app,
    sync,
    unpair,
    pair,
    pairTerminal,
    unpairTerminal,
    closeSettings,
    exitDemo,
    switchHost,
    removeRosterHost,
    setTerminalFontSize,
  } from '../lib/store.svelte'
  import { TERMINAL_FONT_SIZES } from '../lib/termprefs'
  import { relTime, hasIdentity, offerExpiry } from '../lib/domain'
  import { decodeOffer, OfferError, OfferScopeError } from '../lib/offer'
  import { ApiError, errorMessage, request } from '../lib/api'
  import { runtimeMode } from '../lib/runtime'
  import { getActiveHostId, listHosts, type KnownHost } from '../lib/hosts'
  import type { ViewerDoc } from '../lib/types'
  // The one version string this screen may print. tauri.conf.json owns the
  // version (the TestFlight script reads and bumps it there), so the footer
  // cannot drift from the shipping build again (GDK-1788). A JSON import, not
  // a runtime Tauri call, so the line renders identically in dev and preview.
  import tauriConf from '../../src-tauri/tauri.conf.json'

  // Rarely visited, always honest: what am I paired to, how fresh is the
  // snapshot, who does the serve think I am. The one destructive rarity —
  // Unpair — uses the house two-step arm (UX_PRINCIPLES §7): first tap
  // arms, second tap within 3s fires, no modal.
  //
  // Home-screen install hint (GDK-1970): a standalone web app has no
  // browser chrome and none of the edge-swipe history navigation that can
  // walk a hosted page off the tailnet. Read once — display-mode changes
  // with how the page was launched, not while it runs.
  const standalone =
    typeof matchMedia !== 'undefined' && matchMedia('(display-mode: standalone)').matches
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

  /* ── hosted connection (GDK-1966) ──
     Who the serve says is reading, when the page arrived through
     tailscale serve. A missing route (a serve older than viewer/) or a
     failed probe reads exactly like a direct connection: the no-viewer
     sentence, never error chrome on an optional fact. */
  let viewer = $state<ViewerDoc | null>(null)
  $effect(() => {
    if (!app.hosted) return
    let alive = true
    void request<ViewerDoc>('viewer/')
      .then((res) => {
        if (alive && res.body) viewer = res.body
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  })

  /* ── host roster (GDK-1097 B2) ──
     Every host this phone paired with, one active. A row tap switches;
     the active row is a no-op. Forgetting an inactive row is the same
     two-step arm as Unpair (UX_PRINCIPLES §7). The list is re-read after
     each action below — roster rows change only through this screen.
     Hosted (GDK-1966) never reads it — the read half of "never touch
     pairing storage in hosted mode"; the section is hidden anyway. */
  let roster = $state<KnownHost[]>(app.hosted ? [] : listHosts())
  let activeId = $state<string | null>(app.hosted ? null : getActiveHostId())
  let repairHintId = $state<string | null>(null)
  let removeArmedId = $state<string | null>(null)
  let removeArmTimer: ReturnType<typeof setTimeout> | null = null

  function refreshRoster(): void {
    roster = listHosts()
    activeId = getActiveHostId()
  }

  async function onSwitchHost(id: string): Promise<void> {
    if (id === activeId) return
    repairHintId = null
    const ok = await switchHost(id)
    if (!ok) {
      // Token or meta is gone for that host — nothing moved. The row below
      // gets the re-pair hint; the pairing form pairs it again.
      repairHintId = id
      return
    }
    refreshRoster()
  }

  function onRemoveHost(id: string): void {
    if (removeArmedId !== id) {
      removeArmedId = id
      if (removeArmTimer) clearTimeout(removeArmTimer)
      removeArmTimer = setTimeout(() => (removeArmedId = null), 3000)
      return
    }
    if (removeArmTimer) clearTimeout(removeArmTimer)
    removeArmedId = null
    repairHintId = null
    void removeRosterHost(id).then(refreshRoster)
  }

  // Pairing another host rides the same offer flow as PairGate: decode,
  // probe, and pair() — which upserts the roster and makes the newcomer
  // active (B1 contract). Copy is catalog-backed here.
  let addHostOpen = $state(false)
  let addOffer = $state('')
  let addBusy = $state(false)
  let addError = $state<string | null>(null)

  function addOfferCopy(e: OfferError): string {
    const m = e.message
    if (m.includes('empty')) return t('app.hosts.errEmpty')
    if (m.includes('version')) return t('app.hosts.errVersion')
    return t('app.hosts.errBad')
  }

  async function submitAddHost() {
    if (addBusy) return
    addError = null
    addBusy = true
    try {
      const offer = decodeOffer(addOffer)
      await pair(offer)
      addOffer = ''
      addHostOpen = false
      refreshRoster()
    } catch (err) {
      // OfferScopeError: decoded fine, carries no mirror token — its
      // message is the catalog's sentence (app.offer*, GDK-1150).
      addError =
        err instanceof OfferScopeError
          ? err.message
          : err instanceof OfferError
            ? addOfferCopy(err)
            : errorMessage(err)
    } finally {
      addBusy = false
    }
  }

  async function pasteAndAddHost() {
    addError = null
    try {
      const text = (await navigator.clipboard.readText()).trim()
      if (text === '') {
        addError = t('app.hosts.errClipboardEmpty')
        return
      }
      addOffer = text
    } catch {
      addError = t('app.hosts.errClipboardFail')
      return
    }
    await submitAddHost()
  }

  // Same scan block as PairGate.svelte — do not re-derive the plugin call.
  // Hosted is unreachable at the entry itself (GDK-1966): hidden button,
  // unimported camera plugin.
  async function scanAddHost() {
    if (runtimeMode() !== 'tauri') return
    addError = null
    try {
      const { scan: scanQR, Format, cancel } = await import('@tauri-apps/plugin-barcode-scanner')
      const result = await scanQR({ windowed: false, formats: [Format.QRCode] })
      void cancel
      if (result?.content) {
        addOffer = result.content
        await submitAddHost()
      }
    } catch {
      addError = t('app.hosts.errCamera')
    }
  }

  // Friendly copy per decoder refusal — same mapping (and same keys) as
  // the roster flow's addOfferCopy above and PairGate.svelte (GDK-1704).
  function offerCopy(e: OfferError): string {
    const m = e.message
    if (m.includes('empty')) return t('app.hosts.errEmpty')
    if (m.includes('version')) return t('app.hosts.errVersion')
    return t('app.hosts.errBad')
  }

  function terminalProbeCopy(err: unknown): string {
    if (err instanceof OfferError) return offerCopy(err)
    if (err instanceof ApiError && err.code === 'scope_rejected') {
      // A serve QR scanned into the terminal slot. Distinct from expired.
      return t('settings.termOfferWrongScope')
    }
    if (err instanceof ApiError && err.code === 'pairing_rejected') {
      return t('settings.termOfferExpired')
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
        termError = t('app.hosts.errClipboardEmpty')
        return
      }
      termOffer = text
    } catch {
      termError = t('app.hosts.errClipboardFail')
      return
    }
    await submitTerminal()
  }

  // Same scan block as PairGate.svelte — do not re-derive the plugin call.
  // Same hosted unreachability as scanAddHost above (GDK-1966).
  async function scanTerminal() {
    if (runtimeMode() !== 'tauri') return
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
      termError = t('app.hosts.errCamera')
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
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <!-- Settings is a push layer now, not a tab (GDK-902, DESIGN.md §2):
           every screen has an explicit way out, and for a layer that way is
           a back control in its own header — the same one Detail wears, in
           the same corner, from the same catalog word. -->
      <button class="back" onclick={closeSettings} aria-label={t('app.back')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M15 18l-6-6 6-6" />
        </svg>
        <span>{t('app.back')}</span>
      </button>
      <h1 class="type-subject">{t('settings.title')}</h1>
    </div>
  {/snippet}

  <div class="page">
    {#if app.demo}
      <!-- Demo session (GDK-1051): the honest version of "Paired server" —
           nothing is paired. Exit is not the two-step unpair: it deletes
           nothing and is one tap back to the gate. -->
      <section>
        <h3>{t('app.demoWorkspace')}</h3>
        <p class="big">{t('app.demoMode')}</p>
        <p class="sub">{t('app.demoPairedNote')}</p>
      </section>
      <section>
        <button class="unpair" onclick={exitDemo}>{t('app.demoExit')}</button>
      </section>
    {:else if app.meta}
      {#if app.hosted}
        <!-- The hosted connection (GDK-1966): this page IS the serve's own
             bundle, opened in the browser over the tailnet — no roster, no
             offers, nothing to unpair. The question the roster answers
             ("what am I paired to?") is answered by the address bar. -->
        <section data-testid="hosted-connection">
          <h3>{t('settings.hostedTitle')}</h3>
          <p class="big mono">{location.host}</p>
          {#if viewer?.source === 'tailscale'}
            <p class="sub">
              {t('settings.hostedViewer', {
                name: viewer.name || viewer.login || '',
                login: viewer.login || '',
              })}
            </p>
          {:else}
            <p class="sub">{t('settings.hostedNoViewer')}</p>
          {/if}
          {#if !standalone}
            <p class="sub" data-testid="hosted-add-home">{t('settings.hostedAddHome')}</p>
          {/if}
        </section>
      {/if}
      {#if roster.length === 0 && !app.hosted}
        <!-- No roster yet (a pre-GDK-1097 pairing): the paired server is
             its own block. With a roster the active row *is* this block,
             and the screen said "This Mac (dev)" twice in a row (review
             2026-09-14, capture 13). -->
        <section>
          <h3>{t('app.pairedServer')}</h3>
          <p class="big">{app.meta.label || host(app.meta.endpoint)}</p>
          <p class="sub mono">{host(app.meta.endpoint)}</p>
        </section>
      {/if}

      <!-- The two pairings lead, adjacent (DESIGN.md §2: "pairing ·
           hosts · terminal"). One screen answers "is this thing still
           connected" for the mirror and for the shell, so the question
           is asked once at the top and the machinery — the roster, the
           cache, who the serve thinks I am — follows it. Each pairing
           keeps the heading the catalog already gave it (§3.6); no word
           was authored to cover both. -->
      <section>
        <h3>{t('sidebar.terminal')}</h3>
        {#if app.terminal}
          <p class="big">{app.terminal.label || host(app.terminal.endpoint)}</p>
          <!-- Hosted (GDK-1966): the label above already IS the page host —
               the RAM offer is labelled by location.host. -->
          {#if !app.hosted}
            <p class="sub mono">{host(app.terminal.endpoint)}</p>
          {/if}
          <!-- The first real terminal option (GDK-901): the grid's size.
               Four buttons, the numbers themselves in the mono face — a
               number is not copy (§3.6). The label is the desk's catalog
               key; nothing here is phone-authored prose. -->
          <!-- A row label, not a section heading: `.lbl` is the uppercase
               header register, and under it the unpair button read as a
               member of a "FONT SIZE" section (vision verdict, GDK-901
               2026-09-15). Sentence case, muted, and 12px off the host line
               so it is an option row, not a third line of the shell's identity. -->
          <p class="sub opt">{t('settings.terminalFontSize')}</p>
          <div
            class="sizes"
            role="radiogroup"
            aria-label={t('settings.terminalFontSize')}
            data-testid="terminal-font-size"
          >
            {#each TERMINAL_FONT_SIZES as px (px)}
              <button
                class="size"
                class:on={app.terminalFontSize === px}
                role="radio"
                aria-checked={app.terminalFontSize === px}
                onclick={() => setTerminalFontSize(px)}>{px}</button
              >
            {/each}
          </div>
          <!-- Hosted (GDK-1966): the offer is RAM-only — nothing to unpair. -->
          {#if !app.hosted}
            <button class="unpair-shell" class:armed={termArmed} onclick={onUnpairTerminal}>
              {termArmed ? t('app.unpairConfirm') : t('app.unpairShell')}
            </button>
          {/if}
        {:else}
          <label class="lbl" for="term-offer">{t('app.terminalOffer')}</label>
          <textarea
            id="term-offer"
            bind:value={termOffer}
            rows="3"
            placeholder={t('app.terminalOfferPlaceholder')}
            autocapitalize="off"
            spellcheck="false"
          ></textarea>
          <p class="sub">
            {t('app.gate.desktopLead')}
            <span class="mono">gadak pairing mint --scope terminal</span>
            {t('app.gate.desktopTail')}
          </p>
          {#if termError}
            <p class="error" role="alert">{termError}</p>
          {/if}
          {#if termOffer.trim() === ''}
            <button class="act" disabled={termBusy} onclick={() => void pasteAndPairTerminal()}>
              {termBusy ? t('app.hosts.checking') : t('app.hosts.pastePair')}
            </button>
          {:else}
            <button class="act" disabled={termBusy} onclick={() => void submitTerminal()}>
              {termBusy ? t('app.hosts.checking') : t('app.hosts.pair')}
            </button>
          {/if}
          {#if !DEV}
            <button class="act" onclick={() => void scanTerminal()}>{t('app.scan')}</button>
          {/if}
        {/if}
      </section>

      <!-- Host roster (GDK-1097 B2): switch with a tap, forget an inactive
           row through the two-step arm. Hidden on a hosted page (GDK-1966):
           there is no roster to switch and no offer to add with. -->
      {#if !app.hosted}
      <section>
        <h3>{t('app.hosts.title')}</h3>
        {#if offerExpiry(app.meta.expires_at)}
          <p class="sub">{t('app.offerExpires', { when: offerExpiry(app.meta.expires_at) })}</p>
        {/if}
        {#each roster as h (h.id)}
          <div class="hostrow">
            <button class="host" onclick={() => void onSwitchHost(h.id)}>
              <span class="hostline">
                <span class="hostlabel">{h.label || host(h.endpoint)}</span>
                {#if h.id === activeId}
                  <span class="badge">{t('app.hosts.active')}</span>
                {/if}
              </span>
              <span class="hostmeta">
                <span class="mono">{host(h.endpoint)}</span>
                <span class="quiet">{relTime(h.lastUsedAt, app.now)}</span>
              </span>
            </button>
            {#if repairHintId === h.id}
              <p class="error" role="alert">{t('app.hosts.repairHint')}</p>
            {/if}
            {#if h.id !== activeId}
              <button class="rm" class:armed={removeArmedId === h.id} onclick={() => onRemoveHost(h.id)}>
                {removeArmedId === h.id ? t('app.hosts.removeConfirm') : t('app.hosts.remove')}
              </button>
            {/if}
          </div>
        {/each}
        {#if addHostOpen}
          <label class="lbl" for="host-offer">{t('app.hosts.offerLabel')}</label>
          <textarea
            id="host-offer"
            bind:value={addOffer}
            rows="3"
            placeholder={t('app.hosts.offerPlaceholder')}
            autocapitalize="off"
            spellcheck="false"
          ></textarea>
          {#if addError}
            <p class="error" role="alert">{addError}</p>
          {/if}
          {#if addOffer.trim() === ''}
            <button class="act" disabled={addBusy} onclick={() => void pasteAndAddHost()}>
              {addBusy ? t('app.hosts.checking') : t('app.hosts.pastePair')}
            </button>
          {:else}
            <button class="act" disabled={addBusy} onclick={() => void submitAddHost()}>
              {addBusy ? t('app.hosts.checking') : t('app.hosts.pair')}
            </button>
          {/if}
          {#if !DEV}
            <button class="act" onclick={() => void scanAddHost()}>{t('app.hosts.scan')}</button>
          {/if}
          <button class="act" onclick={() => { addHostOpen = false; addError = null }}>
            {t('app.hosts.addHide')}
          </button>
        {:else}
          <button class="act" onclick={() => { addHostOpen = true; addError = null }}>
            {t('app.hosts.add')}
          </button>
        {/if}
      </section>
      {/if}

      <section>
        <h3>{t('app.mirrorSection')}</h3>
        <p class="line">
          <span>{t('app.mirrorIssues', { n: app.issues.length })}</span>
          <span class="quiet">
            {#if app.offline}
              {t('app.offlineLastSync', {
                when: app.lastSyncAt
                  ? relTime(app.lastSyncAt.toISOString(), app.now)
                  : t('app.syncNever'),
              })}
            {:else if app.syncing}
              {t('sync.busy')}
            {:else if app.lastSyncAt}
              {t('sync.settledOk', { when: relTime(app.lastSyncAt.toISOString(), app.now) })}
            {:else}
              {t('app.notSyncedYet')}
            {/if}
          </span>
        </p>
        <button class="act" onclick={() => void sync()} disabled={app.syncing}>{t('sync.now')}</button>
      </section>

      <section>
        <h3>{t('app.identitySection')}</h3>
        {#if hasIdentity(app.me)}
          <p class="big">{app.me?.name || app.me?.email}</p>
          {#if app.me?.email && app.me?.name}
            <p class="sub">{app.me.email}</p>
          {/if}
          <p class="sub">{t('app.identityFilterNote', { view: t('view.myWork.name') })}</p>
        {:else}
          <p class="line">
            <span class="quiet"
              >{t('pairing.noIdentityLocal', {
                tracker: t('settings.workspaceBuiltIn'),
                issues: t('doc.issues'),
                view: t('view.allOpen.name'),
              })}</span
            >
          </p>
        {/if}
      </section>

      {#if !app.hosted}
        <!-- Hidden on a hosted page (GDK-1966): nothing was paired, so
             there is nothing to unpair and nothing to warn about. -->
        <section>
          <button class="unpair" class:armed onclick={onUnpair}>
            {armed ? t('app.unpairConfirm') : t('app.unpairPhone')}
          </button>
          <p class="sub center">{t('app.unpairWarn')}</p>
        </section>
      {/if}
    {/if}

    <p class="ver">gadak mobile {tauriConf.version}</p>
    {#if DEV}
      <p class="probe" data-testid="viewport-probe" hidden>DEV {viewportProbe}</p>
    {/if}
  </div>
</Screen>

<style>
  .head {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    padding: 4px 0 10px;
  }
  h1 {
    margin: 0;
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
  }
  /* The layer's exit, in the corner Detail puts it and at the same 44pt.
     Above the heading rather than beside it: the ledger heading is 26px and
     a back control on its baseline would push the title into an ellipsis on
     a 402px screen. */
  .back {
    display: flex;
    align-items: center;
    gap: 2px;
    min-height: var(--spacing-control);
    padding-right: 12px;
    margin-left: -6px;
    color: var(--color-accent-text);
  }
  .back svg {
    width: 22px;
    height: 22px;
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
  /* The size row (GDK-901): four equal tappable segments over the 44pt
     floor, numbers in the identifier face, the chosen one carrying the
     accent thread — the same dialect as every other control here (hairline
     border, no fills). */
  .opt {
    margin: 12px 0 6px;
  }
  .sizes {
    display: flex;
    gap: 8px;
    /* 24px below, not the chips' own 8px: at chip spacing the unpair
       button read as a fifth chip (vision verdict, GDK-901 2026-09-15) —
       proximity has to bind it to the shell block, not to this row. */
    margin: 2px 0 24px;
  }
  .size {
    flex: 1;
    min-height: var(--spacing-control);
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-body);
    background: transparent;
  }
  .size.on {
    color: var(--color-accent-text);
    border-color: var(--color-accent-text);
    font-weight: 600;
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
  .hostrow {
    margin: 0 0 4px;
  }
  /* A ledger row, not a card (GDK-879): hairline under, no frame. */
  .host {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 0;
    border-bottom: 1px solid var(--color-border-subtle);
    background: transparent;
    text-align: left;
  }
  .hostline {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .hostlabel {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    flex-shrink: 0;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-accent-text);
  }
  .hostmeta {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    min-width: 0;
    font-size: var(--text-micro);
  }
  .rm {
    width: 100%;
    margin: 2px 0 8px;
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
    font-weight: 500;
    font-size: var(--text-micro);
    background: transparent;
  }
  .rm.armed {
    background: var(--color-text-secondary);
    border-color: var(--color-text-secondary);
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
