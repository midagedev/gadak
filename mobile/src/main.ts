import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

// Dev-only error surface: the packaged webview has no console a developer
// can reach from the couch, and a white screen is not a diagnosis. Never
// prints token values because no code path puts them in an Error message.
if (import.meta.env.DEV) {
  const show = (msg: string) => {
    const el = document.createElement('pre')
    el.style.cssText =
      'position:fixed;left:8px;right:8px;bottom:8px;z-index:9999;background:#8f3530;color:#fff;' +
      'padding:12px;border-radius:6px;font-size:11px;white-space:pre-wrap;max-height:40%;overflow:auto'
    el.textContent = msg
    document.body.appendChild(el)
  }
  window.addEventListener('error', (e) => show(`error: ${e.message}\n${e.filename}:${e.lineno}`))
  window.addEventListener('unhandledrejection', (e) => show(`unhandled rejection: ${String(e.reason)}`))
}

const app = mount(App, {
  target: document.getElementById('app')!,
})

// Self-driving capture tour — armed only when /__demo-tour__ exists (DEV;
// see lib/demo-tour.ts). Static import keeps main.ts free of async top-level.
if (import.meta.env.DEV) {
  void import('./lib/demo-tour').then((m) => m.armDemoTourInDev())
}

export default app
