import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en, WRITE_ERROR_KEYS } from './en'
import { ko } from './ko'

const HERE = dirname(fileURLToPath(import.meta.url))
const WRITE_GO = join(HERE, '../../../../internal/server/write.go')
const WEB_SRC = join(HERE, '../..')
const E2E_ROOT = join(HERE, '../../../../e2e')

/**
 * Keys built at runtime, not written as literals:
 *   fieldLabel        → `field.${field}`     (index.ts)
 *   columnLabel       → `column.${key}`      (index.ts)
 *   categoryLabel     → `category.${cat}`    (index.ts)
 *   deployStateLabel  → `deploy.${state}`    (index.ts)
 *
 * A prefix allowlist, not a key allowlist: `category.done` stays legal without
 * a quoted mention, and a brand-new unused key under the same prefix will not
 * fail. That is the honest limit — do not grow this into a list of keys.
 */
const DYNAMIC_KEY_PREFIXES = ['category.', 'field.', 'column.', 'deploy.'] as const

function hasDynamicPrefix(key: string): boolean {
  return DYNAMIC_KEY_PREFIXES.some((p) => key.startsWith(p))
}

function walkSourceFiles(dir: string, acc: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist') continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      walkSourceFiles(p, acc)
      continue
    }
    if (name.endsWith('.ts') || name.endsWith('.svelte') || name.endsWith('.js')) acc.push(p)
  }
  return acc
}

/**
 * Catalog keys with no quoted mention in the scanned tree.
 *
 * Reads: every `.ts` / `.svelte` / `.js` under `web/src` and `e2e`.
 * Skips: `en.ts` / `ko.ts` (those are the definitions).
 * Extra references: `WRITE_ERROR_KEYS` values (looked up at runtime, defined
 *   in `en.ts` after the catalog object).
 * Misses: Go, docs, tools, desktop, contrib, `web/index.html`, `web/public/`,
 *   concatenated keys (`'common' + '.yes'`), unquoted mentions.
 */
function unusedCatalogKeys(): string[] {
  const skip = new Set([join(HERE, 'en.ts'), join(HERE, 'ko.ts')])
  const files = [...walkSourceFiles(WEB_SRC), ...walkSourceFiles(E2E_ROOT)].filter(
    (f) => !skip.has(f),
  )
  const blobs = files.map((f) => readFileSync(f, 'utf8'))
  const extra = new Set<string>(Object.values(WRITE_ERROR_KEYS))
  const unused: string[] = []
  for (const key of Object.keys(en)) {
    if (hasDynamicPrefix(key)) continue
    if (extra.has(key)) continue
    const needles = [`'${key}'`, `"${key}"`, '`' + key + '`']
    if (blobs.some((s) => needles.some((n) => s.includes(n)))) continue
    unused.push(key)
  }
  return unused
}

/** Stable fail() codes inside failCreate. The prose fallback (err.Error()) is not a code. */
function failCreateCodes(src: string): string[] {
  const start = src.indexOf('func failCreate(')
  expect(start, 'internal/server/write.go must define failCreate').toBeGreaterThanOrEqual(0)
  const rest = src.slice(start)
  const next = rest.indexOf('\nfunc ', 1)
  const body = next === -1 ? rest : rest.slice(0, next)
  const codes = [...body.matchAll(/fail\(\s*w\s*,\s*[^,]+,\s*"([a-z][a-z0-9_]*)"\s*\)/g)].map(
    (m) => m[1],
  )
  expect(codes.length, 'failCreate must emit at least one fail() wire code').toBeGreaterThan(0)
  return [...new Set(codes)]
}

describe('catalog contracts', () => {
  test('empty-project label says every project (en/ko catalogs)', () => {
    expect(en['settings.sourcesNoProjects']).toMatch(/every project/)
    expect(ko['settings.sourcesNoProjects']).toContain('모든 프로젝트')
  })

  test('every failCreate wire code maps to a catalog sentence', () => {
    const src = readFileSync(WRITE_GO, 'utf8')
    const codes = failCreateCodes(src)
    const missing = codes.filter((c) => !(c in WRITE_ERROR_KEYS))
    expect(missing, `unmapped failCreate codes: ${missing.join(', ')}`).toEqual([])
    for (const code of codes) {
      const key = WRITE_ERROR_KEYS[code as keyof typeof WRITE_ERROR_KEYS]
      expect(en[key].length, `en ${key}`).toBeGreaterThan(0)
      expect(ko[key].length, `ko ${key}`).toBeGreaterThan(0)
    }
  })

  test('en and ko catalogs have the same key set', () => {
    expect(Object.keys(ko).sort()).toEqual(Object.keys(en).sort())
  })

  test('every catalog key is referenced or sits under a dynamic prefix', () => {
    const unused = unusedCatalogKeys()
    expect(unused, unused.join('\n')).toEqual([])
  })

  test('each status category has exactly one catalog key', () => {
    // category.inProgressSpaced was a second in-progress string. The owner is
    // categoryLabel(); one key per category is the containment.
    for (const cat of ['new', 'inprogress', 'done'] as const) {
      const keys = Object.keys(en).filter((k) => {
        if (!k.startsWith('category.')) return false
        const suffix = k.slice('category.'.length).toLowerCase()
        return suffix === cat || suffix.startsWith(cat)
      })
      expect(keys, `aliases of category.${cat}`).toEqual([`category.${cat}`])
    }
  })

  test('Korean catalog has no competing spelling of settled terms', () => {
    // Found by eye (v0.16 audit F5): 진행중 next to 진행 중, and {n} 이슈 next
    // to {n}건, on one Korean screen. Canonicals:
    //   진행 중  — Jira KO, AGENTS.md transition example, English "In progress"
    //   {n}건    — already the list / body-match / epic / sync counter
    // Exclusions (not competing spellings of those terms):
    //   qa.inProgress "진행"        — QA run-state glyph, not the category
    //   group.byStatusCategory      — "진행 단계", no 진행중 substring
    //   settings.runtimeIssues      — "이슈 {n}개": noun + generic 개, not {n} 이슈
    //   bare 이슈                    — section/noun copy (새 이슈, 이슈가 없습니다)
    const failures: string[] = []
    for (const [key, value] of Object.entries(ko)) {
      if (value.includes('진행중')) {
        failures.push(`${key}=${JSON.stringify(value)} contains "진행중" (use "진행 중")`)
      }
      if (/\{n\}\s*이슈/.test(value)) {
        failures.push(`${key}=${JSON.stringify(value)} uses "{n} 이슈" (use "{n}건")`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('list and sidebar issue counts use the same Korean unit', () => {
    const unitOf = (s: string): 'geon' | 'issue' | 'other' => {
      if (/\{n\}\s*건/.test(s)) return 'geon'
      if (/\{n\}\s*이슈/.test(s)) return 'issue'
      return 'other'
    }
    const list = ko['list.countIssues']
    const side = ko['sidebar.issueCount']
    const listUnit = unitOf(list)
    const sideUnit = unitOf(side)
    expect(
      listUnit,
      `list.countIssues=${JSON.stringify(list)} must use 건 or 이슈`,
    ).not.toBe('other')
    expect(
      sideUnit,
      `sidebar.issueCount=${JSON.stringify(side)} must match list.countIssues=${JSON.stringify(list)} (${listUnit})`,
    ).toBe(listUnit)
  })
})
