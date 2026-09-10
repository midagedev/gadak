import { mkdirSync, mkdtempSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-720: a browser spec that re-proves a pure function.
 *
 * `e2e/integrations.spec.ts` grew a `integrations failure modes` describe that
 * routed a fake install response and then asserted the *parser's* verdict —
 * a mid-stream `exit=0` is output, a stream with no sentinel is
 * "result unknown", exit 0 with the check still negative is not a check mark.
 * Every one of those judgements is a pure function pinned in
 * `web/src/lib/integrations.test.ts`, whose header already drew the line
 * ("the .svelte file only paints"). Running them again through Chromium cost
 * a browser context each and caught nothing the unit did not.
 *
 * Deleting them once does not close the class. The way the class arrives is
 * copying: someone reads a unit test, wants the same guarantee "end to end",
 * and pastes it into a spec — and the sentence describing the guarantee comes
 * along with the paste, because it is the part that was already right. So the
 * measurable trace of the copy is the title.
 *
 * This lint fails when an `e2e/*.spec.ts` test title matches a
 * `web/src/**\/*.test.ts` one, exactly or after normalisation (case, spacing
 * and punctuation dropped — a paste that got re-quoted or re-capitalised is
 * still a paste).
 *
 * It is a tripwire, not a proof: a re-proof that gets a fresh title walks past
 * it. The judgement it cannot make is in e2e/README.md, and the honest scope
 * is written there too. Nothing on this tree trips it today (measured
 * 2026-09-10: 499 e2e titles against 1205 distinct unit titles, zero pairs
 * exact or normalised), so the self-test below is what proves the lint can
 * still go red.
 *
 * The fix when it does go red is not to rename the title. It is to decide
 * which side owns the assertion: if the browser adds nothing (no paint, no
 * wiring across components, no navigation), the e2e case goes; if it does add
 * something, the title should say what — "…, and the banner sits over the
 * list" reads differently from the unit's sentence because it asserts
 * something different.
 *
 * demo/ hosted/ perf/ are other suites (playwright.config testIgnore), same
 * split as no-bare-timeout.unit.ts.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(HERE, '..')
const SKIP_DIRS = new Set(['demo', 'hosted', 'perf', '.tmp', 'node_modules'])

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else out.push(p)
  }
  return out
}

/**
 * Titles of `it(...)` / `test(...)` calls, with the file:line they sit on.
 * Deliberately line-oriented and quote-aware rather than a parse: the input is
 * this repo's own test files, and a title that is built at runtime (a template
 * with a `${}` in it) is not the paste this lint is looking for.
 */
export function testTitles(file: string): { title: string; at: string }[] {
  const lines = readFileSync(file, 'utf8').split('\n')
  const out: { title: string; at: string }[] = []
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(
      /^\s*(?:it|test)(?:\.(?:only|skip|fails|concurrent|sequential|serial|todo))?\s*\(\s*(['"`])((?:\\.|(?!\1).)*)\1/,
    )
    if (!m) continue
    if (m[1] === '`' && m[2].includes('${')) continue
    out.push({ title: m[2].replace(/\\(['"`])/g, '$1'), at: `${file}:${i + 1}` })
  }
  return out
}

/** Case, punctuation and spacing dropped — a re-quoted paste still matches. */
function normalize(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .trim()
}

export function duplicatedUnitTitles(root = ROOT): string[] {
  const e2eFiles = walk(join(root, 'e2e')).filter((p) => p.endsWith('.spec.ts'))
  const unitFiles = walk(join(root, 'web/src')).filter((p) => p.endsWith('.test.ts'))

  const owners = new Map<string, string[]>()
  for (const file of unitFiles) {
    for (const { title, at } of testTitles(file)) {
      const key = normalize(title)
      if (!key) continue
      const seen = owners.get(key)
      if (seen) seen.push(relative(root, at))
      else owners.set(key, [relative(root, at)])
    }
  }

  const hits: string[] = []
  for (const file of e2eFiles) {
    for (const { title, at } of testTitles(file)) {
      const seen = owners.get(normalize(title))
      if (!seen) continue
      hits.push(`${relative(root, at)}: "${title}"\n    already pinned by ${seen.join(', ')}`)
    }
  }
  return hits
}

test('no e2e spec re-proves a unit test under the same title', () => {
  const hits = duplicatedUnitTitles()
  expect(
    hits,
    'an e2e title matches a web/src unit title — decide which side owns the ' +
      'assertion (see the header of this file and e2e/README.md):\n' +
      hits.join('\n'),
  ).toEqual([])
})

test('the lint itself goes red on a copied title — case and punctuation included', () => {
  // The instrument, not the tree: nothing on this tree trips the check above,
  // so without this the first real duplicate would be the first time anyone
  // learned whether it works.
  const dir = mkdtempSync(join(tmpdir(), 'e2e-title-lint-'))
  const e2e = join(dir, 'e2e')
  const web = join(dir, 'web/src/lib')
  for (const d of [e2e, web]) mkdirSync(d, { recursive: true })
  writeFileSync(
    join(web, 'parser.test.ts'),
    "test('a mid-stream exit=0 is output, not a verdict', () => {})\n",
  )
  writeFileSync(
    join(e2e, 'copy.spec.ts'),
    "  test('A mid-stream exit=0 is output — not a verdict!', async () => {})\n",
  )
  const hits = duplicatedUnitTitles(dir)
  expect(hits).toHaveLength(1)
  expect(hits[0]).toContain('e2e/copy.spec.ts:1')
  expect(hits[0]).toContain('web/src/lib/parser.test.ts:1')
})
