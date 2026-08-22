/*
 * GDK-617(b,c): outside-close has one owner — the onOutsideClick / onEscape
 * actions in dom-actions.ts. BreakdownBar, SidebarNav's sync-history popover
 * and NotificationSettings had re-implemented it by hand (BreakdownBar
 * without any Esc handling at all, so an open menu leaked the keystroke to
 * the shell keymap and cleared the selection), and FieldEditor reached its
 * own portaled menu through a global testid selector, which answers for the
 * wrong instance once two editors mount.
 *
 * Wiring assertions on the components' real AST (SearchBox.test.ts idiom):
 * this repo's vitest runs node-environment with no mount harness, and the
 * wiring is the part that is easy to lose again. Rendered behaviour is
 * Playwright's job — list-menus-esc.spec.ts for the header menus,
 * breakdown-esc.spec.ts for this round's defect.
 *
 * Known and deliberate exceptions, so the sweep below stays honest:
 *  - FieldEditor keeps one hand-rolled mousedown listener: its boundary is
 *    two nodes (trigger + menu portaled to document.body), which the
 *    single-node action cannot express.
 *  - ScopePicker still carries a svelte:document hand-roll of its own —
 *    outside this round's whitelist (GDK-630).
 *
 * GDK-645: a component must not look up its own tree (or a child's testid)
 * with document.querySelector. DetailPanel used to find comment-composer that
 * way; it now binds the CommentComposer instance. keymap.svelte.ts is the
 * remaining global-selector dispatcher, and it lives under lib/ because the
 * target may not be mounted — that seam is out of this file's scope.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')

type AnyNode = { type: string; start: number; end: number } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

/* Not every node carries a source range — a Fragment is a bare `{ type,
 * nodes }` — so the range is asked for where it is needed rather than assumed
 * by the walker (SearchBox.test.ts learned this the hard way). */
function span(node: AnyNode): { start: number; end: number } {
  const { start, end } = node
  if (typeof start !== 'number' || typeof end !== 'number') {
    throw new Error(`${node.type} node carries no source range`)
  }
  return { start, end }
}

/* Template children only; `attributes` is not a child key (SearchBox.test.ts). */
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

function attribute(element: AnyNode, name: string): AnyNode | undefined {
  return attributesOf(element).find((a) => a.type === 'Attribute' && a.name === name)
}

function useDirective(element: AnyNode, name: string): AnyNode | undefined {
  return attributesOf(element).find((a) => a.type === 'UseDirective' && a.name === name)
}

function bindDirective(element: AnyNode, name: string): AnyNode | undefined {
  return attributesOf(element).find((a) => a.type === 'BindDirective' && a.name === name)
}

function testId(element: AnyNode): string | undefined {
  const attr = attribute(element, 'data-testid')
  if (!attr || !Array.isArray(attr.value)) return undefined
  const text = attr.value.filter(isNode).find((v) => v.type === 'Text')
  return typeof text?.data === 'string' ? text.data : undefined
}

function contains(outer: AnyNode, inner: AnyNode): boolean {
  const o = span(outer)
  const i = span(inner)
  return o.start <= i.start && i.end <= o.end
}

function load(rel: string): { source: string; nodes: AnyNode[] } {
  const source = readFileSync(join(WEB_SRC, rel), 'utf8')
  const ast = parse(source, { modern: true, filename: rel }) as unknown as AnyNode
  const nodes: AnyNode[] = []
  walkTemplate(ast.fragment, (n) => nodes.push(n))
  return { source, nodes }
}

/* The dismissable-surface contract as the three converted surfaces must
 * carry it: one always-mounted boundary element hosting both actions, the
 * trigger inside it (dom-actions.ts: the trigger counts as inside, or
 * clicking it to close closes and immediately reopens), and — where the
 * surface owns a popover — the popover inside it too. */
function boundaryOf(nodes: AnyNode[], inside: (n: AnyNode) => boolean): AnyNode | undefined {
  return nodes.find(
    (n) =>
      n.type === 'RegularElement' &&
      useDirective(n, 'onOutsideClick') &&
      useDirective(n, 'onEscape') &&
      nodes.filter(inside).every((target) => contains(n, target)),
  )
}

describe('outside-close has one owner (GDK-617c)', () => {
  test('BreakdownBar: the breakdown menu is an onOutsideClick boundary that spends Esc', () => {
    const { source, nodes } = load('components/list/BreakdownBar.svelte')
    const trigger = nodes.find(
      (n) => n.type === 'RegularElement' && n.name === 'button' && attribute(n, 'aria-expanded'),
    )
    expect(trigger, 'no aria-expanded trigger button in BreakdownBar.svelte').toBeDefined()
    expect(
      boundaryOf(nodes, (n) => n === trigger),
      'no element in BreakdownBar.svelte hosts use:onOutsideClick + use:onEscape around the trigger',
    ).toBeDefined()
    expect(
      source.includes('svelte:document'),
      'BreakdownBar still closes its menu from a svelte:document click handler',
    ).toBe(false)
  })

  test('SidebarNav: the sync-history popover is an onOutsideClick boundary that spends Esc', () => {
    const { nodes } = load('components/sidebar/SidebarNav.svelte')
    const trigger = nodes.find((n) => n.type === 'RegularElement' && testId(n) === 'sidebar-sync-now')
    const popover = nodes.find(
      (n) => n.type === 'RegularElement' && testId(n) === 'sync-history-popover',
    )
    expect(trigger, 'no [data-testid="sidebar-sync-now"] in SidebarNav.svelte').toBeDefined()
    expect(popover, 'no [data-testid="sync-history-popover"] in SidebarNav.svelte').toBeDefined()
    expect(
      boundaryOf(nodes, (n) => n === trigger || n === popover),
      'no element in SidebarNav.svelte hosts use:onOutsideClick + use:onEscape around trigger and popover',
    ).toBeDefined()
  })

  test('NotificationSettings: the bell popover is an onOutsideClick boundary that spends Esc', () => {
    const { source, nodes } = load('components/personal/NotificationSettings.svelte')
    const trigger = nodes.find(
      (n) => n.type === 'RegularElement' && n.name === 'button' && attribute(n, 'aria-expanded'),
    )
    expect(trigger, 'no aria-expanded trigger button in NotificationSettings.svelte').toBeDefined()
    expect(
      boundaryOf(nodes, (n) => n === trigger),
      'no element in NotificationSettings.svelte hosts use:onOutsideClick + use:onEscape around the trigger',
    ).toBeDefined()
    expect(
      source.includes('svelte:window'),
      'NotificationSettings still closes its popover from svelte:window handlers',
    ).toBe(false)
  })
})

describe('a component does not find its own child by a global selector (GDK-617b)', () => {
  test("FieldEditor's menu is bound, not queried", () => {
    const { source, nodes } = load('components/detail/FieldEditor.svelte')
    const menu = nodes.find((n) => n.type === 'RegularElement' && testId(n) === 'field-editor-menu')
    expect(menu, 'no [data-testid="field-editor-menu"] element in FieldEditor.svelte').toBeDefined()
    expect(
      bindDirective(menu as AnyNode, 'this'),
      'the field-editor-menu element has no bind:this — it is found some other way',
    ).toBeDefined()
    expect(
      source.includes('document.querySelector'),
      'FieldEditor still reaches for document.querySelector',
    ).toBe(false)
  })

  test("DetailPanel binds CommentComposer instead of querying comment-composer", () => {
    const { source, nodes } = load('components/detail/DetailPanel.svelte')
    const composer = nodes.find(
      (n) => n.type === 'Component' && n.name === 'CommentComposer',
    )
    expect(composer, 'no <CommentComposer> in DetailPanel.svelte').toBeDefined()
    expect(
      bindDirective(composer as AnyNode, 'this'),
      'CommentComposer has no bind:this — Esc still finds it some other way',
    ).toBeDefined()
    expect(
      source.includes('document.querySelector'),
      'DetailPanel still reaches for document.querySelector',
    ).toBe(false)
  })

  test('QuickComment binds CommentComposer instead of querying its own tree', () => {
    const { source, nodes } = load('components/write/QuickComment.svelte')
    const composer = nodes.find(
      (n) => n.type === 'Component' && n.name === 'CommentComposer',
    )
    expect(composer, 'no <CommentComposer> in QuickComment.svelte').toBeDefined()
    expect(
      bindDirective(composer as AnyNode, 'this'),
      'CommentComposer has no bind:this — open-focus still finds it some other way',
    ).toBeDefined()
    expect(
      source.includes('querySelector'),
      'QuickComment still reaches for querySelector',
    ).toBe(false)
  })
})

describe('class blockade: window-level outside-close is dom-actions territory', () => {
  test("no component but FieldEditor attaches its own mousedown outside-close", () => {
    const files = readdirSync(WEB_SRC, { recursive: true, encoding: 'utf8' }).filter((f) =>
      f.endsWith('.svelte'),
    )
    expect(files.length, 'the sweep found no components to read').toBeGreaterThan(50)

    const offenders: string[] = []
    for (const rel of files) {
      const source = readFileSync(join(WEB_SRC, rel), 'utf8')
      // window.addEventListener('mousedown', …) is the owner's signature;
      // FieldEditor's two-node boundary is the one sanctioned exception.
      if (/addEventListener\((['"])mousedown\1/.test(source) && !rel.endsWith('FieldEditor.svelte')) {
        offenders.push(rel)
      }
    }
    expect(offenders).toEqual([])
  })
})

describe('class blockade: a component does not query the document for its tree (GDK-645)', () => {
  test('no file under web/src/components calls document.querySelector', () => {
    const files = readdirSync(WEB_SRC, { recursive: true, encoding: 'utf8' }).filter((f) =>
      f.endsWith('.svelte'),
    )
    expect(files.length, 'the sweep found no components to read').toBeGreaterThan(50)

    const offenders: string[] = []
    for (const rel of files) {
      const source = readFileSync(join(WEB_SRC, rel), 'utf8')
      if (source.includes('document.querySelector')) offenders.push(rel)
    }
    expect(offenders).toEqual([])
  })
})
