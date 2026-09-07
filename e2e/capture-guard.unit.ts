import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1570: every screenshot a CI spec takes is one a round asked for.
 *
 * The release audit found the opposite discipline: capture-only specs ran
 * unconditionally in CI (nobody consumed the PNGs), and behavior specs wrote
 * captures inline on every run — machine-local paths like /tmp/gadak-865c or
 * a repo scratch default. The contract this lint pins is the one
 * e2e/my-work.spec.ts established: a capture happens only when a round names
 * a directory through an env var, in one of two shapes —
 *
 *   skip form    the capture-only test() begins with
 *                test.skip(!process.env.X_SHOT_DIR, 'capture-only; …')
 *   helper form  the .screenshot( call sits in a helper whose prelude reads
 *                process.env.X_SHOT_DIR and returns when unset — the read is
 *                within GUARD_WINDOW lines above the call
 *
 * Scope mirrors what CI actually runs: e2e/** minus the root config's
 * testIgnore dirs (demo/, hosted/, perf/ never run in CI, so their captures
 * are round-local by construction) plus mobile/e2e/** (the Mobile job's
 * Playwright set).
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = join(HERE, '..')

/** Same dirs the root playwright config's testIgnore keeps out of CI. */
const SKIP_DIRS = new Set(['.tmp', 'node_modules', 'demo', 'hosted', 'perf'])

/** How far above a .screenshot( call the helper's env read may sit. */
const GUARD_WINDOW = 8

/**
 * Files whose captures are still ungated, each with the reason it is fenced
 * here rather than fixed in the round that wrote this lint. The fence is not
 * forever: an entry whose file no longer trips the lint is stale and fails
 * the second test below, so the list can only shrink.
 */
const PENDING: Record<string, string> = {}

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

const TEST_START = /^\s*test\s*\(|^\s*test\.skip\s*\(/
const SKIP_GUARD = /test\.skip\(!process\.env\.[A-Z0-9_]+_SHOT_DIR/
const ENV_GUARD = /process\.env\.[A-Z0-9_]+_SHOT_DIR/

/** Unguarded `.screenshot(` sites as repo-relative "path:line". */
export function unguardedCaptures(root: string): string[] {
  const hits: string[] = []
  for (const file of walk(root)) {
    // This lint's own matcher spells the call it bans.
    if (relative(ROOT, file) === 'e2e/capture-guard.unit.ts') continue
    const lines = readFileSync(file, 'utf8').split('\n')
    lines.forEach((line, i) => {
      if (!line.includes('.screenshot(') || isComment(line)) return
      // Skip form: a test.skip(!env) guard between the enclosing test() and
      // the call.
      let start = 0
      for (let j = i; j >= 0; j--) if (TEST_START.test(lines[j])) { start = j; break }
      const block = lines.slice(start, i + 1).join('\n')
      if (SKIP_GUARD.test(block)) return
      // Helper form: the helper's env read just above the call.
      const prelude = lines.slice(Math.max(0, i - GUARD_WINDOW), i).join('\n')
      if (ENV_GUARD.test(prelude)) return
      hits.push(`${relative(ROOT, file)}:${i + 1}`)
    })
  }
  return hits
}

test('every CI spec capture is env-gated (skip guard or shot-dir helper)', () => {
  const hits = [...unguardedCaptures(join(ROOT, 'e2e')), ...unguardedCaptures(join(ROOT, 'mobile/e2e'))]
  const offenders = hits.filter((h) => !(h.split(':')[0] in PENDING))
  expect(
    offenders,
    `screenshots no round asked for — gate them (test.skip(!process.env.X_SHOT_DIR) for capture-only tests, an env-reading helper for behavior tests; see my-work.spec.ts / feed-days.spec.ts):\n${offenders.join('\n')}`,
  ).toEqual([])
})

test('the PENDING fence is live — no entry has been gated or deleted', () => {
  const hits = [...unguardedCaptures(join(ROOT, 'e2e')), ...unguardedCaptures(join(ROOT, 'mobile/e2e'))]
  const trippedFiles = new Set(hits.map((h) => h.split(':')[0]))
  const stale = Object.keys(PENDING).filter((f) => !trippedFiles.has(f))
  expect(
    stale,
    `PENDING fence entries that no longer trip — the capture was gated or the file died; drop the entry so the first test owns it again:\n${stale.join('\n')}`,
  ).toEqual([])
})
