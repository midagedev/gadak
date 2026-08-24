import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-766 / GDK-826: moved from e2e/narrow-clip.spec.ts — this walk had no
 * page fixture, only readFileSync assertions, so it never needed a browser
 * (dialog-shell.unit.ts / boot-theme.test.ts rail).
 *
 * Geometric overlap needs VITE_HOSTED_DEMO=1 (hosted suite). The CI set is
 * gadak serve, so this source walk is the FAIL-first that runs there:
 * `absolute right-3 top-0` is exactly the stacking the 800-banner crop
 * photographed. After the fix the root is relative/in-flow. The painted
 * geometry (copy and links boxes do not overlap at en 800) stays in
 * narrow-clip.spec.ts — that half is user-visible clipping.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const APP = join(HERE, '../App.svelte')
const HOSTED = join(HERE, '../components/shell/HostedLinks.svelte')

test('HostedLinks is in-flow (source: not absolute-stacked on the banner)', () => {
  const src = readFileSync(HOSTED, 'utf8')
  expect(
    src,
    'HostedLinks root must not be position:absolute at the banner corner (GDK-766 banner overlap)',
  ).not.toMatch(/class="absolute right-3 top-0/)
  const app = readFileSync(APP, 'utf8')
  const bannerIdx = app.indexOf('data-testid="demo-banner"')
  const linksIdx = app.indexOf('<HostedLinks')
  expect(bannerIdx, 'demo-banner must exist').toBeGreaterThan(-1)
  expect(linksIdx, 'HostedLinks must be mounted').toBeGreaterThan(-1)
  expect(
    linksIdx > bannerIdx,
    'HostedLinks must sit inside the banner markup, not as an absolute sibling in front of it',
  ).toBe(true)
})
