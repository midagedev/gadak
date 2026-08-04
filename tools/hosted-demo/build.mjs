#!/usr/bin/env node
/**
 * Build the zero-install hosted demo into dist/hosted.
 *
 * 1. Vite build with VITE_HOSTED_DEMO=1 and base /scry/ (GitHub Pages project site)
 * 2. scry export-static freezes examples/demo.db → bootstrap/detail/attachments
 *
 * Usage (from repo root):
 *   node tools/hosted-demo/build.mjs
 *   make hosted-demo
 */
import { spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const outDir = join(root, 'dist', 'hosted')
const basePath = process.env.SCRY_BASE_PATH || '/scry/'
const apiBase = basePath.endsWith('/')
  ? `${basePath}api/v1/issues/`
  : `${basePath}/api/v1/issues/`
const authBase = basePath.endsWith('/')
  ? `${basePath}api/v1/auth/`
  : `${basePath}/api/v1/auth/`

function run(cmd, args, env = {}) {
  console.log(`+ ${cmd} ${args.join(' ')}`)
  const r = spawnSync(cmd, args, {
    cwd: root,
    stdio: 'inherit',
    env: { ...process.env, ...env },
  })
  if (r.status !== 0) {
    process.exit(r.status ?? 1)
  }
}

function findScryBin() {
  const candidates = [
    join(root, 'bin', 'scry'),
    join(root, 'e2e', '.tmp', 'scry'),
    'scry',
  ]
  for (const c of candidates) {
    if (c === 'scry') return c
    if (existsSync(c)) return c
  }
  return null
}

// ── 1. Ensure binary ────────────────────────────────────────────────────────
let bin = findScryBin()
if (!bin || bin === 'scry') {
  mkdirSync(join(root, 'bin'), { recursive: true })
  run('go', ['build', '-trimpath', '-o', 'bin/scry', './cmd/scry'])
  bin = join(root, 'bin', 'scry')
}

// ── 2. Vite hosted-demo build ───────────────────────────────────────────────
rmSync(outDir, { recursive: true, force: true })
mkdirSync(outDir, { recursive: true })
const viteBin = join(root, 'node_modules', '.bin', 'vite')
if (!existsSync(viteBin)) {
  console.error('hosted-demo: vite not found — run npm ci first')
  process.exit(1)
}
run(viteBin, ['build'], {
  VITE_HOSTED_DEMO: '1',
  SCRY_BASE_PATH: basePath,
  HOSTED_OUT: outDir,
})

if (!existsSync(join(outDir, 'demo-sw.js'))) {
  console.error('hosted-demo: demo-sw.js missing from build output')
  process.exit(1)
}
if (!existsSync(join(outDir, 'index.html'))) {
  console.error('hosted-demo: index.html missing from build output')
  process.exit(1)
}

// ── 3. Freeze demo.db into static JSON ──────────────────────────────────────
// Flags before positional outdir (Go flag.Parse stops at the first non-flag).
run(bin, [
  'export-static',
  '--db',
  'examples/demo.db',
  '--attachments',
  'examples/attachments',
  '--api-base',
  apiBase,
  '--auth-base',
  authBase,
  outDir,
])
// ── 4. Sanity ───────────────────────────────────────────────────────────────
const boot = join(outDir, 'bootstrap.json')
const detail = join(outDir, 'detail')
if (!existsSync(boot) || !existsSync(detail)) {
  console.error('hosted-demo: export-static did not produce bootstrap/detail')
  process.exit(1)
}
const bootSize = statSync(boot).size
console.log(`hosted-demo: ready at ${outDir} (bootstrap ${bootSize} bytes, base ${basePath})`)
console.log(`hosted-demo: serve with  npx serve dist -p 4173  then open http://127.0.0.1:4173${basePath}`)
console.log('              (files live at dist/hosted; serve parent with a /scry rewrite, or:')
console.log('               mkdir -p dist/pages/scry && cp -R dist/hosted/. dist/pages/scry/')
console.log('               npx serve dist/pages -p 4173)')
