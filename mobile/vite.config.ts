/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

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
// editing this file. GADAK_SERVE_PORT follows GADAK_E2E_PORT's
// single-owner pattern (e2e/helpers.ts e2eServePort); unset is 7899, the
// default every script that starts a demo for this proxy still uses.
function serveDevPort(): string {
  const raw = process.env.GADAK_SERVE_PORT
  if (raw === undefined || raw === '') return '7899'
  if (!/^[1-9][0-9]*$/.test(raw) || Number(raw) > 65535) {
    throw new Error(`GADAK_SERVE_PORT must be an integer 1-65535, got ${JSON.stringify(raw)}`)
  }
  return raw
}

const SERVE_DEV_ORIGIN = `http://127.0.0.1:${serveDevPort()}`

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
    watch: { ignored: ['**/src-tauri/**'] },
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
