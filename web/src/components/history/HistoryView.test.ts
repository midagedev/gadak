/*
 * GDK-26: "Show issues in list" must land on an ungrouped list.
 *
 * The history pane's whole value is visit order — RECIPES promises
 * `first-seen = ORDER BY` for "show on the app" — and grouping shreds that
 * order into buckets on screen. `emptyConfig()` carries `defaultDisplay()`,
 * whose `group_by` is `status_category`, so an entry point that builds a
 * config from `emptyConfig()` and only fills `filters.keys` regroups by
 * status category and (because `defaultGroupBy` for a keys view is `none`)
 * serializes that regrouping into the URL as an explicit `g=status_category`.
 *
 * These assertions read the component's real AST (svelte/compiler parse)
 * rather than a rendered DOM, for the reason SearchBox.test.ts already
 * records: this repo has no component-mount harness — vitest runs
 * `environment: 'node'` and vitest.config.ts loads no svelte plugin, so
 * importing a `.svelte` file from a test fails outright. What the AST can
 * prove is the part that went missing: the entry point names a grouping, it
 * names it before the config is applied, and the value it names really means
 * "no grouping" once it passes through the view-config schema — that last
 * step is checked against the schema itself, not asserted by eye.
 *
 * Rendered behaviour (that the resulting list paints in visit order) is
 * Playwright's job; see e2e/history.spec.ts.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import {
  configToParams,
  emptyConfig,
  parseConfig,
  type GroupBy,
  type ViewConfig,
} from '../../lib/view-config'

const HERE = dirname(fileURLToPath(import.meta.url))
const HISTORY_VIEW = join(HERE, 'HistoryView.svelte')

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

/** Dotted path of a member expression: `c.display.group_by` → "c.display.group_by". */
function memberPath(node: AnyNode): string | undefined {
  if (node.type === 'Identifier') return typeof node.name === 'string' ? node.name : undefined
  if (node.type !== 'MemberExpression' || node.computed === true) return undefined
  const object = isNode(node.object) ? memberPath(node.object) : undefined
  const property = isNode(node.property) ? node.property.name : undefined
  if (!object || typeof property !== 'string') return undefined
  return `${object}.${property}`
}

const source = readFileSync(HISTORY_VIEW, 'utf8')
const ast = parse(source, { modern: true, filename: 'HistoryView.svelte' }) as unknown as AnyNode

const instanceNodes: AnyNode[] = []
walk((ast.instance as AnyNode | undefined)?.content, (n) => instanceNodes.push(n))

const openAsList = instanceNodes.find(
  (n) =>
    n.type === 'FunctionDeclaration' &&
    isNode(n.id) &&
    n.id.name === 'openAsList',
)

const bodyNodes: AnyNode[] = []
walk(openAsList?.body, (n) => bodyNodes.push(n))

/** Assignments to a `<something>.display.group_by` target inside openAsList. */
const groupingAssignments = bodyNodes.filter((n) => {
  if (n.type !== 'AssignmentExpression' || n.operator !== '=') return false
  if (!isNode(n.left)) return false
  return memberPath(n.left)?.endsWith('.display.group_by') === true
})

/**
 * Where the built config leaves this function: either the shared latch owner
 * (`showIssueList`) or `filters.applyConfig` directly. Whichever it is, the
 * grouping has to be decided before it.
 */
const applyCalls = bodyNodes.filter((n) => {
  if (n.type !== 'CallExpression' || !isNode(n.callee)) return false
  const path = memberPath(n.callee)
  return path === 'showIssueList' || path === 'filters.applyConfig'
})

/** The config this entry point hands on, rebuilt from what the AST says it sets. */
function configWithGrouping(groupBy: GroupBy): ViewConfig {
  const c = emptyConfig()
  c.filters.keys = ['NMA-11', 'NMA-1', 'NMA-118']
  c.display.group_by = groupBy
  return c
}

describe('HistoryView openAsList (GDK-26)', () => {
  test('the function the "show issues in list" button calls exists', () => {
    expect(openAsList, 'openAsList not found in HistoryView.svelte instance script').toBeDefined()
    expect(applyCalls.length, 'openAsList applies no config').toBeGreaterThan(0)
  })

  test('openAsList names a grouping explicitly (not left to emptyConfig)', () => {
    expect(
      groupingAssignments.length,
      'openAsList assigns no display.group_by — the config keeps emptyConfig()\'s status_category grouping',
    ).toBe(1)
    const value = groupingAssignments[0].right
    expect(isNode(value) && value.type, 'grouping must be a plain literal, readable at this line').toBe(
      'Literal',
    )
  })

  test('the grouping it names means "no grouping" in the view-config schema', () => {
    const value = groupingAssignments[0]?.right
    const groupBy = isNode(value) ? (value.value as GroupBy) : undefined
    expect(groupBy).toBe('none')

    // Not just the literal: the same value through the schema the list reads.
    const c = configWithGrouping(groupBy as GroupBy)
    expect(c.display.group_by).toBe('none')
    // A keys view's contextual default is already none, so nothing regroups
    // from the URL either — no `g=status_category` rides along.
    expect(configToParams(c).g).toBeNull()
    const sp = new URLSearchParams()
    for (const [k, v] of Object.entries(configToParams(c))) if (v !== null) sp.set(k, v)
    expect(parseConfig(sp).display.group_by).toBe('none')
  })

  test('the grouping is decided before the config is applied', () => {
    const assigned = span(groupingAssignments[0])
    const applied = Math.min(...applyCalls.map((c) => span(c).start))
    expect(assigned.end).toBeLessThan(applied)
  })
})

/*
 * GDK-850: the history pane's virtualization must re-snapshot rowMetrics
 * when user dimension overrides land at runtime (applyUserTokens →
 * invalidateRowMetrics). The defect: rowHeight read the module cache
 * directly, and a plain function read carries no signal dependency, so
 * VirtualRows' offsets derived kept the old heights until a remount. The
 * fix is the issue list's c34 pattern — a $state snapshot the height prop
 * reads, reassigned by an invalidation subscription (IssueList.svelte).
 */
const rowHeightFn = /function rowHeight\([\s\S]*?\n  \}/.exec(source)?.[0] ?? ''

describe('HistoryView rowMetrics invalidation (GDK-850)', () => {
  test('heights come from a $state snapshot, not untracked cache reads', () => {
    expect(source).toContain('let metrics = $state(rowMetrics())')
    expect(
      rowHeightFn,
      'rowHeight must read the snapshot — a direct rowMetrics() call inside the height prop is the untracked read that kept stale geometry',
    ).not.toContain('rowMetrics()')
    expect(rowHeightFn).toContain('metrics.row')
    expect(rowHeightFn).toContain('metrics.rowExcerpt')
  })

  test('subscribes to invalidation and re-snapshots (IssueList pattern)', () => {
    expect(source).toContain('onRowMetricsInvalidated(() => {')
    expect(source).toContain('metrics = rowMetrics()')
  })

  test('the subscription is torn down with the view', () => {
    // Without the unsubscribe the callback outlives the pane and writes to
    // a destroyed component's $state on every later token change.
    expect(source).toMatch(/offMetrics\(\)/)
  })
})
