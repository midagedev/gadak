/*
 * GDK-248: the create dialog must send a catalog priority id, never a
 * hardcoded English display name.
 *
 * There is no component-mount harness (vitest is node, no svelte plugin),
 * so this reads the source the way SearchBox.test.ts / HistoryView.test.ts
 * do. Rendered catalog → POST payload is Playwright's job
 * (e2e/duedate.spec.ts).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'NewIssueDialog.svelte'), 'utf8')

/*
 * GDK-1474: the priority <option> contract below used to be a regex over the
 * literal markup, /<option value=\{p\.id\}>\{p\.name\}<\/option>/. That
 * matches one spelling: a wrapped line, an added class, or a space between
 * the tags fails a test about ids-versus-display-names. The claim — the
 * option's value is the catalog id and its label is the catalog name — is a
 * property of the element node, so it is read off the AST instead.
 *
 * The rest of this file stays source-reading on purpose: those assertions
 * are absences (no hardcoded 'Highest', no disabled= on submit, no fallback
 * GET) and the absence of a node is not something a walk can be pointed at.
 */
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

/** `p.id` for a non-computed member chain; undefined for anything else. */
function memberPath(node: unknown): string | undefined {
  if (!isNode(node)) return undefined
  if (node.type === 'Identifier') return String(node.name)
  if (node.type === 'MemberExpression' && node.computed === false) {
    const object = memberPath(node.object)
    const property = memberPath(node.property)
    return object && property ? `${object}.${property}` : undefined
  }
  return undefined
}

/** Every <option> as "value expression → text the option renders". */
function optionShapes(): { value: string | undefined; label: string }[] {
  const ast = parse(SRC, { modern: true, filename: 'NewIssueDialog.svelte' }) as unknown as {
    fragment: unknown
  }
  const shapes: { value: string | undefined; label: string }[] = []
  walkTemplate(ast.fragment, (node) => {
    if (node.type !== 'RegularElement' || node.name !== 'option') return
    const attr = (node.attributes as AnyNode[]).find(
      (a) => a.type === 'Attribute' && a.name === 'value',
    )
    const tag = attr?.value
    const value = isNode(tag) && tag.type === 'ExpressionTag' ? memberPath(tag.expression) : undefined
    const kids = ((node.fragment as AnyNode | undefined)?.nodes ?? []) as AnyNode[]
    const label = kids
      .map((c) =>
        c.type === 'ExpressionTag' ? `{${memberPath(c.expression) ?? '?'}}` : String(c.data ?? ''),
      )
      .join('')
    shapes.push({ value, label })
  })
  return shapes
}

describe('NewIssueDialog create payload (GDK-248)', () => {
  test('does not hardcode Jira default priority display names', () => {
    expect(SRC).not.toMatch(/const PRIORITIES/)
    expect(SRC).not.toMatch(/['"]Highest['"]/)
    expect(SRC).not.toMatch(/['"]Lowest['"]/)
  })

  test('reads GET priorities/ through the existing api wrapper', () => {
    expect(SRC).toMatch(/api\.getPriorities\s*\(/)
  })

  test('select values are catalog ids and labels are catalog names', () => {
    expect(optionShapes()).toContainEqual({ value: 'p.id', label: '{p.name}' })
  })

  test('submit sends priority_id and does not send a priority name key', () => {
    expect(SRC).toMatch(/priority_id:\s*priority\s*\|\|\s*undefined/)
    expect(SRC).not.toMatch(/^\s*priority:\s*priority/m)
  })
})

/*
 * GDK-302: a write that cannot run must not spin on Loading.
 *
 * vitest cannot mount the dialog (no svelte plugin). The request-not-made
 * assertion is Playwright's (e2e/duedate.spec.ts). This file holds the
 * wiring: one owner, two distinct terminal keys, no create-meta GET when
 * that owner already knows the answer.
 */
describe('NewIssueDialog create-meta gate (GDK-302)', () => {
  test('keeps no-credential and meta-failed as distinct catalog keys', () => {
    expect(SRC).toContain("t('write.needToken')")
    expect(SRC).toContain("t('write.metaFailed')")
    expect(SRC).toContain("t('common.setCredentials')")
    expect(SRC).toContain("t('common.retry')")
    expect(SRC).toContain("t('common.loading')")
  })

  test('asks write.configured and write.writeMetaLoaded before any create-meta GET', () => {
    expect(SRC).toContain('write.configured')
    expect(SRC).toContain('write.writeMetaLoaded')
    expect(SRC).toContain('write.credentialLoaded')
    expect(SRC).toMatch(/api\.getCreateMeta\s*\(/)
    // The empty-local-meta path must not be an unconditional fallback GET.
    expect(SRC).not.toMatch(
      /if \(write\.writeMetaProjects\.length\) \{[\s\S]*?\} else \{\s*void loadFallback\(\)/,
    )
  })

  test('exposes a free-when-unread dialog state marker', () => {
    expect(SRC).toMatch(/data-write-state=\{writeState\}/)
    expect(SRC).toMatch(/data-testid="new-issue-dialog"/)
  })
})

/*
 * GDK-254: required create-fields are advisory. The dialog still submits
 * when the extra-required warning is showing, and still submits when the
 * endpoint is missing — Playwright holds the live path (e2e/create-fields.spec.ts).
 */
describe('NewIssueDialog create fields (GDK-254)', () => {
  test('loads create fields by project key and issue type id', () => {
    expect(SRC).toMatch(/api\.getCreateFields\s*\(/)
    expect(SRC).toContain('issueTypeId')
    expect(SRC).not.toMatch(/getCreateFields\([^)]*\.name/)
  })

  test('classifies extra required fields through the shared helper', () => {
    expect(SRC).toContain('extraRequiredCreateFields')
    expect(SRC).toContain('isCreateFieldRequired')
    expect(SRC).toContain('CREATE_DIALOG_ALWAYS_SENT')
  })

  test('does not disable submit when extra required fields exist', () => {
    expect(SRC).not.toMatch(/disabled=\{submitting\s*\|\|/)
    expect(SRC).toMatch(/disabled=\{submitting\}/)
  })

  test('reuses the existing required-star token on known fields', () => {
    expect(SRC).toContain('text-status-reopen')
    expect(SRC).toContain('new-issue-required-warn')
    expect(SRC).toContain("t('write.createRequiresMore'")
  })
})
