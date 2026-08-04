import { writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// scry serves the UI from the root of a localhost origin, so the default base is
// `/`. Deployments behind a subpath set SCRY_BASE_PATH; the app reads the same
// value at runtime through import.meta.env.BASE_URL (see web/src/lib/config.ts),
// which is what keeps the service worker scope and manifest correct.
const base = process.env.SCRY_BASE_PATH || '/'

// `emptyOutDir` wipes dist/app on every build, including the tracked
// placeholder that keeps `go:embed all:dist/app` compilable before the first
// web build. Put it back once the bundle is written.
const keepEmbedPlaceholder = {
  name: 'scry-embed-placeholder',
  closeBundle() {
    writeFileSync(
      'dist/app/.placeholder',
      'Placeholder so the go:embed directive in embed.go always has a directory.\n' +
        'Run `npm run build` to produce the real web assets here.\n',
    )
  },
}

export default defineConfig({
  root: 'web',
  base,
  plugins: [svelte(), tailwindcss(), keepEmbedPlaceholder],
  build: {
    outDir: '../dist/app',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    // Dev server talks to a locally running `scry serve`.
    proxy: {
      '/api': 'http://127.0.0.1:7777',
      '/config.json': 'http://127.0.0.1:7777',
    },
  },
})
