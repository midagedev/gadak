/*
 * GDK-1055: the feed's "Field change" detail must show field display
 * names, not raw changelog ids ("Field change issueparentassociation").
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (favorites-labels.test.ts documents the same limit). What this file
 * proves: the catalog owner (fieldLabel + field.*) resolves the ids the
 * feed ships, and the PersonalFeed source routes payload.fields through
 * that owner — no raw passthrough, no second mapping table.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import { fieldLabel } from '../../lib/i18n'
import { en, ja, ko } from '../../lib/i18n/catalog'

const HERE = dirname(fileURLToPath(import.meta.url))
const FEED = join(HERE, 'PersonalFeed.svelte')

/*
 * GDK-1474: "does this component get fieldLabel from the i18n owner" used to
 * be /import \{ t, fieldLabel(, \w+)* \} from '\.\.\/\.\.\/lib\/i18n'/ — one
 * spelling of one line, order included. It had already broken once and been
 * generalised, and the comment above it recorded the repair; the next name
 * added to that list, or a line the formatter wraps, breaks it again. The
 * binding is one tier down, in the module's import declarations, so it is
 * read there: order-independent, wrap-independent, and blind to a match
 * inside a comment.
 */
type AnyNode = { type: string } & Record<string, unknown>

/** Named bindings this component imports from `specifier`. */
function namedImports(path: string, filename: string, specifier: string): string[] {
  const ast = parse(readFileSync(path, 'utf8'), { modern: true, filename }) as unknown as {
    instance: { content: { body: AnyNode[] } } | null
    module: { content: { body: AnyNode[] } } | null
  }
  const names: string[] = []
  for (const part of [ast.instance, ast.module]) {
    for (const node of part?.content.body ?? []) {
      if (node.type !== 'ImportDeclaration') continue
      const source = node.source as { value?: unknown } | undefined
      if (source?.value !== specifier) continue
      for (const spec of (node.specifiers ?? []) as AnyNode[]) {
        if (spec.type !== 'ImportSpecifier') continue
        const imported = spec.imported as { name?: unknown } | undefined
        if (typeof imported?.name === 'string') names.push(imported.name)
      }
    }
  }
  return names
}

describe('GDK-1055 feed field names go through the catalog owner', () => {
  test('fieldLabel resolves the raw changelog ids the feed ships', () => {
    // Node has no localStorage and navigator.language is en*, so the
    // active locale here is en.
    expect(fieldLabel('issueparentassociation')).toBe('Parent')
    expect(fieldLabel('labels')).toBe('Labels')
    // Composed the way the feed composes payload.fields.
    expect(['issueparentassociation', 'labels'].map(fieldLabel).join(', ')).toBe('Parent, Labels')
  })

  test('field.issueparentassociation is present in every locale', () => {
    expect(en['field.issueparentassociation']).toBe('Parent')
    expect(ko['field.issueparentassociation']).toBe('상위 항목')
    expect(ja['field.issueparentassociation']).toBe('親課題')
  })

  test('PersonalFeed maps payload.fields through fieldLabel, never raw', () => {
    const src = readFileSync(FEED, 'utf8')
    // fieldLabel comes from the i18n owner. Where it sits in the import list
    // is not the contract, so the list is not pinned — only the binding.
    expect(namedImports(FEED, 'PersonalFeed.svelte', '../../lib/i18n')).toContain('fieldLabel')
    // Both branches (payload.fields[] and legacy changes[].label) map.
    expect(src).toMatch(/typeof f === 'string' \? fieldLabel\(f\) : ''/)
    expect(src).toMatch(/typeof label === 'string' \? fieldLabel\(label\) : ''/)
    // The pre-GDK-1055 raw passthrough must stay gone.
    expect(src).not.toMatch(/typeof f === 'string' \? f : ''/)
  })
})

describe('GDK-1066 a failed feed load is failure copy, never the empty copy', () => {
  test('feed.loadFailed is present in every locale, in the docs.loadFailed tone', () => {
    expect(en['feed.loadFailed']).toBe('Could not load the feed.')
    expect(ko['feed.loadFailed']).toBe('피드를 불러오지 못했습니다.')
    expect(ja['feed.loadFailed']).toBe('フィードを読み込めませんでした。')
  })

  test('PersonalFeed branches failure before empty', () => {
    const src = readFileSync(FEED, 'utf8')
    const failure = src.indexOf('me.feedLoadFailed && me.feedItems.length === 0')
    const empty = src.indexOf('{:else if me.feedItems.length === 0}')
    // The failure branch exists (flag + zero rows) and says so with its own
    // copy — so feed.empty can only mean "the request succeeded with 0 rows".
    expect(failure, 'failure branch must key on feedLoadFailed with no stale rows').toBeGreaterThan(-1)
    expect(src).toMatch(/t\('feed\.loadFailed'\)/)
    expect(empty, 'the empty branch must survive for success + zero rows').toBeGreaterThan(-1)
    expect(failure).toBeLessThan(empty)
  })
})

/*
 * The feed reads as days: sticky day sections (feed-days.ts), rows that
 * show time-of-day (the day is in the header), and a who-did-what second
 * line (UX §6). Source-reading, like the GDK-1055 block above — the unit
 * project has no svelte plugin. FAIL-first per case: against the
 * pre-round PersonalFeed each of these greps misses (no feed-day header,
 * `actor_name}: ${excerpt}` still present, relativeTime on both rows).
 */
describe('the feed reads as days', () => {
  test('the source builds sections and renders the sticky day header', () => {
    const src = readFileSync(FEED, 'utf8')
    // The layer exists and is fed by feed-groups' output, not me.feedItems
    // directly — the day layer sits above the collapse layer.
    expect(namedImports(FEED, 'PersonalFeed.svelte', './feed-days')).toEqual(
      expect.arrayContaining(['feedDaySections', 'feedDayLabelText']),
    )
    expect(src).toMatch(/feedDaySections\(groups\)/)
    // The header itself, with its sticky treatment and semantic day key.
    expect(src).toContain('data-testid="feed-day"')
    expect(src).toContain('data-day={section.key}')
    expect(src).toMatch(/sticky top-0 z-10/)
  })

  test("the comment detail drops the actor prefix — the actor has its own span", () => {
    const src = readFileSync(FEED, 'utf8')
    // The pre-round line prefixed the excerpt because the actor appeared
    // nowhere else; with a who-did-what line it would say the name twice.
    expect(src).not.toContain('item.actor_name}: ${excerpt}')
    expect(src).toMatch(
      /if \(item\.event_type === 'comment_added'\) \{[\s\S]*?return payloadString\(item, 'excerpt'\)/,
    )
    // And the actor really did move to its own span on the second line.
    expect(src).toContain('{item.actor_name}')
  })

  test('rows show time-of-day, not relative time', () => {
    const src = readFileSync(FEED, 'utf8')
    // relativeTime is fully gone from the component — both row snippets
    // and the import (the day is in the header now, so "2h ago" inside a
    // day section would repeat it less precisely).
    expect(src).not.toContain('relativeTime')
    expect(src).toContain('formatTimeOfDay(item.occurred_at)')
    expect(src).toContain('formatTimeOfDay(rep.occurred_at)')
  })
})
