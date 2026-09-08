/*
 * GDK-1584 recurrence gate: wall-clock ticks live in lib/clock.svelte.ts —
 * one interval for every relative-time surface. A component calling
 * setInterval( directly is a new private clock (IssueRow, FreshnessChip and
 * FavoritesNav each carried one; at --spacing-row 36px and OVERSCAN 8 that
 * is ~41 intervals for one 900px list) and fails here.
 *
 * The two entries below are data polls, not wall clocks — an onboarding
 * pairing poll and a 1s browse progress report — and
 * cannot ride the shared 10s tick: a poll owns its cadence. They predate
 * this gate and sit in files outside the round that wrote it
 * (shell/Onboarding, browse/BrowsePane); the honest home
 * for each is a store module, the way lib/browse.svelte.ts and
 * lib/terminal/sessions.svelte.ts already own theirs. Until those rounds
 * move them they are enumerated with reasons, so the debt is visible in the
 * gate itself — a new component interval anywhere still fails.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const COMPONENTS = join(HERE, '..', 'components')

type Exception = { file: string; why: string }

/** Each suppresses every hit in its file — a poll moves wholesale or not at
 *  all, so per-line bookkeeping would only invite half-moves. */
const ALLOWED: Exception[] = [
  {
    file: 'shell/Onboarding.svelte',
    why: 'pairing-status poll at POLL_MS — a data cadence, not a wall clock; belongs in a store module',
  },
  {
    file: 'browse/BrowsePane.svelte',
    why: '1s progress report while a browse runs — a data cadence, not a wall clock; belongs in a store module',
  },
]

function intervalSites(dir: string): string[] {
  const out: string[] = []
  for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  )) {
    const p = join(dir, e.name)
    if (e.isDirectory()) out.push(...intervalSites(p))
    else if (e.name.endsWith('.svelte') || e.name.endsWith('.ts')) {
      const lines = readFileSync(p, 'utf8').split('\n')
      lines.forEach((line, i) => {
        if (!line.includes('setInterval(')) return
        const rel = relative(COMPONENTS, p)
        if (ALLOWED.some((a) => rel === a.file)) return
        out.push(`${rel}:${i + 1}: ${line.trim()}`)
      })
    }
  }
  return out
}

describe('GDK-1584 no component owns a clock', () => {
  test('web/src/components has no setInterval outside the clock module', () => {
    const sites = intervalSites(COMPONENTS)
    expect(sites.join('\n') || '(none)', 'wall-clock ticks belong to lib/clock.svelte.ts (GDK-1584)').toBe('(none)')
  })

  // The exceptions stay readable: an entry whose file no longer polls is a
  // claim about code that is gone (same discipline as effect-assigns-state).
  test('every allowed poll file still has its interval', () => {
    for (const a of ALLOWED) {
      const src = readFileSync(join(COMPONENTS, a.file), 'utf8')
      expect(src.includes('setInterval('), `${a.file}: ${a.why}`).toBe(true)
    }
  })
})
