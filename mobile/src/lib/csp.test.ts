import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * GDK-1137 — the packaged webview's connect-src stays as narrow as its real
 * consumers. The eye never sees a CSP, so it can only drift one way: wider.
 * The measured consumer map (tauri 2.11.5, this app):
 *
 *   - demo bundle + dev /api proxy — window.fetch on same-origin URLs
 *   - packaged API + terminal WS — @tauri-apps/plugin-http /
 *     plugin-websocket, native and outside CSP entirely
 *   - tauri's own IPC on iOS — ipc://localhost fetch is refused by this same
 *     policy and falls back to window.ipc.postMessage (scripts/ipc-protocol.js
 *     in the tauri crate), so 'self' costs nothing there either
 *   - dev needs no exception: PROXY_DEV_SERVER (all(dev, mobile)) proxies the
 *     dev server through the custom protocol and passes vite's responses
 *     through verbatim — no CSP header is applied in dev at all
 *
 * Source contracts over config files in this directory's established style
 * (KeyBar.test.ts reads .svelte sources the same way).
 */

const here = dirname(fileURLToPath(import.meta.url))
const conf = JSON.parse(
  readFileSync(join(here, '..', '..', 'src-tauri', 'tauri.conf.json'), 'utf8'),
) as { app: { security: { csp: string } } }
const csp: string = conf.app.security.csp

/** Tokens of the connect-src directive; [] when the directive is absent. */
function connectSources(policy: string): string[] {
  const m = /(?:^|;\s*)connect-src\s+([^;]+)/.exec(policy)
  return m ? m[1].trim().split(/\s+/) : []
}

const ALLOWED_CONNECT = ["'self'"]

describe('GDK-1137 — connect-src stays as narrow as its real consumers', () => {
  it('names a connect-src directive at all', () => {
    // Deleting the directive would silently hand the job to default-src;
    // the explicit directive is the contract this file pins.
    expect(connectSources(csp)).not.toEqual([])
  })

  it('every connect-src token is on the allowlist', () => {
    const offenders = connectSources(csp).filter((t) => !ALLOWED_CONNECT.includes(t))
    if (offenders.length > 0) {
      throw new Error(
        `connect-src carries tokens beyond ${ALLOWED_CONNECT.join(' ')}: ${offenders.join(' ')}. ` +
          'Every webview network path is same-origin or native (see the header map); ' +
          'a bare scheme token here reopens the webview to the network at large. If a new ' +
          'consumer genuinely needs one, name it in DESIGN.md §9 and move ALLOWED_CONNECT ' +
          'in the same commit.',
      )
    }
  })
})
