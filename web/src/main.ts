import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { basePath, loadConfig } from './lib/config'
import { migrateStorageKeys } from './lib/storage'

const target = document.getElementById('app')
if (!target) throw new Error('#app not found')

/**
 * Hosted-demo only (VITE_HOSTED_DEMO=1): register the snapshot service worker
 * and wait until it controls this page so the first bootstrap/ fetch is
 * rewritten to bootstrap.json. Local `scry serve` builds never set the flag
 * and never load demo-sw.js.
 */
async function registerHostedDemoSW(): Promise<void> {
  if (import.meta.env.VITE_HOSTED_DEMO !== '1') return
  if (!('serviceWorker' in navigator)) {
    console.warn('[scry] hosted demo needs a service worker; this browser has none')
    return
  }
  const reg = await navigator.serviceWorker.register(`${basePath()}demo-sw.js`, {
    scope: basePath(),
  })
  await navigator.serviceWorker.ready
  // First visit: activate + clients.claim() may land after ready. Wait briefly
  // so the controller is set before the app boots its API calls.
  if (!navigator.serviceWorker.controller) {
    await Promise.race([
      new Promise<void>((resolve) => {
        navigator.serviceWorker.addEventListener(
          'controllerchange',
          () => resolve(),
          { once: true },
        )
      }),
      new Promise<void>((resolve) => setTimeout(resolve, 3000)),
    ])
  }
  // Keep the registration referenced so tree-shaking never drops the await.
  void reg
}

// Load runtime config first — API base, Jira URL, feature flags must be set
// before first render. top-level await is unavailable at es2020, so use an IIFE.
// index.html's inline boot shell fills the screen until then (no white flash).
void (async () => {
  await registerHostedDemoSW()
  await loadConfig()
  // One-shot migrate issue-nav:* → scry:*. Before store onMount.
  migrateStorageKeys()

  // Clear boot shell and mount in its place.
  target.innerHTML = ''
  mount(App, { target })
})()
