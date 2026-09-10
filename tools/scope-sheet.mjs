#!/usr/bin/env node
// scope-sheet — the phone's picker sheet as one table, from the shell.
//
//   node tools/scope-sheet.mjs [--snapshot <bootstrap.json>] [--anonymous]
//
// "Does the picker show two rows for one question?" used to cost a
// viewport-gate capture (~1 min, browser and all). This prints the sheet the
// picker would show — every scope's section, id, name and match count —
// using the phone's own domain code (mobile/src/lib/domain.ts, bundled with
// esbuild and imported here) over the committed demo snapshot, the same data
// path as the duplicate guard in mobile/src/lib/domain.test.ts (GDK-1542).
// It also answers the question outright: scopes whose selected key-sets are
// identical are named as duplicate questions (the recurrence shape of the
// retired "Assigned to me" row). The debugging layer of the sheet round
// (GDK-1554).
//
// Identity is Alex Kim (demo-alex), the demo identity with the most assigned
// rows; --anonymous drops it, which is the sheet an anonymous reader gets.
//
// Exit 0 = the sheet printed (duplicates, if any, are named in the output);
// 1 = duplicate questions found (for scripting); 2 = usage/harness error.
import { execFileSync } from 'node:child_process'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

const HEADER = `
  scope-sheet — the phone's picker sheet (id/name/count) in one command.

  node tools/scope-sheet.mjs [options]

  --snapshot <file>   bootstrap.json to read
                      (default mobile/public/demo/bootstrap.json)
  --anonymous         no identity: the sheet an anonymous reader gets
  -h, --help          this text
`

function usage(msg) {
  if (msg) console.error(`scope-sheet: ${msg}`)
  console.error(HEADER.trimStart())
  process.exit(2)
}

const args = process.argv.slice(2)
let snapshot = path.join(ROOT, 'mobile', 'public', 'demo', 'bootstrap.json')
let anonymous = false
for (let i = 0; i < args.length; i++) {
  const a = args[i]
  if (a === '-h' || a === '--help') {
    console.log(HEADER.trimStart())
    process.exit(0)
  } else if (a === '--snapshot') {
    snapshot = args[++i]
    if (!snapshot) usage('--snapshot needs a path')
  } else if (a === '--anonymous') {
    anonymous = true
  } else {
    usage(`unknown argument ${a}`)
  }
}

let boot
try {
  boot = JSON.parse(readFileSync(snapshot, 'utf8'))
} catch (e) {
  usage(`cannot read snapshot ${snapshot}: ${e.message}`)
}
if (!Array.isArray(boot.issues)) usage(`${snapshot} has no issues[] — not a bootstrap snapshot`)

// Alex Kim, the demo identity with the most assigned rows (domain.test.ts's
// choice for the same reason: the fixture must exercise the identity scopes).
const me = anonymous
  ? null
  : { email: 'demo@example.com', account_id: 'demo-alex', name: 'Alex Kim' }

const WORK = await mkdtemp(path.join(tmpdir(), 'gadak-scope-sheet.'))
try {
  const entry = path.join(WORK, 'entry.mjs')
  await writeFile(
    entry,
    `import { buildScopes, scopeIssues } from ${JSON.stringify(path.join(ROOT, 'mobile/src/lib/domain.ts'))}\n` +
      `import { initLocale, locale } from ${JSON.stringify(path.join(ROOT, 'mobile/src/lib/i18n.ts'))}\n` +
      `initLocale()\nexport { buildScopes, scopeIssues, locale }\n`,
  )
  const bundle = path.join(WORK, 'bundle.mjs')
  try {
    execFileSync(
      path.join(ROOT, 'node_modules', '.bin', 'esbuild'),
      ['--bundle', entry, '--format=esm', '--platform=node', `--outfile=${bundle}`, '--log-level=error'],
      { stdio: 'pipe' },
    )
  } catch (e) {
    console.error('scope-sheet: esbuild failed to bundle mobile/src/lib/domain.ts:')
    console.error(e.stdout?.toString() || e.message)
    process.exit(2)
  }
  const { buildScopes, scopeIssues, locale } = await import(pathToFileURL(bundle).href)

  const scopes = buildScopes([], [], me)
  const rows = scopes.map((s) => {
    const sel = scopeIssues(boot.issues, me, s)
    return {
      section: s.section,
      id: s.id,
      name: s.name,
      count: sel === null ? null : sel.length,
      disabled: s.unsupported?.length > 0,
      keys: sel === null ? null : sel.map((i) => i.issue_key).sort().join(','),
    }
  })

  console.log(`scope-sheet — ${path.relative(ROOT, snapshot)} (${boot.issues.length} issues, locale ${locale()}, ${anonymous ? 'anonymous' : 'demo-alex'})`)
  const cols = ['section', 'id', 'name', 'count']
  const table = rows.map((r) => ({
    section: r.section,
    id: r.id,
    name: r.name,
    count: r.count === null ? '—' : r.disabled ? `${r.count} ⛔` : String(r.count),
  }))
  const w = cols.map((c) => Math.max(c.length, ...table.map((r) => String(r[c]).length)) + 2)
  console.log(cols.map((c, i) => c.padEnd(w[i])).join(''))
  for (const r of table) console.log(cols.map((c, i) => String(r[c]).padEnd(w[i])).join(''))

  // The picker question, answered: identical non-empty selections are one
  // question asked twice. Empty selections are excluded on purpose — two
  // views that both select nothing on a fixture are not evidence of a
  // duplicate (the domain-test guard pins non-empty answers separately).
  const byKeys = new Map()
  for (const r of rows) {
    if (!r.keys || r.count === 0) continue
    const group = byKeys.get(r.keys) ?? []
    group.push(r)
    byKeys.set(r.keys, group)
  }
  const dups = [...byKeys.values()].filter((g) => g.length > 1)
  if (dups.length === 0) {
    console.log('no two rows ask the same question (identical non-empty selections: none)')
    process.exit(0)
  }
  for (const g of dups) {
    const names = g.map((r) => `${r.id} "${r.name}"`).join(' ≡ ')
    console.log(`duplicate question (${g[0].count} rows): ${names}`)
  }
  process.exit(1)
} finally {
  await rm(WORK, { recursive: true, force: true })
}
