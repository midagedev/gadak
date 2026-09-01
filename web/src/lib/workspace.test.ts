/*
 * GDK-237: workspaceKind is server-owned. The badge and the create-command
 * copy key off isLocalOrigin() in workspace.ts — never an empty site URL.
 *
 * Rendered visibility is Playwright's job (no component-mount harness).
 * These tests pin the decision function, the command string, and the
 * "no scattered === 'standalone'" wiring that is easy to lose again.
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const OWNER = join(HERE, 'workspace.ts')
const WEB_SRC = join(HERE, '..')
const COMPONENTS = join(WEB_SRC, 'components')
const STORES = join(WEB_SRC, 'stores')

const COMPARE_STANDALONE =
  /(?:===|==|!==|!=)\s*['"]local-origin['"]|['"]local-origin['"]\s*(?:===|==|!==|!=)|===\s*WORKSPACE_KIND_STANDALONE|WORKSPACE_KIND_STANDALONE\s*===/

function walk(dir: string, acc: string[] = []): string[] {
  for (const ent of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, ent.name)
    if (ent.isDirectory()) walk(p, acc)
    else if (/\.(ts|svelte)$/.test(ent.name) && !ent.name.endsWith('.test.ts')) acc.push(p)
  }
  return acc
}

describe('isLocalOrigin (server-owned, never inferred from site)', () => {
  test('the derived-value owner is workspace.ts', () => {
    expect(existsSync(OWNER), 'web/src/lib/workspace.ts must own isLocalOrigin').toBe(true)
    const src = readFileSync(OWNER, 'utf8')
    expect(src).toMatch(/export function isLocalOrigin/)
    expect(src).toMatch(COMPARE_STANDALONE)
  })

  test('local-origin document → badge on; connected / empty-site-connected → off', async () => {
    const { isLocalOrigin } = await import('./workspace')
    expect(isLocalOrigin({ workspaceKind: 'standalone' })).toBe(true)
    expect(isLocalOrigin({ workspaceKind: 'connected' })).toBe(false)
    // Hosted demo / older document: empty site is not evidence of localOrigin.
    expect(isLocalOrigin({ workspaceKind: 'connected', jiraBaseUrl: '' })).toBe(false)
    expect(isLocalOrigin({ jiraBaseUrl: '' })).toBe(false)
    expect(isLocalOrigin({ workspaceKind: 'local', jiraBaseUrl: '' })).toBe(false)
    expect(isLocalOrigin({})).toBe(false)
    expect(isLocalOrigin(null)).toBe(false)
    expect(isLocalOrigin(undefined)).toBe(false)
  })
})

describe('local-origin init command', () => {
  test('names --profile <name> so the slot the user fills is visible', async () => {
    const { STANDALONE_INIT_COMMAND } = await import('./workspace')
    expect(STANDALONE_INIT_COMMAND).toBe('gadak --workspace <name> init --local')
  })
})

describe('no scattered local-origin comparisons', () => {
  test('web/src/{components,stores} never compare a local-origin literal', () => {
    const hits: string[] = []
    for (const file of [...walk(COMPONENTS), ...walk(STORES)]) {
      const src = readFileSync(file, 'utf8')
      if (COMPARE_STANDALONE.test(src)) hits.push(file.slice(WEB_SRC.length + 1))
    }
    expect(hits, `comparisons must live in workspace.ts, found in: ${hits.join(', ')}`).toEqual(
      [],
    )
  })

  test('config.ts delegates; it does not own the comparison', () => {
    const src = readFileSync(join(HERE, 'config.ts'), 'utf8')
    expect(src).not.toMatch(COMPARE_STANDALONE)
    expect(src).toMatch(/from ['"]\.\/workspace['"]/)
  })
})

describe('badge surfaces wire through isLocalOrigin', () => {
  test('RuntimeMirror shows the badge only via isLocalOrigin and labels it', () => {
    const src = readFileSync(join(COMPONENTS, 'settings/RuntimeMirror.svelte'), 'utf8')
    expect(src).toMatch(/isLocalOrigin\(/)
    expect(src).toContain('data-testid="workspace-kind"')
    expect(src).toMatch(/aria-label/)
    expect(src).toContain('local-origin-init-command')
    expect(src).toContain('local-origin-init-copy')
  })

  test('SidebarNav badges the current workspace name via isLocalOrigin', () => {
    const src = readFileSync(join(COMPONENTS, 'sidebar/SidebarNav.svelte'), 'utf8')
    expect(src).toMatch(/isLocalOrigin\(/)
    expect(src).toContain('data-testid="workspace-kind"')
    expect(src).toContain('local-origin-create')
  })
})
