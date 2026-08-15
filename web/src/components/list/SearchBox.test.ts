/*
 * The search box's help affordance, and the class of defect it belongs to
 * (GDK-53): a control whose only explanation is a hover `title` does nothing
 * on a touch screen — there is no hover to reveal it, and no handler behind
 * the tap.
 *
 * These assertions read the components' real AST (svelte/compiler parse)
 * rather than a rendered DOM, because this repo has no component-mount
 * harness: vitest runs `environment: 'node'` with no DOM implementation and
 * vitest.config.ts loads no svelte plugin, so importing a `.svelte` file from
 * a test fails outright ("Failed to parse source for import analysis").
 * Rendered behaviour — that the tapped panel is visible, that its text is the
 * help string — is Playwright's job. What is checkable here is the wiring
 * that went missing, and it is the part that is easy to lose again.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '../..')
const SEARCH_BOX = join(HERE, 'SearchBox.svelte')

type AnyNode = { type: string } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

/*
 * Not every node carries a source range — a Fragment is a bare `{ type,
 * nodes }` — so the range is asked for where it is needed rather than assumed
 * by the walker. (Assuming it silently stopped the walk at the root once.)
 */
function span(node: AnyNode): { start: number; end: number } {
  const { start, end } = node
  if (typeof start !== 'number' || typeof end !== 'number') {
    throw new Error(`${node.type} node carries no source range`)
  }
  return { start, end }
}

/*
 * Template children only. `attributes` is deliberately not a child key: a
 * `title={t('list.searchHelp')}` lives there, and the whole point of this file
 * is to tell a tooltip apart from something on screen.
 */
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

function templateNodes(source: string, filename: string): AnyNode[] {
  const ast = parse(source, { modern: true, filename }) as unknown as AnyNode
  const out: AnyNode[] = []
  walkTemplate(ast.fragment, (n) => out.push(n))
  return out
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

/*
 * Anything that makes a tap do something. `onclick` is the one that matters
 * here; the rest are counted so the sweep below does not report a control that
 * is driven by a pointer/touch handler instead, and a spread is opaque enough
 * to have to count as handled.
 */
const TAP_HANDLERS = new Set([
  'onclick',
  'onmousedown',
  'onmouseup',
  'onpointerdown',
  'onpointerup',
  'ontouchstart',
  'ontouchend',
])

function respondsToTap(element: AnyNode): boolean {
  return attributesOf(element).some(
    (a) =>
      a.type === 'SpreadAttribute' ||
      a.type === 'OnDirective' ||
      (a.type === 'Attribute' && typeof a.name === 'string' && TAP_HANDLERS.has(a.name)),
  )
}

function testId(element: AnyNode): string | undefined {
  const attr = attribute(element, 'data-testid')
  if (!attr || !Array.isArray(attr.value)) return undefined
  const text = attr.value.filter(isNode).find((v) => v.type === 'Text')
  return typeof text?.data === 'string' ? text.data : undefined
}

const searchBoxSource = readFileSync(SEARCH_BOX, 'utf8')
const searchBoxNodes = templateNodes(searchBoxSource, 'SearchBox.svelte')

const helpButton = searchBoxNodes.find(
  (n) => n.type === 'RegularElement' && n.name === 'button' && testId(n) === 'search-help',
)

/*
 * The help string in a screen position — an expression in the template body,
 * not in an attribute. walkTemplate never descends into `attributes`, so a
 * hit here cannot be the tooltip.
 */
const helpTextOnScreen = searchBoxNodes.filter((n) => {
  if (n.type !== 'ExpressionTag') return false
  const { start, end } = span(n)
  return searchBoxSource.slice(start, end).includes('list.searchHelp')
})

function contains(outer: AnyNode, inner: AnyNode): boolean {
  const o = span(outer)
  const i = span(inner)
  return o.start <= i.start && i.end <= o.end
}

describe('the ? help button works without a hover (GDK-53)', () => {
  test('it exists and answers a tap', () => {
    expect(helpButton, 'no button[data-testid="search-help"] in SearchBox.svelte').toBeDefined()
    expect(
      respondsToTap(helpButton as AnyNode),
      'the help button has no click handler: on a touch screen the tap does nothing',
    ).toBe(true)
  })

  test('it reports whether the help is open', () => {
    expect(attribute(helpButton as AnyNode, 'aria-expanded')).toBeDefined()
  })

  test('the desktop hover tooltip stays', () => {
    const title = attribute(helpButton as AnyNode, 'title')
    expect(title).toBeDefined()
    const { start, end } = span(title as AnyNode)
    expect(searchBoxSource.slice(start, end)).toContain('list.searchHelp')
  })

  test('the help text reaches the screen, not only the tooltip', () => {
    expect(
      helpTextOnScreen.length,
      'list.searchHelp appears only inside attributes — nothing renders it',
    ).toBeGreaterThan(0)
  })

  test('the panel closes on Escape', () => {
    const escapeHosts = searchBoxNodes.filter((n) => useDirective(n, 'onEscape'))
    expect(
      escapeHosts.some((host) => helpTextOnScreen.some((tag) => contains(host, tag))),
      'the element rendering the help text is not under a use:onEscape',
    ).toBe(true)
  })

  test('an outside click closes it, and the trigger counts as inside', () => {
    // dom-actions.ts states the guard: the boundary has to contain the
    // trigger, or clicking it to close closes and immediately reopens.
    const boundaries = searchBoxNodes.filter((n) => useDirective(n, 'onOutsideClick'))
    expect(
      boundaries.some(
        (boundary) =>
          contains(boundary, helpButton as AnyNode) &&
          helpTextOnScreen.some((tag) => contains(boundary, tag)),
      ),
      'no use:onOutsideClick boundary encloses both the ? button and the help panel',
    ).toBe(true)
  })
})

describe('class blockade: no button explains itself by hover alone', () => {
  test('every titled button in web/src answers a tap, or is a form submit', () => {
    const files = readdirSync(WEB_SRC, { recursive: true, encoding: 'utf8' }).filter((f) =>
      f.endsWith('.svelte'),
    )
    expect(files.length, 'the sweep found no components to read').toBeGreaterThan(50)

    const offenders: string[] = []
    for (const rel of files) {
      const source = readFileSync(join(WEB_SRC, rel), 'utf8')
      for (const node of templateNodes(source, rel)) {
        if (node.type !== 'RegularElement' || node.name !== 'button') continue
        if (!attribute(node, 'title') || respondsToTap(node)) continue
        // A submit button is driven by its form's onsubmit, not by itself.
        const type = attribute(node, 'type')
        if (type) {
          const { start, end } = span(type)
          if (source.slice(start, end).includes('submit')) continue
        }
        const line = source.slice(0, span(node).start).split('\n').length
        offenders.push(`${rel}:${line}`)
      }
    }
    expect(offenders).toEqual([])
  })
})
