/*
 * GDK-727: the three narrowing fields (issue list, documents, history) share
 * one Enter contract — widen to the list's server search — and one owner of
 * that sequence.
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (HistoryView.test.ts / SearchBox.test.ts). What this file can prove is the
 * class in the source the compiler emits.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')

const FIELDS = [
  {
    name: 'issue list',
    file: join(WEB_SRC, 'components/list/SearchBox.svelte'),
    handler: 'onKeydown',
    // Own Enter: omnibox / jump-to-key / body search. Must not be rerouted
    // through widenToServerSearch; it already imports this module for outcomes.
    importsWiden: false,
  },
  {
    name: 'documents',
    file: join(WEB_SRC, 'components/docs/DocsFilter.svelte'),
    handler: 'onKeydown',
    importsWiden: true,
  },
  {
    name: 'history',
    file: join(WEB_SRC, 'components/history/HistoryView.svelte'),
    handler: 'onFilterKey',
    importsWiden: true,
  },
] as const

const OWNER = 'lib/server-search.ts'
const WIDEN_NAME = 'widenToServerSearch'
const SWALLOW_NON_ESCAPE = /if\s*\(\s*e\.key\s*!==\s*['"]Escape['"]\s*\)\s*return/
const ENTER_BRANCH = /e\.key\s*===\s*['"]Enter['"]/
const WIDEN_SEQUENCE = /setQuery\([^)]*\)[\s\S]{0,200}?runServerSearch\(/

function read(path: string): string {
  return readFileSync(path, 'utf8')
}

function functionBody(src: string, name: string): string {
  const start = src.search(new RegExp(`function\\s+${name}\\s*\\(`))
  expect(start, `${name} not found`).toBeGreaterThanOrEqual(0)
  const brace = src.indexOf('{', start)
  expect(brace, `${name} has no body`).toBeGreaterThan(start)
  let depth = 0
  for (let i = brace; i < src.length; i++) {
    const c = src[i]
    if (c === '{') depth++
    else if (c === '}') {
      depth--
      if (depth === 0) return src.slice(start, i + 1)
    }
  }
  throw new Error(`${name} body did not close`)
}

function walkFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walkFiles(p, out)
    else if (name.endsWith('.ts') || name.endsWith('.svelte')) {
      if (name.endsWith('.test.ts')) continue
      out.push(p)
    }
  }
  return out
}

describe('GDK-727 filter Enter widens', () => {
  test('each of the three fields handles Enter; none swallows it by returning unless Escape', () => {
    for (const field of FIELDS) {
      const body = functionBody(read(field.file), field.handler)
      expect(body, `${field.name}: no Enter branch in ${field.handler}`).toMatch(ENTER_BRANCH)
      expect(
        body,
        `${field.name}: ${field.handler} returns early for every key but Escape, so Enter never runs`,
      ).not.toMatch(SWALLOW_NON_ESCAPE)
    }
  })

  test('documents and history import the shared widen; the issue list imports the module', () => {
    for (const field of FIELDS) {
      const src = read(field.file)
      expect(src, `${field.name}: does not import lib/server-search`).toMatch(/from ['"].*server-search['"]/)
      if (field.importsWiden) {
        expect(src, `${field.name}: does not import ${WIDEN_NAME}`).toContain(WIDEN_NAME)
      }
    }
  })

  test('nothing but the shared owner implements setQuery + runServerSearch', () => {
    // SearchBox Enter is omnibox/jump then runServerSearch; the query is
    // already in the filter store from typing (setQuery on input), so a
    // true widen sequence should not appear there. If a future edit puts
    // the pair together, add that file to this list with a comment — do
    // not widen the regex. ListView retries runServerSearch on failure and
    // setQuery('') on clear, in different branches; the window is tight
    // enough that those do not count as one sequence.
    const allowed = new Set([OWNER])
    const hits: string[] = []
    for (const abs of walkFiles(WEB_SRC)) {
      if (!WIDEN_SEQUENCE.test(read(abs))) continue
      hits.push(relative(WEB_SRC, abs).replaceAll('\\', '/'))
    }
    expect(hits.sort(), `widen sequence files:\n${hits.join('\n')}`).toEqual([...allowed].sort())
  })

  test('each of the three inputs names the Enter contract', () => {
    for (const field of FIELDS) {
      expect(read(field.file), `${field.name}: missing data-enter="widen"`).toContain('data-enter="widen"')
    }
  })

  test('the shared owner exports the widen sequence', () => {
    const src = read(join(WEB_SRC, OWNER))
    expect(src).toContain(`export function ${WIDEN_NAME}`)
    expect(src).toMatch(WIDEN_SEQUENCE)
  })
})
