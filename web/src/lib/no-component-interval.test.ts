/*
 * GDK-1584 recurrence gate: wall-clock ticks live in lib/clock.svelte.ts —
 * one interval for every relative-time surface. A component calling
 * setInterval( directly is a new private clock (IssueRow, FreshnessChip and
 * FavoritesNav each carried one; at --spacing-row 36px and OVERSCAN 8 that
 * is ~41 intervals for one 900px list) and fails here.
 *
 * The 2026-09-11 test-surface audit round found the sweep walked
 * web/src/components only, so the phone — the same app on a battery — was
 * never policed. The walk now covers the mobile component trees too
 * (mobile/src minus lib/, which is the store home exactly as web/src/lib
 * is; a store module owning its cadence is what this gate asks for, not a
 * finding).
 *
 * The four entries below are data polls, not wall clocks — pairing polls
 * and progress/report cadences cannot ride the shared 10s tick: a poll
 * owns its cadence. They predate the gate (or its mobile reach) and sit in
 * files outside the round that wrote it; the honest home for each is a
 * store module, the way lib/browse.svelte.ts and lib/terminal/sessions.svelte.ts
 * already own theirs. Until those rounds move them they are enumerated
 * with reasons, so the debt is visible in the gate itself — a new
 * component interval anywhere still fails.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const REPO = join(HERE, '..', '..', '..')

type Tree = { root: string; skip?: string[] }

/** Component trees on both surfaces. `skip` names sanctioned store homes. */
const TREES: Tree[] = [
  { root: 'web/src/components' },
  { root: 'mobile/src', skip: ['mobile/src/lib'] },
]

type Exception = { file: string; why: string }

/** Each suppresses every hit in its file — a poll moves wholesale or not at
 *  all, so per-line bookkeeping would only invite half-moves. Paths are
 *  repo-relative so the phone entries name themselves. */
const ALLOWED: Exception[] = [
  {
    file: 'web/src/components/shell/Onboarding.svelte',
    why: 'pairing-status poll at POLL_MS — a data cadence, not a wall clock; belongs in a store module',
  },
  {
    file: 'web/src/components/browse/BrowsePane.svelte',
    why: '1s progress report while a browse runs — a data cadence, not a wall clock; belongs in a store module',
  },
  {
    file: 'mobile/src/screens/Shell.svelte',
    why: 'roster poll at ROSTER_POLL_MS, only while the sheet is open (GDK-1497 A6) — a data cadence over a tailnet on a battery, not a wall clock; belongs in a store module',
  },
  {
    file: 'mobile/src/screens/PairingTab.svelte',
    why: 'import.meta.env.DEV-only 1s viewport probe — a debug readout compiled out of production builds, not a wall clock',
  },
]

function intervalSites(tree: Tree): string[] {
  const rootAbs = join(REPO, tree.root)
  const out: string[] = []
  for (const e of readdirSync(rootAbs, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  )) {
    const p = join(rootAbs, e.name)
    if (e.isDirectory()) out.push(...intervalSites({ ...tree, root: relative(REPO, p) }))
    else if (e.name.endsWith('.svelte') || (e.name.endsWith('.ts') && !e.name.endsWith('.test.ts'))) {
      const lines = readFileSync(p, 'utf8').split('\n')
      lines.forEach((line, i) => {
        if (!line.includes('setInterval(')) return
        const rel = relative(REPO, p)
        if (tree.skip?.some((s) => rel === s || rel.startsWith(s + '/'))) return
        if (ALLOWED.some((a) => rel === a.file)) return
        out.push(`${rel}:${i + 1}: ${line.trim()}`)
      })
    }
  }
  return out
}

describe('GDK-1584 no component owns a clock', () => {
  test('no setInterval in any component tree outside the clock module', () => {
    const sites = TREES.flatMap(intervalSites)
    expect(sites.join('\n') || '(none)', 'wall-clock ticks belong to lib/clock.svelte.ts (GDK-1584); the sweep covers web and mobile component trees (the 2026-09-11 audit round)').toBe('(none)')
  })

  // The exceptions stay readable: an entry whose file no longer polls is a
  // claim about code that is gone (same discipline as
  // effect-assigns-state). Reading repo-relative paths also proves the
  // mobile entries reach a real file — a wrong path fails here, not
  // silently everywhere.
  test('every allowed poll file still has its interval', () => {
    for (const a of ALLOWED) {
      const src = readFileSync(join(REPO, a.file), 'utf8')
      expect(src.includes('setInterval('), `${a.file}: ${a.why}`).toBe(true)
    }
  })

  // A walk that misses a tree passes green forever — the audit round's
  // finding in its original form. Each tree must actually see component
  // files.
  test('the sweep reaches every component tree it names', () => {
    expect(countSvelte('web/src/components'), 'web components walk is empty').toBeGreaterThan(50)
    for (const sub of ['mobile/src/screens', 'mobile/src/ui']) {
      expect(countSvelte(sub), `${sub} walk is empty`).toBeGreaterThan(3)
    }
    expect(countSvelte('mobile/src/App.svelte'), 'mobile/src/App.svelte is not seen by the walk').toBe(1)
  })
})

function countSvelte(rel: string): number {
  const abs = join(REPO, rel)
  if (!statSync(abs).isDirectory()) return rel.endsWith('.svelte') ? 1 : 0
  let n = 0
  for (const e of readdirSync(abs, { withFileTypes: true })) {
    n += countSvelte(join(rel, e.name))
  }
  return n
}
