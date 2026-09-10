/*
 * GDK-733 / GDK-734 / GDK-1093 — three keyboard-and-legibility contracts that
 * are otherwise invisible to the type checker.
 *
 * The helpers are pure, so they are asserted directly. The three call sites
 * are asserted against their source the way workspace.test.ts does it: the
 * rendered result is Playwright's job, but "this row still owns the gesture"
 * and "this path no longer breaks mid-word" are one-token edits that a
 * refactor can silently undo with every other gate staying green.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import { altReorderDelta, stepTarget } from './reorder'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const read = (rel: string) => readFileSync(join(WEB_SRC, rel), 'utf8')

const key = (init: Partial<KeyboardEvent>) => init as KeyboardEvent

describe('altReorderDelta', () => {
  test('Alt+Arrow gives a step, everything else gives null', () => {
    expect(altReorderDelta(key({ altKey: true, key: 'ArrowUp' }))).toBe(-1)
    expect(altReorderDelta(key({ altKey: true, key: 'ArrowDown' }))).toBe(1)
    expect(altReorderDelta(key({ altKey: false, key: 'ArrowUp' }))).toBeNull()
    expect(altReorderDelta(key({ altKey: true, key: 'ArrowLeft' }))).toBeNull()
    expect(altReorderDelta(key({ altKey: true, key: 'j' }))).toBeNull()
  })
})

describe('stepTarget', () => {
  const list = ['a', 'b', 'c'] as const

  test('returns the neighbour in the step direction', () => {
    expect(stepTarget(list, 'b', -1)).toBe('a')
    expect(stepTarget(list, 'b', 1)).toBe('c')
  })

  test('does not wrap at either edge', () => {
    expect(stepTarget(list, 'a', -1)).toBeNull()
    expect(stepTarget(list, 'c', 1)).toBeNull()
  })

  test('an id outside the visible list is not a step', () => {
    expect(stepTarget(list, 'z' as unknown as 'a', 1)).toBeNull()
  })
})

describe('the two reorderable lists answer one gesture (GDK-733)', () => {
  const favorites = read('components/personal/FavoritesNav.svelte')
  const section = read('components/sidebar/SidebarSection.svelte')

  test('both call sites route their keydown through altReorderDelta', () => {
    for (const src of [favorites, section]) expect(src).toContain('altReorderDelta(')
  })

  test('both advertise the same shortcut to assistive tech', () => {
    for (const src of [favorites, section])
      expect(src).toContain('aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"')
  })

  test('neither store hand-rolls the step arithmetic any more', () => {
    for (const rel of ['stores/favorites.svelte.ts', 'stores/sidebar-sections.svelte.ts'])
      expect(read(rel)).toContain('stepTarget(')
  })
})

describe('hover-only controls stay reachable by keyboard (GDK-734)', () => {
  test('the comment reply button reveals itself on focus, not only on hover', () => {
    const src = read('components/detail/CommentList.svelte')
    const reply = src.slice(src.indexOf('onclick={() => reply(c)}') - 400)
    expect(reply).toContain('group-hover:opacity-100')
    expect(reply).toContain('focus-within:opacity-100')
  })
})

describe('paths wrap at their separators (GDK-1093 C-4)', () => {
  test('no filesystem path in the Sync tab breaks mid-token', () => {
    const src = read('components/settings/RuntimeMirror.svelte')
    // break-all splits `config.json` into `config.j` + `son`; break-words
    // takes the slash the path already offers.
    for (const line of src.split('\n')) {
      if (!line.includes('break-all')) continue
      expect(line, `break-all on a path line: ${line.trim()}`).not.toContain('font-mono')
    }
  })
})
