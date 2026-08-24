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
// talks to the configured endpoint directly — see lib/api.ts.
const SERVE_DEV_ORIGIN = 'http://127.0.0.1:7899'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  clearScreen: false,
  server: {
    port: 5180,
    strictPort: true,
    host: host || false,
    fs: { allow: [repoRoot] },
    proxy: {
      '/api': { target: SERVE_DEV_ORIGIN, changeOrigin: true },
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
