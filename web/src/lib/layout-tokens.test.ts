import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  LAYOUT_SIDEBAR_PX,
  VIEWPORT_DOCKED_MIN_PX,
  layoutTokenStyle,
} from './viewport-regime'

/*
 * GDK-826: moved from e2e/narrow-clip.spec.ts ("docked track mins sum to
 * VIEWPORT_DOCKED_MIN_PX"). That test booted a browser to read four CSS
 * custom properties off one element — but the properties are an inline
 * style generated from the TS constants (layoutTokenStyle), so the whole
 * chain is ownable here:
 *
 *   viewport-regime.ts derives the token string from the constants whose sum
 *   viewport-regime.test.ts pins → App.svelte mounts it on
 *   [data-testid="issue-layout"] → app.css consumes var(--layout-*) without
 *   restating px. The painted geometry tests (trail at 740/1100) stay in
 *   narrow-clip.spec.ts.
 */

const HERE = dirname(fileURLToPath(import.meta.url))

function tokens(): Record<string, number> {
  return Object.fromEntries(
    layoutTokenStyle().split(';').map((decl) => {
      const [k, v] = decl.split(':')
      return [k, parseFloat(v)]
    }),
  )
}

test('layoutTokenStyle carries every track min and they sum to the docked floor', () => {
  const t = tokens()
  expect(t['--layout-sidebar']).toBe(LAYOUT_SIDEBAR_PX)
  expect(t['--layout-list-min']).toBe(LAYOUT_LIST_MIN_PX)
  expect(t['--layout-detail-min']).toBe(LAYOUT_DETAIL_MIN_PX)
  expect(t['--layout-docked-min'], 'CSS --layout-docked-min follows VIEWPORT_DOCKED_MIN_PX').toBe(
    VIEWPORT_DOCKED_MIN_PX,
  )
  expect(
    t['--layout-sidebar'] + t['--layout-list-min'] + t['--layout-detail-min'],
    `sidebar ${t['--layout-sidebar']} + list ${t['--layout-list-min']} + detail ${t['--layout-detail-min']} must equal docked ${t['--layout-docked-min']} (was 272+390+440=1102 vs 1100)`,
  ).toBe(t['--layout-docked-min'])
})

test('App mounts the token style on the issue-layout element', () => {
  const app = readFileSync(join(HERE, '../App.svelte'), 'utf8')
  const idx = app.indexOf('data-testid="issue-layout"')
  expect(idx, 'App must render [data-testid="issue-layout"]').toBeGreaterThan(-1)
  const tag = app.slice(Math.max(0, idx - 200), idx + 200)
  expect(tag, 'the layout element carries the generated token style').toContain(
    'style={layoutTokenStyle()}',
  )
})

test('app.css consumes the tokens and never restates their px', () => {
  // viewport-regime.ts's own rule ("CSS must not restate the px"): a px
  // literal in a --layout-* definition would fork the floor a second time.
  const css = readFileSync(join(HERE, '../app.css'), 'utf8')
  expect(css, 'the docked grid sizes its tracks from the tokens').toContain(
    'minmax(var(--layout-list-min), 1fr)',
  )
  expect(css).not.toMatch(/--layout-[a-z-]+:\s*\d/)
})
