/*
 * GDK-1052: `{INPUT} w-24` renders w-full anyway — w-full vs w-24 resolves
 * by Tailwind's emission order in the compiled sheet, not by class order in
 * the attribute. Width overrides must compose from INPUT_BARE / SELECT_BARE;
 * this scan keeps that rule closed for every .svelte under settings/.
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin (FieldsTab.test.ts). This file scans
 * the source the compiler emits.
 */
import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))

function svelteFiles(dir: string): string[] {
  const out: string[] = []
  for (const ent of readdirSync(dir, { withFileTypes: true })) {
    if (ent.isDirectory()) out.push(...svelteFiles(join(dir, ent.name)))
    else if (ent.name.endsWith('.svelte')) out.push(join(dir, ent.name))
  }
  return out
}

/**
 * A `w-*` width utility token (`w-24`, `w-[12rem]`). min-w-/max-w- compose
 * with w-full and are exempt: the `w-` there is preceded by a hyphen.
 */
const W_UTILITY = /(?<![\w:-])w-[^\s"]+/

describe('settings control width composes from the bare bases (GDK-1052)', () => {
  test('INPUT/SELECT are INPUT_BARE/SELECT_BARE plus exactly w-full', async () => {
    // Dynamic import so the source-scan test below still runs — and lists —
    // against a tree where the bare constants do not exist yet.
    const c = await import('./controls')
    expect(c.INPUT_BARE, 'INPUT_BARE must exist (width lives at the call site)').toBeTruthy()
    expect(c.SELECT_BARE, 'SELECT_BARE must exist (width lives at the call site)').toBeTruthy()
    expect(c.INPUT).toBe(`${c.INPUT_BARE} w-full`)
    expect(c.SELECT).toBe(`${c.SELECT_BARE} w-full`)
    expect(c.INPUT_BARE).not.toMatch(W_UTILITY)
    expect(c.SELECT_BARE).not.toMatch(W_UTILITY)
  })

  test('no class attribute pairs {INPUT}/{SELECT} with another w-* utility', () => {
    const offenders: string[] = []
    for (const path of svelteFiles(HERE)) {
      const src = readFileSync(path, 'utf8')
      for (const m of src.matchAll(/class="([^"]*)"/g)) {
        const value = m[1]
        if (!/\{(?:INPUT|SELECT)\}/.test(value)) continue
        const w = value.match(W_UTILITY)
        if (!w) continue
        const line = src.slice(0, m.index).split('\n').length
        offenders.push(
          `${path.slice(HERE.length + 1)}:${line}: "${value.trim()}" — ${w[0]} fights the constant's w-full`,
        )
      }
    }
    expect(
      offenders,
      'width overrides must compose from INPUT_BARE / SELECT_BARE, not fight the w-full inside INPUT/SELECT:\n' +
        offenders.join('\n'),
    ).toEqual([])
  })
})
