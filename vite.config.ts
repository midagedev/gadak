import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// gadak serves the UI from the root of a localhost origin, so the default base is
// `/`. Deployments behind a subpath set GADAK_BASE_PATH; the app reads the same
// value at runtime through import.meta.env.BASE_URL (see web/src/lib/config.ts),
// which is what keeps the service worker scope and manifest correct.
// Hosted demo (GitHub Pages project site) builds with GADAK_BASE_PATH=/gadak/.
const base = process.env.GADAK_BASE_PATH || '/'
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
    name: 'gadak-embed-placeholder',
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


export default defineConfig({
  root: 'web',
  base,
  plugins: [svelte(), tailwindcss(), keepEmbedPlaceholder()],
  build: {
    // GDK-1045 (2026-08-27): vite's default target ('modules', ~es2020)
    // downlevels `||=` into `(void 0 || (i = {}))` with `i` never declared,
    // so the bundled xterm's requestMode threw ReferenceError on the first
    // DECRQM (crush's `ESC[?2026$p` handshake) and every byte after it in
    // that write chunk was dropped — TUIs rendered nothing in the pane while
    // upstream xterm (and our source) were fine. es2022 keeps logical
    // assignment intact and matches mobile/vite.config.ts, whose xterm
    // bundle is the healthy reference for the same dependency set. If an
    // older local browser ever matters: es2021 (Safari 14, 2020-09, first
    // logical-assignment support) is the lowest floor that still seals this.
    // e2e/terminal-modes.spec.ts pins the seal against the built bundle —
    // the defect only exists in build output, never on the dev server.
    target: 'es2022',
    outDir: defaultOutDir,
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    // Dev server talks to a locally running `gadak serve`.
    proxy: {
      '/api': 'http://127.0.0.1:7777',
      '/config.json': 'http://127.0.0.1:7777',
    },
  },
})
