#!/usr/bin/env node
// adf-render — an issue key's stored ADF, through the real renderer, as HTML.
//
//   node tools/adf-render.mjs <issue-key> [--db <mirror.db>] [--comments-only]
//
// During the attachment-chip round (GDK-1505) the question "what does the
// stored comment ADF actually render to?" was answered by a hand-built probe
// that has since been thrown away twice. This is it, promoted: it bundles
// web/src/lib/adf.ts (a pure module since the phone shares it, GDK-1497)
// with esbuild, imports it in node, and prints renderAdf()'s HTML for the
// issue's description and each comment — no phone, no browser, no fixture
// regeneration. apiBase is left at its default (undefined), so attachment
// URLs are validated exactly as a surface with no API base validates them:
// the chip path, never an <img>.
//
// The bundle is written to a mktemp dir outside the tree and imported with
// pathToFileURL — the web source is read, never executed in place.
//
// Exit 0 = rendered; 1 = the key has no ADF anywhere / renderAdf returned
// '' for everything it was handed; 2 = usage or harness error (missing
// sqlite3, bad key shape, unreadable db, bundle failure).
import { execFileSync } from 'node:child_process'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

const HEADER = `
  adf-render — render a stored issue's ADF with the real web renderer.

  node tools/adf-render.mjs <issue-key> [options]

  <issue-key>            e.g. NMB-110 (the demo fixture's keys)
  --db <mirror.db>       mirror to read (default examples/demo.db)
  --comments-only        skip the description, render comments only
  --desc-only            render the description only
  -h, --help             this text

  The renderer is web/src/lib/adf.ts itself, bundled by esbuild and imported
  here — locale initializes to 'en', apiBase stays at its default. Output is
  the HTML string renderAdf() returns, one block per description/comment.
`

function usage(msg) {
  if (msg) console.error(`adf-render: ${msg}`)
  console.error(HEADER.trimStart())
  process.exit(2)
}

const args = process.argv.slice(2)
let key = null
let db = path.join(ROOT, 'examples', 'demo.db')
let wantComments = true
let wantDesc = true
for (let i = 0; i < args.length; i++) {
  const a = args[i]
  if (a === '-h' || a === '--help') {
    console.log(HEADER.trimStart())
    process.exit(0)
  } else if (a === '--db') {
    db = args[++i]
    if (!db) usage('--db needs a path')
  } else if (a === '--comments-only') {
    wantDesc = false
  } else if (a === '--desc-only') {
    wantComments = false
  } else if (a.startsWith('-')) {
    usage(`unknown option ${a}`)
  } else if (key === null) {
    key = a
  } else {
    usage(`unexpected argument ${a}`)
  }
}
if (key === null) usage('an issue key is required')
if (!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(key)) {
  usage(`key ${JSON.stringify(key)} is not an issue-key shape (the db is queried with it)`)
}

// ── read the stored ADF ────────────────────────────────────────────────────
const q = (sql) =>
  JSON.parse(execFileSync('sqlite3', ['-json', db, sql], { encoding: 'utf8' }) || '[]')

let issue
try {
  issue = q(
    `select i."key" as key, i.description_adf as description_adf, ` +
      `(select count(*) from comments c where c.item_id = i.item_id) as n_comments ` +
      `from issues i where i."key" = '${key}'`,
  )[0]
} catch (e) {
  usage(`cannot read ${db}: ${e.message}`)
}
if (!issue) usage(`no issue ${key} in ${db}`)

const comments = q(
  `select c.author as author, c.created_at as created_at, c.body_adf as body_adf, ` +
    `c.body_text as body_text from comments c join issues i on i.item_id = c.item_id ` +
    `where i."key" = '${key}' order by c.created_at, c.id`,
)

// ── bundle the real renderer and import it ─────────────────────────────────
const WORK = await mkdtemp(path.join(tmpdir(), 'gadak-adf-render.'))
try {
  // initLocale() is the one boot step the web does that node would not: it
  // settles t() to a concrete catalog ('en' here — localStorage and
  // navigator are both absent, which detectLocale already treats as "en").
  const entry = path.join(WORK, 'entry.mjs')
  await writeFile(
    entry,
    `import { renderAdf } from ${JSON.stringify(path.join(ROOT, 'web/src/lib/adf.ts'))}\n` +
      `import { initLocale, locale } from ${JSON.stringify(path.join(ROOT, 'web/src/lib/i18n/index.ts'))}\n` +
      `initLocale()\nexport { renderAdf, locale }\n`,
  )
  const bundle = path.join(WORK, 'bundle.mjs')
  try {
    execFileSync(
      path.join(ROOT, 'node_modules', '.bin', 'esbuild'),
      ['--bundle', entry, '--format=esm', '--platform=node', `--outfile=${bundle}`, '--log-level=error'],
      { stdio: 'pipe' },
    )
  } catch (e) {
    console.error('adf-render: esbuild failed to bundle web/src/lib/adf.ts:')
    console.error(e.stdout?.toString() || e.message)
    process.exit(2)
  }
  const { renderAdf, locale } = await import(pathToFileURL(bundle).href)

  let rendered = 0
  const emit = (label, adfJson) => {
    const doc = adfJson ? JSON.parse(adfJson) : null
    const html = renderAdf(doc, { issueKey: key })
    console.log(`── ${label} (${locale()})`)
    console.log(html || '(renderAdf returned "" — plain-text fallback or empty ADF)')
    if (html) rendered++
  }

  if (wantDesc) emit(`${key} description`, issue.description_adf)
  if (wantComments) {
    if (comments.length === 0) console.log(`── ${key} has no comments`)
    comments.forEach((c, i) => {
      const who = `${c.author ?? 'unknown author'} · ${c.created_at ?? ''}`.trim()
      emit(`${key} comment ${i + 1} — ${who}`, c.body_adf)
    })
  }
  if (rendered === 0) {
    console.error(`adf-render: nothing rendered for ${key} (no ADF stored, or every render returned '')`)
    process.exit(1)
  }
  process.exit(0)
} finally {
  await rm(WORK, { recursive: true, force: true })
}
