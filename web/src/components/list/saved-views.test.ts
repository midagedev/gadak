/*
 * GDK-437: saved views are one kind. The product picks the store (server
 * when one exists, this browser in the hosted demo); the user is not asked.
 *
 * vitest is node, no svelte plugin — importing .svelte fails
 * (SearchBox.test.ts). These assertions read the source and the catalog.
 * Rendered Enter→server and the leftover-absorb path are Playwright's
 * (e2e/view-cross-device.spec.ts).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import { en, ja, ko } from '../../lib/i18n/catalog'

const HERE = dirname(fileURLToPath(import.meta.url))
const FILTER_BAR = join(HERE, 'ViewSettingsMenu.svelte')
const SIDEBAR_NAV = join(HERE, '../sidebar/SidebarNav.svelte')
const VIEWS_STORE = join(HERE, '../../stores/views.svelte.ts')

const filterBarSrc = readFileSync(FILTER_BAR, 'utf8')
const sidebarNavSrc = readFileSync(SIDEBAR_NAV, 'utf8')
const viewsStoreSrc = readFileSync(VIEWS_STORE, 'utf8')

type AnyNode = { type: string } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

const CHILD_KEYS = [
  'fragment',
  'nodes',
  'consequent',
  'alternate',
  'body',
  'pending',
  'then',
  'catch',
  'fallback',
] as const

function walkTemplate(node: unknown, visit: (n: AnyNode) => void): void {
  if (Array.isArray(node)) {
    for (const child of node) walkTemplate(child, visit)
    return
  }
  if (!isNode(node)) return
  visit(node)
  for (const key of CHILD_KEYS) walkTemplate(node[key], visit)
}

function attributesOf(element: AnyNode): AnyNode[] {
  return Array.isArray(element.attributes) ? element.attributes.filter(isNode) : []
}

function testId(element: AnyNode): string | undefined {
  const attr = attributesOf(element).find((a) => a.type === 'Attribute' && a.name === 'data-testid')
  if (!attr || !Array.isArray(attr.value)) return undefined
  const text = attr.value.filter(isNode).find((v) => v.type === 'Text')
  return typeof text?.data === 'string' ? text.data : undefined
}

function span(node: AnyNode): { start: number; end: number } {
  const { start, end } = node
  if (typeof start !== 'number' || typeof end !== 'number') {
    throw new Error(`${node.type} node carries no source range`)
  }
  return { start, end }
}

function contains(outer: AnyNode, inner: AnyNode): boolean {
  const o = span(outer)
  const i = span(inner)
  return o.start <= i.start && i.end <= o.end
}

const filterBarNodes: AnyNode[] = []
walkTemplate(
  (parse(filterBarSrc, { modern: true, filename: 'ViewSettingsMenu.svelte' }) as unknown as AnyNode)
    .fragment,
  (n) => filterBarNodes.push(n),
)

const savePopover = filterBarNodes.find(
  (n) => n.type === 'RegularElement' && testId(n) === 'filter-save-popover',
)

/*
 * Keys the save popover and sidebar used to offer a personal-vs-team choice.
 * Recurrence: if any of these reappear in the catalog, the UI can grow a
 * second save path again without this file noticing a string change.
 */
const REMOVED_SCOPE_KEYS = [
  'filter.saveTeam',
  'filter.savePersonal',
  'filter.saveServerHint',
  'filter.saveLocalHint',
  'sidebar.teamViews',
  'sidebar.viewOwner',
] as const

/*
 * sidebar.serverSettings lists the settings dialog's surfaces
 * ("projects, features, teams, field map") — that "teams" is teamconfig
 * (gadak team export/import), not saved-view sharing. Allowlisted 2026-08-23
 * so this gate stays on the saved-view vocabulary the issue named.
 */
/*
 * sidebar.stanceTeam ("Team flow") is the steward-stance heading over the
 * built-in views (THEORY.md "Two stances"), not a saved-view scope — no save
 * path hangs off it. Allowlisted 2026-09-06 (r2-views review) for the same
 * reason as serverSettings: the word is about who the view serves, not where
 * a saved view is stored.
 */
const TEAM_VOCAB_ALLOWED = new Set(['sidebar.serverSettings', 'sidebar.stanceTeam'])

const TEAM_VOCAB_RE = /\bteams?\b|팀|チーム/i

function teamVocabHits(locale: string, table: Record<string, string>): string[] {
  const hits: string[] = []
  for (const [key, value] of Object.entries(table)) {
    if (!key.startsWith('filter.') && !key.startsWith('sidebar.')) continue
    if (TEAM_VOCAB_ALLOWED.has(key)) continue
    if (TEAM_VOCAB_RE.test(value)) hits.push(`${locale}.${key}=${JSON.stringify(value)}`)
  }
  return hits
}

describe('GDK-437 save popover is one action (in the Display menu since GDK-1343)', () => {
  test('the popover exists and holds exactly one button', () => {
    expect(savePopover, 'no [data-testid="filter-save-popover"] in ViewSettingsMenu.svelte').toBeDefined()
    const buttons = filterBarNodes.filter(
      (n) =>
        n.type === 'RegularElement' &&
        n.name === 'button' &&
        contains(savePopover as AnyNode, n),
    )
    expect(
      buttons,
      'save popover must not offer a personal/team (or any other) scope choice',
    ).toHaveLength(1)
    expect(testId(buttons[0])).toBe('filter-save-view')
  })

  test('Enter and the button call the same no-scope doSave', () => {
    expect(filterBarSrc).toMatch(/async function doSave\(\)/)
    expect(filterBarSrc).toMatch(/e\.key === 'Enter' && doSave\(\)/)
    expect(filterBarSrc).toMatch(/onclick=\{\(\) => doSave\(\)\}/)
    expect(filterBarSrc).not.toMatch(/doSave\('team'\)/)
    expect(filterBarSrc).not.toMatch(/doSave\('personal'\)/)
    expect(filterBarSrc).not.toMatch(/defaultScope/)
  })

  test('scope-choice catalog keys are not rendered', () => {
    for (const key of [
      'filter.saveTeam',
      'filter.savePersonal',
      'filter.saveServerHint',
      'filter.saveLocalHint',
    ]) {
      expect(filterBarSrc, key).not.toContain(`t('${key}')`)
    }
    expect(filterBarSrc).toContain("t('filter.saveAsView')")
    expect(filterBarSrc).toContain("t('filter.saveDemoLocal')")
    expect(filterBarSrc).toContain("t('filter.saveServerFailed')")
  })
})

describe('GDK-437 catalog has no saved-view team vocabulary', () => {
  test('removed scope-choice keys are gone', () => {
    const present = REMOVED_SCOPE_KEYS.filter((k) => k in en)
    expect(present, present.join('\n')).toEqual([])
  })

  test('filter.* / sidebar.* strings do not say team', () => {
    const hits = [
      ...teamVocabHits('en', en),
      ...teamVocabHits('ko', ko),
      ...teamVocabHits('ja', ja),
    ]
    expect(hits, hits.join('\n')).toEqual([])
  })
})

describe('GDK-437 sidebar lists one views section', () => {
  test('does not render a team section, owner suffix, or owner tooltip', () => {
    expect(sidebarNavSrc).not.toContain("t('sidebar.teamViews')")
    expect(sidebarNavSrc).not.toContain("t('sidebar.viewOwner')")
    expect(sidebarNavSrc).not.toMatch(/id === 'team'/)
    expect(sidebarNavSrc).toContain("t('sidebar.myViews')")
    expect(sidebarNavSrc).toContain('data-view-storage')
  })
})

describe('GDK-437 absorb leftover local rows every boot', () => {
  test('does not skip because the absorb flag was already written', () => {
    expect(
      viewsStoreSrc,
      'one-shot guard skips leftovers after the first boot',
    ).not.toMatch(/if \(readAbsorbedIds\(\) !== null\) return null/)
    expect(viewsStoreSrc).toMatch(/#raw\.filter\(\(v\) => !this\.#absorbed\.has\(v\.id\)\)/)
    expect(viewsStoreSrc).toContain('isHostedDemo()')
    expect(viewsStoreSrc).toMatch(/localStorage is never cleared/)
  })
})
