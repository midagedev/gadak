#!/usr/bin/env node
/**
 * Build the public site into dist/hosted.
 *
 *   /         landing page (static, authored here)
 *   /demo/    the zero-install live demo (one Vite build + frozen examples/demo.db)
 *   /backlog/ the public backlog viewer (same bundle + committed snapshot)
 *
 * The site is served at the apex of gadak.dev, so the base path is `/`
 * (GDK-676). It used to be `/gadak/` for midagedev.github.io/gadak/; that
 * host now 301s to the domain with the repo segment dropped, which is why
 * the demo had to leave the root and the base had to lose its prefix.
 * GADAK_BASE_PATH still overrides for a subpath deployment.
 *
 * Usage (from repo root):
 *   node tools/hosted-demo/build.mjs
 *   make hosted-demo
 *   ./tools/hosted-demo/preview.sh   # serve dist/hosted at :4173/
 */
import { spawnSync } from 'node:child_process'
import { copyFileSync, cpSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const outDir = join(root, 'dist', 'hosted')
const basePath = process.env.GADAK_BASE_PATH || '/'
const withSlash = basePath.endsWith('/') ? basePath : `${basePath}/`
// Site layout: /demo/ and /backlog/. One Vite bundle (relative `./` assets);
// each index.html gets a <base href> so basePath() can read the mount at
// runtime (GDK-673). GADAK_BASE_PATH still prefixes a subpath deployment.
const demoBase = `${withSlash}demo/`
const backlogBase = `${withSlash}backlog/`
const demoOut = join(outDir, 'demo')
const apiBase = `${demoBase}api/v1/issues/`
const authBase = `${demoBase}api/v1/auth/`
const siteOrigin = process.env.GADAK_SITE_ORIGIN || 'https://gadak.dev'

function withTrailingSlash(p) {
  return p.endsWith('/') ? p : `${p}/`
}

function injectBaseHref(html, href) {
  const tag = `    <base href="${href}" />\n`
  if (/<base\b/i.test(html)) {
    return html.replace(/<base\b[^>]*>\s*/i, tag)
  }
  if (!html.includes('<head>')) {
    console.error('hosted-demo: no <head> in index.html — cannot inject <base>')
    process.exit(1)
  }
  const next = html.replace('<head>', `<head>\n${tag}`)
  if (next === html) {
    console.error('hosted-demo: could not inject <base> — <head> tag changed shape')
    process.exit(1)
  }
  return next
}

function copyViteShell(src, dest) {
  mkdirSync(dest, { recursive: true })
  for (const name of readdirSync(src)) {
    cpSync(join(src, name), join(dest, name), { recursive: true })
  }
}

function rewriteSnapshotConfig(configPath, appBase) {
  if (!existsSync(configPath)) {
    console.error(`hosted-demo: missing ${configPath}`)
    process.exit(1)
  }
  const doc = JSON.parse(readFileSync(configPath, 'utf8'))
  const base = withTrailingSlash(appBase)
  doc.apiBase = `${base}api/v1/issues/`
  doc.authBase = `${base}api/v1/auth/`
  writeFileSync(configPath, `${JSON.stringify(doc, null, 2)}\n`)
}

function assertSharedBundle(demoDir, backlogDir) {
  const demoAssetsDir = join(demoDir, 'assets')
  const backlogAssetsDir = join(backlogDir, 'assets')
  const demoAssets = readdirSync(demoAssetsDir).sort()
  const backlogAssets = readdirSync(backlogAssetsDir).sort()
  if (JSON.stringify(demoAssets) !== JSON.stringify(backlogAssets)) {
    console.error(
      `hosted-demo: demo/assets [${demoAssets}] != backlog/assets [${backlogAssets}] — the SPA was built twice`,
    )
    process.exit(1)
  }
  for (const name of demoAssets) {
    const a = readFileSync(join(demoAssetsDir, name))
    const b = readFileSync(join(backlogAssetsDir, name))
    if (!a.equals(b)) {
      console.error(`hosted-demo: ${name} bytes differ between demo and backlog — the SPA was built twice`)
      process.exit(1)
    }
  }
  for (const [dir, href] of [
    [demoDir, demoBase],
    [backlogDir, backlogBase],
  ]) {
    const html = readFileSync(join(dir, 'index.html'), 'utf8')
    if (!html.includes(`<base href="${href}"`)) {
      console.error(`hosted-demo: ${dir}/index.html missing <base href="${href}">`)
      process.exit(1)
    }
    if (!html.includes('src="./assets/')) {
      console.error(`hosted-demo: ${dir}/index.html does not use relative Vite assets`)
      process.exit(1)
    }
  }
}

if (process.argv[2] === '--rewrite-backlog-config') {
  const configPath = process.argv[3] || join(root, 'dist/hosted/backlog/config.json')
  const appBase = process.argv[4] || '/backlog/'
  rewriteSnapshotConfig(configPath, appBase)
  console.log(`hosted-demo: rewrote apiBase/authBase on ${configPath} for ${withTrailingSlash(appBase)}`)
  process.exit(0)
}

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
  // Relative emit so one HTML works under /demo/ and /backlog/ (GDK-673).
  // Do not pass demoBase here: that is the site mount, not Vite's asset base.
  GADAK_BASE_PATH: './',
  HOSTED_OUT: demoOut,
})

const indexPath = join(demoOut, 'index.html')
if (!existsSync(indexPath)) {
  console.error('hosted-demo: index.html missing from build output')
  process.exit(1)
}

const backlogOut = join(outDir, 'backlog')
// Copy the Vite shell before export-static pollutes demo/ with snapshot JSON.
copyViteShell(demoOut, backlogOut)

// The tab title is the one label a visitor sees before the app paints, and it
// follows a bookmark or a shared link anywhere. Say "demo" there too.
// Social meta is hosted-only: injected here, never in web/index.html.
const indexHtml = injectBaseHref(readFileSync(indexPath, 'utf8'), demoBase)
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
    <meta property="og:url" content="${siteOrigin}/demo/">
    <meta property="og:image" content="${siteOrigin}/og.png">
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
copyFileSync(mp4Src, join(demoOut, 'web-demo.mp4'))

const ogSrc = join(root, 'docs', 'media', 'og.png')
if (!existsSync(ogSrc)) {
  console.error('hosted-demo: docs/media/og.png missing')
  process.exit(1)
}
copyFileSync(ogSrc, join(demoOut, 'og.png'))
// The landing page's own social card and the favicon set it shares with the
// apps live at the site root, where a crawler that only reads / can find them.
copyFileSync(ogSrc, join(outDir, 'og.png'))

// Poster is a still, not the 1.1MB mp4. ffmpeg is optional: Pages CI may
// not have it. Fall back to the OG card copied above.
const posterOut = join(demoOut, 'web-demo-poster.jpg')
const ff = spawnSync(
  'ffmpeg',
  ['-y', '-ss', '00:00:02', '-i', mp4Src, '-frames:v', '1', '-q:v', '5', posterOut],
  { cwd: root, stdio: 'pipe' },
)
if (ff.status !== 0 || !existsSync(posterOut)) {
  copyFileSync(join(demoOut, 'og.png'), posterOut)
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
  demoOut,
])
// ── 3b. Public backlog (GDK-389) — same Vite shell, committed snapshot ──
// Git tracks examples/backlog-snapshot.tar.gz. The viewer still fetches
// detail/<KEY>.json, so unpack to a temp tree and copy as before.
let backlogSnapshot = join(root, 'examples', 'backlog-snapshot')
const backlogArchive = join(root, 'examples', 'backlog-snapshot.tar.gz')
if (!existsSync(join(backlogSnapshot, 'bootstrap.json')) && existsSync(backlogArchive)) {
  backlogSnapshot = mkdtempSync(join(tmpdir(), 'gadak-backlog-'))
  run('bash', ['tools/backlog-snapshot.sh', '--unpack', backlogSnapshot])
}
if (existsSync(join(backlogSnapshot, 'bootstrap.json'))) {
  const backlogIndex = join(backlogOut, 'index.html')
  const backlogHtml = injectBaseHref(readFileSync(backlogIndex, 'utf8'), backlogBase)
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
  rewriteSnapshotConfig(join(backlogOut, 'config.json'), backlogBase)
  assertSharedBundle(demoOut, backlogOut)
  console.log(`hosted-demo: public backlog at ${backlogOut} (base ${backlogBase})`)
} else {
  rmSync(backlogOut, { recursive: true, force: true })
  console.log('hosted-demo: public backlog snapshot missing — skipping backlog page')
}

// ── 3c. Landing page ────────────────────────────────────────────────────────
// The apex is a door, not an app: one sentence about what gadak is and the
// three places a visitor can go. Static HTML on purpose — it must paint with
// no bundle, and it is the page a shared link lands on.
writeFileSync(join(outDir, 'index.html'), landingHtml())

// Favicons and the manifest are emitted per app by Vite; the landing page
// shares them, so copy the demo's set up to the root.
for (const f of ['icon-16.png', 'icon-32.png', 'icon-512.png', 'apple-touch-icon.png']) {
  const src = join(demoOut, f)
  if (existsSync(src)) copyFileSync(src, join(outDir, f))
}

// A Pages deployment made by an Action replaces the published tree wholesale,
// so the custom domain has to be part of the artifact — the repo setting alone
// has been observed to drop out from under an Actions deploy. Derived from the
// site origin so there is one place that knows the domain.
const siteHost = new URL(siteOrigin).host
if (basePath === '/' && siteHost && !siteHost.endsWith('.github.io')) {
  writeFileSync(join(outDir, 'CNAME'), `${siteHost}\n`)
}

// ── 4. Sanity ───────────────────────────────────────────────────────────────
const boot = join(demoOut, 'bootstrap.json')
const detail = join(demoOut, 'detail')
if (!existsSync(boot) || !existsSync(detail)) {
  console.error('hosted-demo: export-static did not produce bootstrap/detail')
  process.exit(1)
}
const landing = readFileSync(join(outDir, 'index.html'), 'utf8')
for (const href of [`${demoBase}`, `${backlogBase}`]) {
  if (!landing.includes(`href="${href}"`)) {
    console.error(`hosted-demo: landing page does not link ${href} — the site's front door must reach both apps`)
    process.exit(1)
  }
}
const bootSize = statSync(boot).size
console.log(`hosted-demo: ready at ${outDir} (bootstrap ${bootSize} bytes, base ${basePath})`)
console.log(`hosted-demo: landing ${withSlash} · demo ${demoBase} · backlog ${backlogBase}`)
console.log(`hosted-demo: preview with  npx serve dist/hosted -l 4173  then open http://127.0.0.1:4173${withSlash}`)

function landingHtml() {
  // Palette is the app's own (web/src/app.css @theme): paper grounds, ink
  // text, one 쪽빛 indigo thread. Dark mode mirrors the app's tokens.
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="theme-color" content="#f4efe4" media="(prefers-color-scheme: light)" />
    <meta name="theme-color" content="#0f1013" media="(prefers-color-scheme: dark)" />
    <link rel="icon" type="image/png" sizes="32x32" href="${withSlash}icon-32.png" />
    <link rel="icon" type="image/png" sizes="16x16" href="${withSlash}icon-16.png" />
    <link rel="apple-touch-icon" href="${withSlash}apple-touch-icon.png" />
    <title>gadak — Follow the thread.</title>
    <meta name="description" content="Jira and Confluence in one local SQLite file. Search it, query it in SQL, point your coding agent at it. Reads never touch the network." />
    <meta property="og:type" content="website" />
    <meta property="og:site_name" content="gadak" />
    <meta property="og:title" content="gadak — Follow the thread." />
    <meta property="og:description" content="Jira and Confluence in one local SQLite file. Search it, query it in SQL, point your coding agent at it." />
    <meta property="og:url" content="${siteOrigin}/" />
    <meta property="og:image" content="${siteOrigin}/og.png" />
    <meta property="og:image:width" content="1280" />
    <meta property="og:image:height" content="640" />
    <meta name="twitter:card" content="summary_large_image" />
    <meta name="twitter:title" content="gadak — Follow the thread." />
    <meta name="twitter:description" content="Jira and Confluence in one local SQLite file. Search it, query it in SQL, point your coding agent at it." />
    <style>
      :root {
        --bg: #f4efe4;
        --panel: #ebe3d2;
        --elevated: #e4d9c4;
        --border: #d5c9b2;
        --border-strong: #b9ab92;
        --ink: #1c1812;
        --ink-2: #534c42;
        --ink-3: #635a4f;
        --accent: #2e4560;
        --accent-hover: #24384e;
        --on-accent: #f4efe4;
      }
      @media (prefers-color-scheme: dark) {
        :root {
          --bg: #0f1013;
          --panel: #131417;
          --elevated: #1a1c20;
          --border: #2a2d33;
          --border-strong: #3a3e46;
          --ink: #e8e4dc;
          --ink-2: #b3aea4;
          --ink-3: #8d887e;
          --accent: #9db4d0;
          --accent-hover: #b6c8de;
          --on-accent: #0f1013;
        }
      }
      * { box-sizing: border-box; }
      html { -webkit-text-size-adjust: 100%; }
      body {
        margin: 0;
        background: var(--bg);
        color: var(--ink);
        font: 16px/1.6 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
        -webkit-font-smoothing: antialiased;
      }
      main { max-width: 46rem; margin: 0 auto; padding: 4.5rem 1.5rem 5rem; }
      .mark {
        font-size: 2.75rem;
        font-weight: 650;
        letter-spacing: -0.02em;
        margin: 0 0 0.35rem;
      }
      .tagline { margin: 0 0 2rem; color: var(--ink-2); font-size: 1.0625rem; }
      .lede { margin: 0 0 2.5rem; font-size: 1.0625rem; color: var(--ink); }
      .lede code {
        font: 0.9375rem/1.4 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        background: var(--elevated);
        border: 1px solid var(--border);
        border-radius: 4px;
        padding: 0.1rem 0.3rem;
      }
      .doors { display: grid; gap: 0.75rem; margin: 0 0 3rem; }
      a.door {
        display: block;
        padding: 1.05rem 1.25rem;
        background: var(--panel);
        border: 1px solid var(--border);
        border-radius: 8px;
        color: inherit;
        text-decoration: none;
        transition: border-color 120ms ease, background 120ms ease;
      }
      a.door:hover, a.door:focus-visible {
        background: var(--elevated);
        border-color: var(--border-strong);
      }
      a.door:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
      a.door.primary { background: var(--accent); border-color: var(--accent); color: var(--on-accent); }
      a.door.primary:hover, a.door.primary:focus-visible { background: var(--accent-hover); border-color: var(--accent-hover); }
      a.door.primary .sub { color: var(--on-accent); opacity: 0.82; }
      .door .title { font-weight: 620; font-size: 1.0625rem; }
      .door .sub { display: block; margin-top: 0.2rem; color: var(--ink-3); font-size: 0.9375rem; }
      h2 { font-size: 0.8125rem; text-transform: uppercase; letter-spacing: 0.08em; color: var(--ink-3); margin: 0 0 0.75rem; font-weight: 600; }
      pre {
        margin: 0 0 2.5rem;
        padding: 1rem 1.25rem;
        background: var(--panel);
        border: 1px solid var(--border);
        border-radius: 8px;
        overflow-x: auto;
        font: 0.875rem/1.7 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        color: var(--ink);
      }
      footer { border-top: 1px solid var(--border); padding-top: 1.25rem; color: var(--ink-3); font-size: 0.875rem; }
      footer a { color: var(--accent); text-decoration: none; }
      footer a:hover { text-decoration: underline; }
      @media (max-width: 30rem) {
        main { padding: 3rem 1.15rem 4rem; }
        .mark { font-size: 2.25rem; }
      }
    </style>
  </head>
  <body>
    <main>
      <h1 class="mark">gadak</h1>
      <p class="tagline">Follow the thread.</p>

      <p class="lede">
        Your Jira and Confluence, mirrored into one local SQLite file. Search it,
        query it in <code>SQL</code>, point your coding agent at it. Reads never
        touch the network, and nothing about your tracker leaves this machine.
      </p>

      <div class="doors">
        <a class="door primary" href="${demoBase}">
          <span class="title">▶&nbsp; Open the live demo</span>
          <span class="sub">The real UI over a scrubbed sample mirror. No install, no account.</span>
        </a>
        <a class="door" href="${backlogBase}">
          <span class="title">Public backlog</span>
          <span class="sub">gadak's own issues, published from gadak — every commit cites one.</span>
        </a>
        <a class="door" href="https://github.com/midagedev/gadak">
          <span class="title">Source and releases</span>
          <span class="sub">Apache-2.0 on GitHub. macOS app, and a CLI for Linux and Windows.</span>
        </a>
      </div>

      <h2>Install</h2>
      <pre>brew install --cask midagedev/tap/gadak   # macOS app (CLI included)
brew install midagedev/tap/gadak-cli     # CLI only (macOS, Linux)

gadak init &amp;&amp; gadak sync &amp;&amp; gadak serve</pre>

      <footer>
        Built by <a href="https://github.com/midagedev">midagedev</a> ·
        <a href="https://github.com/midagedev/gadak">GitHub</a> ·
        <a href="https://github.com/midagedev/gadak/blob/main/SECURITY.md">Where the bytes go</a>
      </footer>
    </main>
  </body>
</html>
`
}
