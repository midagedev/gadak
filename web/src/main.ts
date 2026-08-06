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
 *
 * Returns false when the demo cannot work here: no service-worker support, a
 * failed registration, or a worker that never takes control. In-app browsers
 * (Twitter/X, Instagram, some KakaoTalk WebViews) restrict service workers,
 * and without one every API call falls through to 404s — better to say so
 * than to boot into a broken shell.
 */
async function registerHostedDemoSW(): Promise<boolean> {
  if (import.meta.env.VITE_HOSTED_DEMO !== '1') return true
  if (!('serviceWorker' in navigator)) return false
  // Restricted WebViews don't just reject — register() or `ready` can hang
  // forever. Race the whole handshake against a deadline so the visitor gets
  // the notice instead of an eternal skeleton.
  const handshake = (async () => {
    await navigator.serviceWorker.register(`${basePath()}demo-sw.js`, {
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
    return navigator.serviceWorker.controller !== null
  })()
  try {
    return await Promise.race([
      handshake,
      new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 6000)),
    ])
  } catch (e) {
    console.warn('[scry] hosted demo service worker failed to register', e)
    return false
  }
}

/**
 * Full-screen notice for browsers where the demo cannot run. Static DOM on
 * purpose — the app itself must not boot (it would render a broken shell).
 */
function renderUnsupportedNotice(el: HTMLElement): void {
  const url = window.location.href.split('#')[0]
  el.innerHTML = `
    <div style="min-height:100%;display:flex;align-items:center;justify-content:center;padding:24px;box-sizing:border-box">
      <div style="max-width:420px;text-align:center;line-height:1.6">
        <!-- Inline rather than the Icon component: this notice renders instead
             of booting the app, so no Svelte tree exists yet. -->
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#9aa3ad"
             stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"
             aria-hidden="true" style="margin:0 auto 12px;display:block">
          <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
          <path d="M7 11V7a5 5 0 0 1 10 0v4" />
        </svg>
        <h1 style="font-size:16px;margin:0 0 8px">이 브라우저에서는 데모를 열 수 없어요</h1>
        <p style="font-size:13px;color:#9aa3ad;margin:0 0 6px">
          트위터·인스타그램 같은 인앱 브라우저는 데모가 쓰는 브라우저 기능(서비스 워커)을 막아 둡니다.
          Safari나 Chrome에서 직접 열면 바로 동작합니다.
        </p>
        <p style="font-size:12px;color:#9aa3ad;margin:0 0 14px">
          In-app browsers (Twitter, Instagram, …) block the service worker this
          demo runs on. Open it in Safari or Chrome instead.
        </p>
        <button id="copy-demo-url" style="font-size:13px;padding:8px 14px;border-radius:8px;border:1px solid #364152;background:#1a212b;color:#e6e8eb;cursor:pointer">
          링크 복사 · Copy link
        </button>
        <p style="font-size:11px;color:#6b7480;margin:12px 0 0;word-break:break-all">${url}</p>
      </div>
    </div>`
  document.getElementById('copy-demo-url')?.addEventListener('click', async (ev) => {
    try {
      await navigator.clipboard.writeText(url)
      ;(ev.currentTarget as HTMLButtonElement).textContent = '복사됨 · Copied'
    } catch {
      /* clipboard blocked too — the visible URL below is the fallback */
    }
  })
}

// Load runtime config first — API base, Jira URL, feature flags must be set
// before first render. top-level await is unavailable at es2020, so use an IIFE.
// index.html's inline boot shell fills the screen until then (no white flash).
void (async () => {
  if (!(await registerHostedDemoSW())) {
    renderUnsupportedNotice(target)
    return
  }
  await loadConfig()
  // One-shot migrate issue-nav:* → scry:*. Before store onMount.
  migrateStorageKeys()

  // Clear boot shell and mount in its place.
  target.innerHTML = ''
  mount(App, { target })
})()
