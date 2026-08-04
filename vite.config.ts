import { unlinkSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// scry serves the UI from the root of a localhost origin, so the default base is
// `/`. Deployments behind a subpath set SCRY_BASE_PATH; the app reads the same
// value at runtime through import.meta.env.BASE_URL (see web/src/lib/config.ts),
// which is what keeps the service worker scope and manifest correct.
// Hosted demo (GitHub Pages project site) builds with SCRY_BASE_PATH=/scry/.
const base = process.env.SCRY_BASE_PATH || '/'
const hostedDemo = process.env.VITE_HOSTED_DEMO === '1'
// Hosted demo must not clobber dist/app (go:embed). Set HOSTED_OUT to a separate
// directory (Makefile uses dist/hosted).
const defaultOutDir = process.env.HOSTED_OUT
  ? resolve(process.env.HOSTED_OUT)
  : resolve('dist/app')

// `emptyOutDir` wipes dist/app on every build, including the tracked
// placeholder that keeps `go:embed all:dist/app` compilable before the first
// web build. Put it back once the bundle is written.
function keepEmbedPlaceholder(): Plugin {
  let resolvedOut = defaultOutDir
  return {
    name: 'scry-embed-placeholder',
    configResolved(cfg) {
      resolvedOut = resolve(cfg.root, cfg.build.outDir)
    },
    closeBundle() {
      // Only when the normal embed outDir was the target.
      if (resolvedOut !== resolve('dist/app')) return
      writeFileSync(
        'dist/app/.placeholder',
        'Placeholder so the go:embed directive in embed.go always has a directory.\n' +
          'Run `npm run build` to produce the real web assets here.\n',
      )
    },
  }
}

// demo-sw.js lives in web/public so Vite copies it, but the normal local build
// must not ship it — only the hosted-demo target registers or needs it.
function stripDemoSW(): Plugin {
  let resolvedOut = defaultOutDir
  return {
    name: 'scry-strip-demo-sw',
    configResolved(cfg) {
      resolvedOut = resolve(cfg.root, cfg.build.outDir)
    },
    closeBundle() {
      if (hostedDemo) return
      try {
        unlinkSync(resolve(resolvedOut, 'demo-sw.js'))
      } catch {
        /* not present */
      }
    },
  }
}

export default defineConfig({
  root: 'web',
  base,
  plugins: [svelte(), tailwindcss(), keepEmbedPlaceholder(), stripDemoSW()],
  build: {
    outDir: defaultOutDir,
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
