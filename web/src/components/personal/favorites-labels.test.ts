/*
 * GDK-739: a timestamp slot must hold a time, and one word must mean one
 * thing. Navigation vocabulary leaked into FavoritesNav — a screen name in
 * the unviewed-favourite time column, and "All" as both the history-open
 * verb and the history-screen tab filter.
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (FeaturesTab.test.ts / HistoryView.test.ts). What this file can prove is
 * the source the compiler emits: those two keys stay gone, and the only
 * history.* key this nav may name is the destination screen's own title.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const FAVORITES_NAV = join(HERE, 'FavoritesNav.svelte')
const MESSAGES = join(HERE, '../../lib/i18n/messages')

function catalogSource(): string {
  return readdirSync(MESSAGES)
    .filter((n) => n.endsWith('.ts'))
    .map((n) => readFileSync(join(MESSAGES, n), 'utf8'))
    .join('\n')
}

function historyKeysNamed(src: string): Set<string> {
  const keys = new Set<string>()
  for (const match of src.matchAll(/\bt\('(history\.[^']+)'\)/g)) {
    keys.add(match[1])
  }
  return keys
}

describe('GDK-739 favorites labels stay in their slot', () => {
  const nav = readFileSync(FAVORITES_NAV, 'utf8')
  const catalog = catalogSource()

  test('FavoritesNav does not reference personal.recentHistory or history.openAll', () => {
    expect(nav).not.toMatch(/personal\.recentHistory/)
    expect(nav).not.toMatch(/history\.openAll/)
  })

  test('neither retired key is defined in the catalog', () => {
    expect(catalog).not.toContain("'personal.recentHistory'")
    expect(catalog).not.toContain("'history.openAll'")
  })

  test('the only history.* key FavoritesNav may name is history.title', () => {
    expect(historyKeysNamed(nav)).toEqual(new Set(['history.title']))
  })
})
