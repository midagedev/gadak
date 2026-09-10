import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-728: a control the mouse reveals on hover must appear for the
 * keyboard too. The team view's delete (rare × irreversible) and the Jira
 * filter's "open origin" link sat at opacity-0 until a group hover — a
 * Tab-reaching finger saw an invisible focused button. The favorites star
 * already carried the reveal (group-focus-within:opacity-100,
 * FavoritesNav) and the section grip used group-focus-visible; this pins
 * the same reveal on the two SidebarNav controls so the asymmetry cannot
 * quietly return.
 *
 * Source sweep rather than a mounted test: the contract is the class on
 * the rendered element, and the element lives behind armed-state logic the
 * unit project cannot mount (runes component).
 */
const HERE = dirname(fileURLToPath(import.meta.url))

const NAV = readFileSync(join(HERE, 'SidebarNav.svelte'), 'utf8')

/** The button (or link) carrying `testid`, as a source slice. */
function controlOf(source: string, testid: string): string {
  const at = source.indexOf(`data-testid="${testid}"`)
  expect(at, `${testid} must exist in the source`).toBeGreaterThan(-1)
  // The tag opens before the testid attribute; capture the whole tag.
  const open = source.lastIndexOf('<', at)
  const end = source.indexOf('>', at)
  return source.slice(open, end + 1)
}

test('the team view delete reveals on focus, not only on hover (GDK-728)', () => {
  const tag = controlOf(NAV, 'sidebar-view-delete')
  expect(tag, 'hover reveals it').toContain('group-hover:opacity-100')
  expect(tag, 'and so does keyboard focus inside the row').toContain(
    'group-focus-within:opacity-100',
  )
})

test('the Jira filter origin link reveals on focus too (GDK-728)', () => {
  const tag = controlOf(NAV, 'sidebar-jira-filter-open')
  expect(tag, 'hover reveals it').toContain('group-hover:opacity-100')
  expect(tag, 'and so does keyboard focus inside the row').toContain(
    'group-focus-within:opacity-100',
  )
})
