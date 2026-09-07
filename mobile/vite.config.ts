/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { mobileAPIPort } from './e2e/serve'

// The gadak design tokens live in web/src/app.css and are imported from
// mobile/src/app.css — referenced, never copied. That file sits outside the
// mobile/ root, so the dev server's fs boundary is the repo root. The @theme
// block only becomes CSS variables through the Tailwind v4 pipeline, which is
// why mobile runs the same @tailwindcss/vite plugin the web app does.
const repoRoot = fileURLToPath(new URL('../', import.meta.url))

// Tauri sets TAURI_DEV_HOST when the app runs on a device (mobile target).
const host = process.env.TAURI_DEV_HOST

// Dev proxy: `gadak demo --addr 127.0.0.1:7899` (or any serve on that port).
// The mirror sends no CORS headers by design (docs/ARCHITECTURE.md), so the
// browser-facing dev origins (vite dev server, and the tauri dev window which
// loads from it) reach serve same-origin through this proxy. The packaged app
// rides tauri-plugin-http instead of this proxy — see lib/api.ts.
//
// The port is env-openable: a second serve on another port — a parallel
// workspace with its own home — can back a phone dev harness without
// editing this file. The single owner of that value is mobile/e2e/serve.ts
// (GDK-1540): GADAK_MOBILE_API_PORT, with the older GADAK_SERVE_PORT still
// honoured and 7899 when neither is set. It has to be the same owner the
// Playwright configs read, because `vite preview` serves the gate's bundle
// and inherits this very proxy (preview.proxy ?? server.proxy) — a proxy
// aimed at a port the gate did not start is a silent mis-serve.
const SERVE_DEV_ORIGIN = `http://127.0.0.1:${mobileAPIPort()}`

/**
 * What the dev server must not watch (GDK-1526).
 *
 * Scope note (GDK-1540): this list now protects `npm run dev` only. The
 * viewport gate and the capture harness no longer run a dev server at all —
 * they build the bundle and serve it with `vite preview`
 * (mobile/e2e/gate-serve.sh), which has no watcher, because no ignore glob
 * could fence off the reload an *app source* edit is supposed to cause.
 *
 * The watch root is mobile/, which also holds the whole test harness — the
 * Playwright specs, their two configs, the captures they save, Playwright's
 * output dir and the vitest units. A write to a .ts file in that root
 * reloads the page, and a reload throws the app back to its first screen
 * under every spec still running, so one edit lands as a pile of unrelated
 * click timeouts. None of these files is app source; none may reload the app.
 *
 * Measured 2026-09-07 on vite 6.4.3, a dev server with a chromium client
 * attached (an empty module graph reports nothing, which is why an earlier
 * clientless probe read as "no reload" everywhere):
 *   e2e/viewport.spec.ts re-saved  → page reload
 *   playwright.config.ts re-saved  → page reload
 *   src/lib/types.ts re-saved      → page reload   (wanted — app source)
 *   src/screens/Search.svelte      → hmr update, no reload
 *   test-results/<x>/trace.zip     → no reload
 *   e2e/.shots/<x>.png             → no reload
 * So the trace/screenshot output named in the ticket is not a trigger: vite
 * already ships '**\/test-results/**' in its own default ignore list
 * (node_modules/vite/dist/node/chunks/dep-Dm0c1Wj2.js:27542; user globs are
 * appended to that list, not substituted for it) and a .png nothing imports
 * matches no module. The trigger that does fire is the harness *sources*.
 * test-results/ is listed here anyway so this function, not a vite default
 * that may change, is the single owner of the boundary.
 *
 * The vitest units are the one entry that has to be conditional. Vitest
 * builds its own file watcher from this very option, so ignoring the units
 * unconditionally leaves `vitest --watch` deaf to an edited test — measured
 * the same day: with the glob the edit produced no output at all, without it
 * the same edit printed "RERUN". Under vitest the units stay watched; under
 * the app's dev server they do not.
 *
 * Kept narrow on purpose: src/, index.html and the shared tokens in
 * web/src/ must keep reloading. mobile/src/lib/watch-boundary.test.ts
 * asserts both directions of that, and mobile/e2e/reload-isolation.spec.ts
 * proves the behaviour against a live server.
 */
export function watchIgnored(env: NodeJS.ProcessEnv = process.env): string[] {
  return [
    '**/src-tauri/**',
    '**/e2e/**', // Playwright specs + the .shots captures they write
    '**/shots/**', // the capture harness testDir (shots.config.ts)
    '**/test-results/**', // Playwright outputDir (also a vite default)
    '**/playwright.config.ts', // the gate's own config, which sits beside the app's
    '**/shots.config.ts', // and the capture harness's
    ...(env.VITEST ? [] : ['**/*.test.ts']), // vitest units, unless vitest is the one watching
  ]
}

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  clearScreen: false,
  server: {
    port: 5180,
    strictPort: true,
    host: host || false,
    fs: { allow: [repoRoot] },
    proxy: {
      '/api': {
        target: SERVE_DEV_ORIGIN,
        changeOrigin: true,
        ws: true,
        // Browser POST/WS send Origin: the vite page (:5182). The serve's
        // browser guard requires Origin to match Host exactly, and allows
        // a missing Origin (CLI/curl). The proxy is the trust boundary
        // (DESIGN.md §7), so it strips Origin the way a loopback client
        // would — otherwise create-session and the PTY upgrade 403.
        configure(proxy) {
          const stripOrigin = (proxyReq: { removeHeader(name: string): void }) => {
            proxyReq.removeHeader('origin')
          }
          proxy.on('proxyReq', stripOrigin)
          proxy.on('proxyReqWs', stripOrigin)
        },
      },
    },
    watch: { ignored: watchIgnored() },
  },
  envPrefix: ['VITE_', 'TAURI_ENV_'],
  build: {
    target: 'es2022',
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
