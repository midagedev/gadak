#!/usr/bin/env node
/**
 * Build the zero-install hosted demo into dist/hosted.
 *
 * 1. Vite build with VITE_HOSTED_DEMO=1 and base /gadak/ (GitHub Pages project site)
 * 2. gadak export-static freezes examples/demo.db → bootstrap/detail/attachments
 *
 * Usage (from repo root):
 *   node tools/hosted-demo/build.mjs
 *   make hosted-demo
 */
import { spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const outDir = join(root, 'dist', 'hosted')
const basePath = process.env.GADAK_BASE_PATH || '/gadak/'
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

function findGadakBin() {
  const candidates = [
    join(root, 'bin', 'gadak'),
    join(root, 'e2e', '.tmp', 'gadak'),
    'gadak',
  ]
  for (const c of candidates) {
    if (c === 'gadak') return c
    if (existsSync(c)) return c
  }
  return null
}

// ── 1. Ensure binary ────────────────────────────────────────────────────────
let bin = findGadakBin()
if (!bin || bin === 'gadak') {
  mkdirSync(join(root, 'bin'), { recursive: true })
  run('go', ['build', '-trimpath', '-o', 'bin/gadak', './cmd/gadak'])
  bin = join(root, 'bin', 'gadak')
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
  GADAK_BASE_PATH: basePath,
  HOSTED_OUT: outDir,
})

if (!existsSync(join(outDir, 'demo-sw.js'))) {
  console.error('hosted-demo: demo-sw.js missing from build output')
  process.exit(1)
}
const indexPath = join(outDir, 'index.html')
if (!existsSync(indexPath)) {
  console.error('hosted-demo: index.html missing from build output')
  process.exit(1)
}

// The tab title is the one label a visitor sees before the app paints, and it
// follows a bookmark or a shared link anywhere. Say "demo" there too.
// Social meta is hosted-only: injected here, never in web/index.html.
const indexHtml = readFileSync(indexPath, 'utf8')
const titled = indexHtml.replace('<title>gadak</title>', '<title>gadak — live demo</title>')
if (titled === indexHtml) {
  console.error('hosted-demo: could not retitle index.html — the <title> tag changed shape')
  process.exit(1)
}
const socialMeta = `
    <meta property="og:type" content="website">
    <meta property="og:site_name" content="gadak">
    <meta property="og:title" content="gadak — Follow the thread.">
    <meta property="og:description" content="Jira and Confluence in one local SQLite file — search it, query it, point your agent at it. This is the live demo.">
    <meta property="og:url" content="https://midagedev.github.io/gadak/">
    <meta property="og:image" content="https://midagedev.github.io/gadak/og.png">
    <meta property="og:image:width" content="1280">
    <meta property="og:image:height" content="640">
    <meta name="twitter:card" content="summary_large_image">
    <meta name="twitter:title" content="gadak — Follow the thread.">
    <meta name="twitter:description" content="Jira and Confluence in one local SQLite file — search it, query it, point your agent at it. This is the live demo.">`
if (!titled.includes('</title>')) {
  console.error('hosted-demo: could not inject social meta — the </title> tag is missing')
  process.exit(1)
}
const withMeta = titled.replace('</title>', `</title>${socialMeta}`)
if (withMeta === titled) {
  console.error('hosted-demo: could not inject social meta — the </title> tag is missing')
  process.exit(1)
}
writeFileSync(indexPath, withMeta)

const ogSrc = join(root, 'docs', 'media', 'og.png')
if (!existsSync(ogSrc)) {
  console.error('hosted-demo: docs/media/og.png missing')
  process.exit(1)
}
copyFileSync(ogSrc, join(outDir, 'og.png'))

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
console.log('              (files live at dist/hosted; serve parent with a /gadak rewrite, or:')
console.log('               mkdir -p dist/pages/gadak && cp -R dist/hosted/. dist/pages/gadak/')
console.log('               npx serve dist/pages -p 4173)')
