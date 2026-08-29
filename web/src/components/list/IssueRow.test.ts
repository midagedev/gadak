/*
 * GDK-617(a): the updated-time accent is a wall-clock derivation. A
 * $derived.by cannot depend on Date.now() — it recomputes only when
 * issue.updated_at changes, so a list left open overnight keeps yesterday's
 * "updated within 24h" accent until the data happens to change. The repo
 * already answered this question for the freshness chip (FreshnessChip.svelte):
 * a tick $state driven by an interval, re-read inside the derivation
 * (`void tick`). These assertions pin that wiring here.
 *
 * AST wiring assertions, not rendered behaviour, for the same reason as
 * SearchBox.test.ts: vitest runs environment:'node' with no component-mount
 * harness, so importing a .svelte file fails outright. The clock cannot be
 * injected here; what is checkable is that the derivation's dependency list
 * contains the clock, and that the tick is an interval with cleanup. The
 * rendered row is Playwright's job.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SOURCE_PATH = join(HERE, 'IssueRow.svelte')
const source = readFileSync(SOURCE_PATH, 'utf8')

type AnyNode = { type: string; start: number; end: number } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

/** Depth-first over the instance script's ESTree program. Unlike the
 *  template walker in SearchBox.test.ts there are no child keys to pick —
 *  ESTree nodes name their children as plain properties — but start/end
 *  still must not be walked into. */
function walkScript(node: unknown, visit: (n: AnyNode) => void): void {
  if (Array.isArray(node)) {
    for (const child of node) walkScript(child, visit)
    return
  }
  if (!isNode(node)) return
  visit(node)
  for (const [key, value] of Object.entries(node)) {
    if (key === 'start' || key === 'end') continue
    walkScript(value, visit)
  }
}

const program = (
  parse(source, { modern: true, filename: 'IssueRow.svelte' }) as unknown as {
    instance: { content: unknown }
  }
).instance.content

function declarator(name: string): string | undefined {
  let out: string | undefined
  walkScript(program, (n) => {
    if (out !== undefined || n.type !== 'VariableDeclarator') return
    const id = n.id as { type?: string; name?: string } | undefined
    if (id?.type === 'Identifier' && id.name === name) out = source.slice(n.start, n.end)
  })
  return out
}

function onMountBodies(): string[] {
  const out: string[] = []
  walkScript(program, (n) => {
    if (n.type !== 'CallExpression') return
    const callee = n.callee as { type?: string; name?: string } | undefined
    if (callee?.type !== 'Identifier' || callee.name !== 'onMount') return
    const first = (n.arguments as AnyNode[] | undefined)?.[0]
    if (first) out.push(source.slice(first.start, first.end))
  })
  return out
}

describe('the updated-time accent tracks the wall clock (GDK-617a)', () => {
  test("isFresh's dependency list contains the clock — FreshnessChip's tick", () => {
    const isFresh = declarator('isFresh')
    expect(isFresh, 'no isFresh declarator in IssueRow.svelte').toBeDefined()
    // The idiom: `void tick` makes the derivation re-read Date.now() each
    // tick. Without a tick read the derived only ever sees
    // issue.updated_at, and the accent freezes at mount.
    expect(isFresh, 'isFresh never reads the tick state — the accent freezes at mount').toMatch(
      /\btick\b/,
    )
    // The window itself is unchanged behaviour: 24h from updated_at.
    expect(isFresh).toContain('Date.now()')
    expect(isFresh).toContain('24 * 60 * 60 * 1000')
  })

  test('the tick is an interval started on mount and cleaned up on unmount', () => {
    const tick = declarator('tick')
    expect(tick, 'no tick $state in IssueRow.svelte').toBeDefined()
    expect(tick).toMatch(/\$state/)
    const clock = onMountBodies().filter((body) => body.includes('setInterval'))
    expect(clock, 'no onMount in IssueRow.svelte starts an interval for the clock').toHaveLength(1)
    expect(clock[0]).toMatch(/clearInterval/)
  })
})

/*
 * GDK-1087: the column header is this component in `header` mode, and that is
 * the whole recurrence story. The e2e gate (e2e/list-column-header.spec.ts)
 * measures that the header's cells sit exactly over the row's; this one keeps
 * the *reason* they do — that nobody has started a second copy of the slot
 * geometry somewhere else. A grep is enough because the failure mode is
 * textual: a header that writes `data-col` or a `w-…` slot of its own.
 */
describe('the column header has no geometry of its own (GDK-1087)', () => {
  const listSource = readFileSync(join(HERE, 'IssueList.svelte'), 'utf8')

  test('IssueList draws the header with IssueRow, not with markup of its own', () => {
    expect(listSource).toMatch(/<IssueRow\b[^>]*\bheader\b/s)
    expect(
      listSource,
      'IssueList names a column slot itself — the row is no longer the single owner of the geometry',
    ).not.toContain('data-col')
  })

  test('every trailing slot in the row has a header branch', () => {
    // Each slot is `data-col="key"`, and in header mode each renders the
    // shared `head` snippet instead of its value. A slot added without one
    // paints an empty header cell over a column with a value — the exact
    // shape of "the header labels half its columns".
    const slots = [...source.matchAll(/data-col="([a-z_]+)"/g)].map((m) => m[1])
    expect(slots.length).toBeGreaterThan(15)
    const labelled = new Set([...source.matchAll(/@render head\('([a-z_]+)'\)/g)].map((m) => m[1]))
    expect(slots.filter((col) => !labelled.has(col))).toEqual([])
  })
})
