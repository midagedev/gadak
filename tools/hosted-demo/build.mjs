#!/usr/bin/env node
/**
 * Build the zero-install hosted demo into dist/hosted.
 *
 * 1. Vite build with VITE_HOSTED_DEMO=1 and base /gadak/ (GitHub Pages project site)
 * 2. Copy demo video/poster/og assets (About popover links to web-demo.mp4)
 * 3. gadak export-static freezes examples/demo.db → bootstrap/detail/attachments
 *
 * Usage (from repo root):
 *   node tools/hosted-demo/build.mjs
 *   make hosted-demo
 *   ./tools/hosted-demo/preview.sh   # serve dist/hosted at :4173/gadak/
 */
import { spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
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

const mp4Src = join(root, 'docs', 'media', 'web-demo.mp4')
if (!existsSync(mp4Src)) {
  console.error('hosted-demo: docs/media/web-demo.mp4 missing')
  process.exit(1)
}
copyFileSync(mp4Src, join(outDir, 'web-demo.mp4'))

const ogSrc = join(root, 'docs', 'media', 'og.png')
if (!existsSync(ogSrc)) {
  console.error('hosted-demo: docs/media/og.png missing')
  process.exit(1)
}
copyFileSync(ogSrc, join(outDir, 'og.png'))

// Poster is a still, not the 1.1MB mp4. ffmpeg is optional: Pages CI may
// not have it. Fall back to the OG card copied above.
const posterOut = join(outDir, 'web-demo-poster.jpg')
const ff = spawnSync(
  'ffmpeg',
  ['-y', '-ss', '00:00:02', '-i', mp4Src, '-frames:v', '1', '-q:v', '5', posterOut],
  { cwd: root, stdio: 'pipe' },
)
if (ff.status !== 0 || !existsSync(posterOut)) {
  copyFileSync(join(outDir, 'og.png'), posterOut)
  console.log('hosted-demo: ffmpeg poster skipped — using og.png')
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
// ── 3b. Public backlog (GDK-389) — committed scrubbed snapshot, own bundle ──
// A second Vite build because basePath() is compile-time BASE_URL: the same
// bundle under /gadak/backlog/ would fetch /gadak/config.json (the demo's).
// Git tracks examples/backlog-snapshot.tar.gz. The viewer still fetches
// detail/<KEY>.json, so unpack to a temp tree and copy as before.
let backlogSnapshot = join(root, 'examples', 'backlog-snapshot')
const backlogArchive = join(root, 'examples', 'backlog-snapshot.tar.gz')
if (!existsSync(join(backlogSnapshot, 'bootstrap.json')) && existsSync(backlogArchive)) {
  backlogSnapshot = mkdtempSync(join(tmpdir(), 'gadak-backlog-'))
  run('bash', ['tools/backlog-snapshot.sh', '--unpack', backlogSnapshot])
}
if (existsSync(join(backlogSnapshot, 'bootstrap.json'))) {
  const backlogBase = basePath.endsWith('/') ? `${basePath}backlog/` : `${basePath}/backlog/`
  const backlogOut = join(outDir, 'backlog')
  run(viteBin, ['build'], {
    VITE_HOSTED_DEMO: '1',
    GADAK_BASE_PATH: backlogBase,
    HOSTED_OUT: backlogOut,
  })
  const backlogIndex = join(backlogOut, 'index.html')
  const backlogHtml = readFileSync(backlogIndex, 'utf8')
  const backlogTitled = backlogHtml.replace('<title>gadak</title>', '<title>gadak — public backlog</title>')
  if (backlogTitled === backlogHtml) {
    console.error('hosted-demo: could not retitle backlog index.html')
    process.exit(1)
  }
  writeFileSync(backlogIndex, backlogTitled)
  // About popover links these relative to the base path.
  copyFileSync(mp4Src, join(backlogOut, 'web-demo.mp4'))
  copyFileSync(ogSrc, join(backlogOut, 'og.png'))
  copyFileSync(posterOut, join(backlogOut, 'web-demo-poster.jpg'))
  for (const f of ['bootstrap.json', 'config.json']) {
    copyFileSync(join(backlogSnapshot, f), join(backlogOut, f))
  }
  mkdirSync(join(backlogOut, 'detail'), { recursive: true })
  const detailSrc = join(backlogSnapshot, 'detail')
  for (const f of readdirSync(detailSrc)) {
    copyFileSync(join(detailSrc, f), join(backlogOut, 'detail', f))
  }
  console.log(`hosted-demo: public backlog at ${backlogOut} (base ${backlogBase})`)
} else {
  console.log('hosted-demo: public backlog snapshot missing — skipping backlog page')
}

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
