import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en, WRITE_ERROR_KEYS } from './en'
import { ko } from './ko'

const HERE = dirname(fileURLToPath(import.meta.url))
const WRITE_GO = join(HERE, '../../../../internal/server/write.go')

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
})
