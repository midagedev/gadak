import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { applyCacheScopeDebug, loadConfig } from './lib/config'
import { installHostedFetch } from './lib/hosted-fetch'
import { migrateStorageKeys } from './lib/storage'
import { applyThemeAtBoot } from './lib/theme'

// Before the async config load — index.html ships data-theme="light" so the
// boot shell has a theme; strip or replace it now so the first real paint
// matches the stored preference (system = no attribute).
applyThemeAtBoot()

const target = document.getElementById('app')
if (!target) throw new Error('#app not found')

// Load runtime config first — API base, Jira URL, feature flags must be set
// before first render. top-level await is unavailable at es2020, so use an IIFE.
// index.html's inline boot shell fills the screen until then (no white flash).
// Hosted demo: wrap fetch before the first request so bootstrap/ does not 404
// on a static host. No service worker — in-app browsers reject those.
void (async () => {
  installHostedFetch()
  await loadConfig()
  applyCacheScopeDebug()
  // One-shot migrate issue-nav:* → gadak:*. Before store onMount.
  migrateStorageKeys()

  // Clear boot shell and mount in its place.
  target.innerHTML = ''
  mount(App, { target })
})()
