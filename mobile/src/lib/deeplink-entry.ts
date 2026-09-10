/*
 * GDK-873 — the wiring half of gadak:// deep links.
 *
 * deeplink.ts decides; this file connects that decision to the OS on one
 * side and the store on the other. The split is what makes the decision
 * testable without a simulator, so keep this file thin: anything here that
 * is worth asserting belongs over there instead.
 *
 * Two problems this layer exists to solve, neither of which is parsing:
 *
 *  ① A link can arrive before the app can act on it. iOS delivers a cold
 *    launch URL while the webview is still booting — app.phase is 'boot',
 *    there is no issue set, and openIssue() would push a detail screen over
 *    nothing. So a target that arrives early is HELD, and released once the
 *    app reaches 'paired'. Exactly one is held: a second link supersedes the
 *    first, because the user's last tap is the one they are waiting on.
 *
 *  ② A link that is refused must be refused *visibly enough*, but never
 *    noisily. A URL that is not ours passes in silence — another app's
 *    scheme can reach a shared handler and that is not an error. One that is
 *    ours and refused calls onRefused, which App.svelte wires to a toast:
 *    a link that does nothing at all, with no reason anywhere, is the
 *    failure mode desktop/deeplink.go's log exists to prevent, and a phone
 *    has no log a user can read.
 */

import { decideDeepLink, type ParseFailure, type ResolveRefusal } from './deeplink'

/** The reasons a link produced no navigation. `not_gadak` never surfaces. */
export type DeepLinkRefusal = ParseFailure | ResolveRefusal

export interface DeepLinkSink {
  /** Open this issue now. Wired to store.openIssue + switchTab('issues'). */
  openIssue: (key: string) => void
  /** True once the app can actually show a detail screen. */
  ready: () => boolean
  /** Told about a link that was ours and refused. Never called for another
   *  app's scheme. */
  onRefused?: (refusal: DeepLinkRefusal, why: string) => void
}

/**
 * A deep-link router with no dependency on Tauri or on the store.
 *
 * `deliver` takes the raw string from the OS. `flush` is called when the app
 * becomes ready and releases anything held. Both are safe to call at any
 * time and in any order — which is the point, because the order the OS uses
 * is not ours to choose.
 */
export function createDeepLinkRouter(sink: DeepLinkSink) {
  let held: string | null = null

  function act(key: string): void {
    if (!sink.ready()) {
      held = key
      return
    }
    held = null
    sink.openIssue(key)
  }

  return {
    /** Handle one URL from the OS. Never throws: this runs on an OS
     *  callback with attacker-influenced input, where a throw is an
     *  unhandled rejection inside the webview rather than a refusal. */
    deliver(raw: string): void {
      let decision: ReturnType<typeof decideDeepLink>
      try {
        decision = decideDeepLink(raw)
      } catch {
        // decideDeepLink is total by construction and asserted so in
        // deeplink.test.ts; this is belt-and-braces for an OS callback.
        return
      }
      if (decision.ok) {
        act(decision.target.key)
        return
      }
      // Not ours: stay silent. Anything else: say why.
      if (decision.reason === 'not_gadak') return
      sink.onRefused?.(decision.reason, decision.why)
    },

    /** Release a link that arrived before the app could show it. */
    flush(): void {
      if (held !== null && sink.ready()) {
        const key = held
        held = null
        sink.openIssue(key)
      }
    },

    /** Test seam: what is waiting for readiness, if anything. */
    pending(): string | null {
      return held
    },
  }
}

export type DeepLinkRouter = ReturnType<typeof createDeepLinkRouter>

/**
 * Subscribes a router to the OS.
 *
 * The plugin is imported dynamically and failures are swallowed on purpose:
 * the dev webview runs in a plain browser tab under vite where
 * @tauri-apps/plugin-deep-link has no backend at all, and deep links are an
 * enhancement — an app that refused to boot without them would be worse on
 * every surface than one that simply has no deep links there.
 *
 * Returns a teardown. `getCurrent()` covers the cold-launch case, where the
 * URL that started the app was delivered before anything subscribed.
 */
export async function bindOsDeepLinks(router: DeepLinkRouter): Promise<() => void> {
  let unlisten: (() => void) | null = null
  try {
    const plugin = await import('@tauri-apps/plugin-deep-link')
    const current = await plugin.getCurrent()
    for (const url of current ?? []) router.deliver(url)
    unlisten = await plugin.onOpenUrl((urls) => {
      for (const url of urls) router.deliver(url)
    })
  } catch {
    // No deep-link backend here (a browser tab, or a build without the
    // plugin). Nothing to bind and nothing to report.
  }
  return () => {
    unlisten?.()
    unlisten = null
  }
}

/**
 * Test seam for the e2e harness (GDK-873).
 *
 * A Playwright spec runs the built bundle in a real browser, where no OS
 * ever delivers a gadak:// URL — so the one thing a browser CAN prove about
 * this feature is that the entry point routes a URL to the right screen.
 * Exposing the router under a namespaced global is how the spec reaches it.
 *
 * This is not a back door into the app: the router only ever calls
 * openIssue, the scheme carries no verb, and every string still goes through
 * the same parser and the same refusals. A page that could call this could
 * already call the store directly.
 */
export const DEEP_LINK_TEST_HOOK = '__gadakDeepLink'

export function exposeForTests(router: DeepLinkRouter): void {
  ;(window as unknown as Record<string, unknown>)[DEEP_LINK_TEST_HOOK] = (raw: string) => router.deliver(raw)
}
