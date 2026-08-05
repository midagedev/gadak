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

// 런타임 설정을 먼저 읽는다 — API base·Jira URL·기능 플래그가 첫 렌더 전에 확정돼야 한다.
// top-level await 은 빌드 타깃(es2020)에서 못 쓰므로 async IIFE 로 감싼다.
// index.html 의 인라인 부트 셸이 그동안 화면을 채우고 있어 흰 플래시는 없다.
void (async () => {
  await registerHostedDemoSW()
  await loadConfig()
  // 옛 issue-nav:* → scry:* 1회 이관. 스토어 onMount 보다 먼저.
  migrateStorageKeys()

  // 부트 셸을 비우고 그 자리에 마운트한다.
  target.innerHTML = ''
  mount(App, { target })
})()
