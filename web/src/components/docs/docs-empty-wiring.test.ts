/*
 * GDK-738: Documents body empty-hint must read the same six-cause table as
 * the sidebar. vitest is environment:'node' with no svelte plugin, so these
 * assertions scan source the way FeaturesTab.test.ts does.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '../..')
const DOCS_VIEW = join(HERE, 'DocsView.svelte')
const SPACE_DOCS_VIEW = join(HERE, 'SpaceDocsView.svelte')
const SIDEBAR_NAV = join(HERE, '../sidebar/SidebarNav.svelte')
const STORE = join(WEB_SRC, 'stores/docs-empty.svelte.ts')
const STORE_IMPORT = /from ['"][^'"]*stores\/docs-empty\.svelte['"]/
const CONFLUENCE_RUNS = /getSyncRuns\(\s*['"]confluence['"]\s*\)/

function collect(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      collect(p, out)
      continue
    }
    if (name.endsWith('.test.ts')) continue
    if (name.endsWith('.ts') || name.endsWith('.svelte')) out.push(p)
  }
  return out
}

describe('GDK-738 docs empty wiring', () => {
  const docsView = readFileSync(DOCS_VIEW, 'utf8')
  const spaceDocsView = readFileSync(SPACE_DOCS_VIEW, 'utf8')
  const sidebarNav = readFileSync(SIDEBAR_NAV, 'utf8')

  test('neither body view hardcodes the off / never-fetched hints', () => {
    for (const [path, src] of [
      [DOCS_VIEW, docsView],
      [SPACE_DOCS_VIEW, spaceDocsView],
    ] as const) {
      expect(src, path).not.toContain('sidebar.docsNoneHint')
      expect(src, path).not.toContain('sidebar.docsNotFetchedHint')
    }
  })

  test('both body views import the single docs-empty store', () => {
    expect(docsView, 'DocsView.svelte').toMatch(STORE_IMPORT)
    expect(spaceDocsView, 'SpaceDocsView.svelte').toMatch(STORE_IMPORT)
  })

  test("SidebarNav no longer calls getSyncRuns('confluence')", () => {
    expect(sidebarNav).not.toMatch(CONFLUENCE_RUNS)
  })

  test("exactly one web/src file calls getSyncRuns('confluence'), and it is the store", () => {
    const hits = collect(WEB_SRC).filter((p) => CONFLUENCE_RUNS.test(readFileSync(p, 'utf8')))
    expect(hits).toEqual([STORE])
  })
})
