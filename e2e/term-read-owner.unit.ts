import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1567: `translateToString(` — reading xterm's buffer — has one owner,
 * e2e/term-read.ts. Four forked copies of that walk lived in
 * demo/terminal-claude-demo, demo/terminal-demo, demo/hero-desk and
 * mobile/e2e/shell.spec.ts, and none of them stitched xterm's wrapped
 * continuations, which is exactly the 2026-08-30 CI-only failure class
 * (24-column prompt on the runner, short prompt locally). A fifth copy today
 * would re-import that fork, so this lint makes a new one red.
 *
 * Scope:
 *  - e2e/** (demo/ included — three of the four copies were there) and
 *    mobile/e2e/**: banned outside the owner, except ALLOWED_SINGLE_ROW
 *    below. Those two are same-frame geometry probes, not text readers: the
 *    buffer row and the .xterm-screen rect must land in the same evaluate to
 *    hit-test a column / place a physical click, so the read cannot be lifted
 *    into the owner without splitting the frame. An allowlisted file that
 *    stops calling translateToString fails too (vitest-project-routing
 *    precedent — an allowlist that outlives its entry is a lie).
 *  - web/src/**: allowed only inside web/src/lib/terminal/ — that is the
 *    product's own terminal module and the __gadakTerm provider; everywhere
 *    else in app code a buffer walk is as much a fork as in a spec.
 *
 * Comment lines are skipped (isComment): the owner's own header names the
 * call, and term-rows.unit.ts documents its padding behaviour in prose.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = join(HERE, '..')
const OWNER = 'e2e/term-read.ts'
const SKIP_DIRS = new Set(['.tmp', 'node_modules'])

const ALLOWED_SINGLE_ROW: Record<string, string> = {
  'e2e/terminal-link-focus.spec.ts':
    'one viewport row, hit-testing the column under a click (GDK-1186) — same-frame with the .xterm-screen rect',
  'e2e/demo/roundtrip.spec.ts':
    'viewport-row scan that computes where to physically click a key (media pipeline) — same-frame with the screen rect',
}

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (name.endsWith('.ts')) out.push(p)
  }
  return out
}

function isComment(line: string): boolean {
  const t = line.trim()
  return t.startsWith('//') || t.startsWith('*') || t.startsWith('/*')
}

/** Non-comment `translateToString(` hits as repo-relative "path:line". */
export function bufferReads(root: string): string[] {
  const hits: string[] = []
  for (const file of walk(root)) {
    // This lint's own matcher spells the call it bans.
    if (relative(ROOT, file) === 'e2e/term-read-owner.unit.ts') continue
    const lines = readFileSync(file, 'utf8').split('\n')
    lines.forEach((line, i) => {
      if (!line.includes('translateToString(') || isComment(line)) return
      hits.push(`${relative(ROOT, file)}:${i + 1}`)
    })
  }
  return hits
}

test('translateToString lives only in the term-read owner (and the product terminal module)', () => {
  const byFile = (paths: string[]) =>
    [...new Set(paths.map((p) => p.split(':')[0]))].sort()

  const specHits = [...bufferReads(join(ROOT, 'e2e')), ...bufferReads(join(ROOT, 'mobile/e2e'))]
  const specFiles = byFile(specHits)
  const offenders = specFiles.filter((f) => f !== OWNER && !(f in ALLOWED_SINGLE_ROW))
  expect(
    offenders,
    `forked terminal-buffer walks — import readTerm from the owner (e2e/term-read.ts):\n${offenders.join('\n')}`,
  ).toEqual([])

  const stale = Object.keys(ALLOWED_SINGLE_ROW).filter((f) => !specFiles.includes(f))
  expect(
    stale,
    `ALLOWED_SINGLE_ROW entries that no longer call translateToString — the geometry probe moved or died; drop the entry:\n${stale.join('\n')}`,
  ).toEqual([])

  const webHits = bufferReads(join(ROOT, 'web/src')).filter((p) => !p.startsWith('web/src/lib/terminal/'))
  expect(
    webHits,
    'web/src buffer walks outside lib/terminal/ — app code reading the buffer directly is the same fork',
  ).toEqual([])
})
