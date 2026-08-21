/*
 * GDK-121: the detail-panel child section must list direct children of a
 * story (parent_key), not only epic_key descendants.
 *
 * There is no component-mount harness — vitest runs environment: 'node' and
 * vitest.config.ts loads no svelte plugin on the unit project, so importing a
 * .svelte file from a test fails outright (HistoryView.test.ts /
 * SearchBox.test.ts already record this). Rendered pixels are Playwright's
 * job (e2e/epics.spec.ts for the epic branch). Sub-task rows cannot be
 * asserted in e2e this round: the fixture has none (GDK-114 ③).
 *
 * What this file can prove is the part that is missing today and easy to
 * lose again: the children binding, the title key it chooses, and the
 * empty-list unrender gate. Those expressions are extracted from the
 * component AST and evaluated against a pool — the same source the
 * compiler will emit, not a hand-copied replica of the filter.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import { en } from '../../lib/i18n/en'
import { ko } from '../../lib/i18n/ko'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC_PATH = join(HERE, 'EpicProgress.svelte')
const source = readFileSync(SRC_PATH, 'utf8')

type AnyNode = { type: string } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

/*
 * Generic ESTree walk: every own property is a potential child, because the
 * instance script is ordinary JS and naming its node keys by hand would make
 * the sweep silently stop at whichever construct was not listed.
 */
function walk(node: unknown, visit: (n: AnyNode) => void): void {
  if (Array.isArray(node)) {
    for (const child of node) walk(child, visit)
    return
  }
  if (!isNode(node)) return
  visit(node)
  for (const key of Object.keys(node)) {
    if (key === 'type' || key === 'loc') continue
    walk(node[key], visit)
  }
}

function span(node: AnyNode): { start: number; end: number } {
  const { start, end } = node
  if (typeof start !== 'number' || typeof end !== 'number') {
    throw new Error(`${node.type} node carries no source range`)
  }
  return { start, end }
}

const ast = parse(source, { modern: true, filename: 'EpicProgress.svelte' }) as unknown as AnyNode

/** Dotted path of a member expression: `$derived.by` → "$derived.by". */
function memberPath(node: AnyNode): string | undefined {
  if (node.type === 'Identifier') return typeof node.name === 'string' ? node.name : undefined
  if (node.type !== 'MemberExpression' || node.computed === true) return undefined
  const object = isNode(node.object) ? memberPath(node.object) : undefined
  const property = isNode(node.property) ? node.property.name : undefined
  if (!object || typeof property !== 'string') return undefined
  return `${object}.${property}`
}

function unwrapRune(init: AnyNode): string | null {
  if (init.type !== 'CallExpression' || !isNode(init.callee)) return null
  const rune = memberPath(init.callee)
  if (rune !== '$derived' && rune !== '$derived.by') return null
  const args = Array.isArray(init.arguments) ? init.arguments.filter(isNode) : []
  const arg = args[0]
  if (!arg) return null
  const inner = source.slice(span(arg).start, span(arg).end)
  return rune === '$derived.by' ? `(${inner})()` : inner
}

type Derived = { name: string; expr: string }

function derivedBindings(): Derived[] {
  const out: Derived[] = []
  walk((ast.instance as AnyNode | undefined)?.content, (n) => {
    if (n.type !== 'VariableDeclarator' || !isNode(n.id) || !isNode(n.init)) return
    if (n.id.type !== 'Identifier' || typeof n.id.name !== 'string') return
    const expr = unwrapRune(n.init)
    if (expr === null) return
    out.push({ name: n.id.name, expr })
  })
  return out
}

type Lite = { issue_key: string; parent_key: string | null; epic_key: string | null }

const CHILD_BINDINGS = new Set(['epicChildren', 'parentChildren', 'children'])

function evalPool(allIssues: Lite[], issueKey: string): { children: Lite[]; titleKey: string } {
  const needed = derivedBindings().filter((d) => CHILD_BINDINGS.has(d.name))
  if (!needed.some((d) => d.name === 'children')) {
    throw new Error('EpicProgress.svelte has no children $derived binding')
  }
  const titleAttr = sectionTitleAttr()
  const titleExpr = titleExpression(titleAttr)
  const body = needed.map((d) => `const ${d.name} = ${d.expr};`).join('\n')
  const fn = new Function(
    'issues',
    'issueKey',
    't',
    `${body}\nreturn { children, titleKey: (${titleExpr}) };`,
  )
  return fn({ allIssues }, issueKey, (k: string) => k) as { children: Lite[]; titleKey: string }
}

function sectionTitleAttr(): AnyNode {
  const nodes: AnyNode[] = []
  walk(ast.fragment, (n) => nodes.push(n))
  const section = nodes.find((n) => n.type === 'Component' && n.name === 'Section')
  if (!section) throw new Error('EpicProgress.svelte has no <Section>')
  const attrs = Array.isArray(section.attributes) ? section.attributes.filter(isNode) : []
  const title = attrs.find((a) => a.type === 'Attribute' && a.name === 'title')
  if (!title) throw new Error('<Section> has no title attribute')
  return title
}

function titleExpression(title: AnyNode): string {
  const value = title.value
  if (!isNode(value)) throw new Error('<Section title> is not an expression')
  if (value.type === 'ExpressionTag' && isNode(value.expression)) {
    const s = span(value.expression)
    return source.slice(s.start, s.end)
  }
  throw new Error('<Section title> is not an ExpressionTag')
}

function catalog(table: typeof en | typeof ko): Record<string, string> {
  return table as unknown as Record<string, string>
}

/*
 * Two-hop pool (docs/DERIVE.md): a sub-task's epic_key is the ancestor epic,
 * never the parent story. Filtering only on epic_key therefore hides the
 * story's own children — the defect GDK-121 names.
 *
 * ORPHAN-1 has parent_key === the epic but no epic_key. When the epic_key
 * list is non-empty the two lists must not be mixed, so it stays out of
 * the epic branch.
 */
const EPIC = 'EPIC-1'
const STORY = 'STORY-1'
const SIBLING = 'STORY-2'
const pool: Lite[] = [
  { issue_key: EPIC, parent_key: null, epic_key: null },
  { issue_key: STORY, parent_key: EPIC, epic_key: EPIC },
  { issue_key: SIBLING, parent_key: EPIC, epic_key: EPIC },
  { issue_key: 'SUB-1', parent_key: STORY, epic_key: EPIC },
  { issue_key: 'SUB-2', parent_key: STORY, epic_key: EPIC },
  { issue_key: 'SUB-X', parent_key: SIBLING, epic_key: EPIC },
  { issue_key: 'ORPHAN-1', parent_key: EPIC, epic_key: null },
]

describe('EpicProgress child selection (GDK-121)', () => {
  test('① a story lists parent_key children under Child issues / 하위 이슈', () => {
    const { children, titleKey } = evalPool(pool, STORY)
    expect(children.map((c) => c.issue_key)).toEqual(['SUB-1', 'SUB-2'])
    expect(titleKey).toBe('detail.childIssues')
    expect(catalog(ko)[titleKey]).toBe('하위 이슈')
    expect(catalog(en)[titleKey]).toBe('Child issues')
  })

  test('② an epic keeps the epic_key list and the existing title', () => {
    const { children, titleKey } = evalPool(pool, EPIC)
    expect(children.map((c) => c.issue_key)).toEqual([STORY, SIBLING, 'SUB-1', 'SUB-2', 'SUB-X'])
    expect(children.map((c) => c.issue_key)).not.toContain('ORPHAN-1')
    expect(titleKey).toBe('detail.epicChildren')
    expect(catalog(en)[titleKey]).toBe('In this epic')
    expect(catalog(ko)[titleKey]).toBe('이 에픽의 이슈')
  })

  test('③ no children → the section does not render', () => {
    const { children } = evalPool(pool, 'SUB-1')
    expect(children).toEqual([])

    const ifTests: string[] = []
    walk(ast.fragment, (n) => {
      if (n.type !== 'IfBlock' || !isNode(n.test)) return
      const s = span(n.test)
      ifTests.push(source.slice(s.start, s.end))
    })
    expect(ifTests[0]).toBe('children.length > 0')
  })

  test('keeps the epic-progress test id the existing e2e binds to', () => {
    expect(source).toContain('data-testid="epic-progress"')
    expect(source).toContain('data-testid="epic-child-row"')
    expect(source).toContain('data-testid="epic-children-toggle"')
    expect(source).toMatch(/const PREVIEW = 20/)
  })
})
