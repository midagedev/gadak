import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en, ja, ko, messages, type MessageKey } from './catalog'
import { WRITE_ERROR_KEYS } from './errors'
import { detail } from './messages/detail'
import { common } from './messages/common'
import { fields } from './messages/fields'
import { list } from './messages/list'
import { personal } from './messages/personal'
import { settings } from './messages/settings'
import { shell } from './messages/shell'
import { write } from './messages/write'

const HERE = dirname(fileURLToPath(import.meta.url))
const WRITE_GO = join(HERE, '../../../../internal/server/write.go')
const WEB_SRC = join(HERE, '../..')
const E2E_ROOT = join(HERE, '../../../../e2e')
// The phone is a second app with its own tsconfig and its own lockfile, and
// it reads this catalog. Nothing at the repo root typechecks it, so a key
// deleted here stays green through every web gate and turns the Mobile CI
// job red on its own (2026-08-26, `terminal.unavailable`).
const MOBILE_SRC = join(HERE, '../../../../mobile/src')

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
 * Skips: `messages/*.ts` (definitions) and the derived `en.ts` / `ko.ts` /
 *   `ja.ts` re-exports. Keys live once under messages/, not in three
 *   locale files.
 * Extra references: `WRITE_ERROR_KEYS` values (looked up at runtime, defined
 *   in `errors.ts`).
 * Misses: Go, docs, tools, desktop, contrib, `web/index.html`, `web/public/`,
 *   concatenated keys (`'common' + '.yes'`), unquoted mentions.
 */
function isCatalogDefinition(p: string): boolean {
  const n = p.replaceAll('\\', '/')
  if (n.includes('/i18n/messages/')) return true
  return /\/i18n\/(en|ko|ja)\.ts$/.test(n)
}

function unusedCatalogKeys(): string[] {
  const files = [
    ...walkSourceFiles(WEB_SRC),
    ...walkSourceFiles(E2E_ROOT),
    ...walkSourceFiles(MOBILE_SRC),
  ].filter((f) => !isCatalogDefinition(f))
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

/**
 * `t('some.key')` literals in a tree, as (key, file) pairs.
 *
 * Deliberately literal-only: a key built at runtime is invisible here, which
 * is the same honest limit `unusedCatalogKeys` carries.
 */
function quotedTKeys(root: string): { key: string; file: string }[] {
  const out: { key: string; file: string }[] = []
  for (const file of walkSourceFiles(root)) {
    const src = readFileSync(file, 'utf8')
    for (const m of src.matchAll(/\bt\(\s*'([A-Za-z][\w-]*(?:\.[\w-]+)+)'/g)) {
      out.push({ key: m[1], file })
    }
  }
  return out
}

describe('catalog contracts', () => {
  // The web suite runs on every change; mobile's does not. So the web suite
  // is where a key the phone depends on has to be defended — otherwise the
  // first sign of a deletion is a red CI job on a diff that never opened
  // mobile/.
  test('every key the phone asks for exists in the catalog', () => {
    const refs = quotedTKeys(MOBILE_SRC)
    expect(refs.length, 'mobile/src must contain t() calls at all').toBeGreaterThan(10)
    const missing = refs
      .filter(({ key }) => !hasDynamicPrefix(key) && !(key in en))
      .map(({ key, file }) => `${key} (${file.slice(file.indexOf('mobile/'))})`)
    expect([...new Set(missing)], missing.join('\n')).toEqual([])
  })


  test('derived locale tables match the per-key catalog', () => {
    // t() reads the derived tables; a mismatch here is copy that the UI
    // never shows.
    const failures: string[] = []
    for (const key of Object.keys(messages) as MessageKey[]) {
      if (en[key] !== messages[key].en) failures.push(`en.${key}`)
      if (ko[key] !== messages[key].ko) failures.push(`ko.${key}`)
      if (ja[key] !== messages[key].ja) failures.push(`ja.${key}`)
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('domain catalogs do not share keys', () => {
    const parts: Record<string, object> = {
      common,
      fields,
      list,
      shell,
      personal,
      detail,
      write,
      settings,
    }
    const seen = new Map<string, string>()
    const dups: string[] = []
    for (const [name, part] of Object.entries(parts)) {
      for (const key of Object.keys(part)) {
        const prev = seen.get(key)
        if (prev) dups.push(`${key} in ${prev} and ${name}`)
        else seen.set(key, name)
      }
    }
    expect(dups, dups.join('\n')).toEqual([])
    expect(seen.size, `keys=${seen.size} locales=3`).toBe(Object.keys(en).length)
  })

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
      expect(ja[key].length, `ja ${key}`).toBeGreaterThan(0)
    }
  })

  test('no catalog value is empty or whitespace-only', () => {
    // GDK-620: the same-key test this replaces re-proved at runtime what
    // the type clause already enforces. That clause is now `satisfies
    // Record<string, Message>` on each messages/*.ts object (a missing
    // locale is a type error). The type clause still cannot see '' — a
    // blank value ships missing UI copy in exactly one locale. That axis
    // stays here.
    const failures: string[] = []
    for (const [locale, table] of [
      ['en', en],
      ['ko', ko],
      ['ja', ja],
    ] as const) {
      for (const [key, value] of Object.entries(table)) {
        if (value.trim() === '') failures.push(`${locale}.${key}=${JSON.stringify(value)}`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('ko and ja preserve every en placeholder token', () => {
    // Types force the key set; they cannot see `{n}` vs `{count}` inside a
    // string. Compare the multiset of `{…}` tokens (counts, not just
    // presence) so a translation cannot drop, rename, invent, or repeat a
    // placeholder the runtime substitutes — '{n}건 ({n})' against en's
    // '{n} issues' substitutes the same value twice and is a copy/paste
    // defect, not a translation.
    const tokenRe = /\{[^{}]+\}/g
    const tokens = (s: string): string[] => [...(s.match(tokenRe) ?? [])].sort()
    const failures: string[] = []
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      const want = tokens(en[key])
      for (const [locale, table] of [
        ['ko', ko],
        ['ja', ja],
      ] as const) {
        const got = tokens(table[key])
        if (got.join('\0') !== want.join('\0')) {
          failures.push(
            `${locale}.${key}: en=${JSON.stringify(want)} ${locale}=${JSON.stringify(got)}`,
          )
        }
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
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

  test('Japanese catalog has no competing spelling of settled terms', () => {
    // Canonicals (Japanese Jira Cloud):
    //   課題     — not イシュー
    //   進行中   — status category; not 進行 中
    //   {n}件    — issue/document counter (ko {n}건)
    // Exclusions:
    //   qa.inProgress "進行"           — QA run-state glyph, not the category
    //   settings.runtimeIssues         — "課題 {n}個": noun + generic 個, not {n}件
    //                                    (so the sidebar pool "{n}件" stays unique on screen)
    const failures: string[] = []
    for (const [key, value] of Object.entries(ja)) {
      if (value.includes('イシュー')) {
        failures.push(`${key}=${JSON.stringify(value)} contains "イシュー" (use "課題")`)
      }
      if (value.includes('進行 中')) {
        failures.push(`${key}=${JSON.stringify(value)} contains "進行 中" (use "進行中")`)
      }
      if (/\{n\}\s*課題/.test(value)) {
        failures.push(`${key}=${JSON.stringify(value)} uses "{n} 課題" (use "{n}件")`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('list and sidebar issue counts use the same Japanese unit', () => {
    const unitOf = (s: string): 'ken' | 'kadai' | 'other' => {
      if (/\{n\}\s*件/.test(s)) return 'ken'
      if (/\{n\}\s*課題/.test(s)) return 'kadai'
      return 'other'
    }
    const list = ja['list.countIssues']
    const side = ja['sidebar.issueCount']
    const listUnit = unitOf(list)
    const sideUnit = unitOf(side)
    expect(
      listUnit,
      `list.countIssues=${JSON.stringify(list)} must use 件 or 課題`,
    ).not.toBe('other')
    expect(
      sideUnit,
      `sidebar.issueCount=${JSON.stringify(side)} must match list.countIssues=${JSON.stringify(list)} (${listUnit})`,
    ).toBe(listUnit)
  })
})

/**
 * Wiki-object catalog keys (mirrored Confluence pages / documents).
 * browse.* is a different object (browser tabs) and is excluded on purpose.
 */
function isWikiObjectKey(key: string): boolean {
  return /^(doc\.|docs\.|sidebar\.docs|list\.doc|person\.docs|palette\.sectionDocs|palette\.actionDocs|palette\.docCount|history\.tabDocs|history\.emptyHint|settings\.confluence|settings\.sourcesNoSpaces|settings\.sourcesSpaces|settings\.roleBody|sync\.busyDocuments|sync\.partial|shortcuts\.sectionColumnViews|shortcuts\.tabMoveRows|shortcuts\.closeColumnView|onboarding\.standalone)/.test(
    key,
  )
}

describe('GDK-652 wiki-object noun: one noun per locale', () => {
  // Counted 2026-08-23 against wiki-object keys (browse.* excluded):
  //   EN document(s) 35 keys vs page(s) 12 → document
  //   KO 문서 44 vs 페이지 4 → 문서
  //   JA ドキュメント 37 vs ページ 12 → ドキュメント
  test('wiki-object strings do not use the competing page noun', () => {
    const failures: string[] = []
    for (const [key, value] of Object.entries(en)) {
      if (!isWikiObjectKey(key)) continue
      if (/\bpages?\b/i.test(value)) {
        failures.push(`en.${key}=${JSON.stringify(value)} uses page/pages (use document/documents)`)
      }
    }
    for (const [key, value] of Object.entries(ko)) {
      if (!isWikiObjectKey(key)) continue
      if (value.includes('페이지')) {
        failures.push(`ko.${key}=${JSON.stringify(value)} uses 페이지 (use 문서)`)
      }
    }
    for (const [key, value] of Object.entries(ja)) {
      if (!isWikiObjectKey(key)) continue
      if (value.includes('ページ')) {
        failures.push(`ja.${key}=${JSON.stringify(value)} uses ページ (use ドキュメント)`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('browse.* page means a web page and stays a page', () => {
    // The exclusion in the wiki-noun lock: these are browser tabs, not wiki objects.
    expect(en['browse.tabs']).toMatch(/pages?/i)
    expect(en['browse.closeTab']).toMatch(/page/i)
    expect(ko['browse.closeTab']).toContain('페이지')
    expect(ja['browse.closeTab']).toContain('ページ')
  })
})

describe('GDK-652 Korean issue counters share one unit', () => {
  test('selected-issue counts use the same Korean unit as list.countIssues', () => {
    // list.countIssues / sidebar.issueCount already lock 건. Selected-issue
    // copy (bulk bar, palette triage target) counts the same objects.
    const unitOf = (s: string): 'geon' | 'gae' | 'other' => {
      if (/\{n\}\s*건/.test(s)) return 'geon'
      if (/\{n\}\s*개/.test(s)) return 'gae'
      return 'other'
    }
    const list = ko['list.countIssues']
    const listUnit = unitOf(list)
    expect(listUnit, `list.countIssues=${JSON.stringify(list)}`).toBe('geon')
    for (const key of ['list.selectedCount', 'palette.triageSelected'] as const) {
      expect(
        unitOf(ko[key]),
        `${key}=${JSON.stringify(ko[key])} must use the same unit as list.countIssues=${JSON.stringify(list)}`,
      ).toBe(listUnit)
    }
  })
})

describe('GDK-652 loading verb is one per locale', () => {
  test('Korean Loading-family strings use 불러오는, never 로딩', () => {
    const failures: string[] = []
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      if (!/^Loading/.test(en[key])) continue
      const value = ko[key]
      if (value.includes('로딩')) {
        failures.push(`${key}=${JSON.stringify(value)} uses 로딩 (use 불러오는)`)
      }
      if (!value.includes('불러오는')) {
        failures.push(`${key}=${JSON.stringify(value)} has no 불러오는`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })

  test('feed.loading uses the same loading verb and ellipsis as common.loading', () => {
    const failures: string[] = []
    for (const [locale, table] of [
      ['en', en],
      ['ko', ko],
      ['ja', ja],
    ] as const) {
      const common = table['common.loading']
      const feed = table['feed.loading']
      const ellipsis = common.includes('…')
      if (ellipsis && !feed.includes('…')) {
        failures.push(`${locale} feed.loading=${JSON.stringify(feed)} dropped the ellipsis of common.loading`)
      }
      if (locale === 'en' && !/^Loading/.test(feed)) {
        failures.push(`${locale} feed.loading=${JSON.stringify(feed)} must start with Loading`)
      }
      if (locale === 'ko' && !feed.includes('불러오는')) {
        failures.push(`${locale} feed.loading=${JSON.stringify(feed)} must use 불러오는`)
      }
      if (locale === 'ja' && !feed.includes('読み込み')) {
        failures.push(`${locale} feed.loading=${JSON.stringify(feed)} must use 読み込み`)
      }
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
})

describe('GDK-652 Korean history filter uses 필터, not 좁히기', () => {
  test('history.filterPlaceholder and history.filterLabel share 필터 with docs.filter', () => {
    expect(ko['history.filterPlaceholder']).toContain('필터')
    expect(ko['history.filterLabel']).toContain('필터')
    expect(ko['history.filterPlaceholder']).not.toContain('좁히기')
    expect(ko['history.filterLabel']).not.toContain('좁히기')
  })
})

describe('GDK-652 onboarding first-sync copy matches the running-sync verb', () => {
  test('onboarding in-progress copy is sync.busyIssues (ellipsis included)', () => {
    // One owner: the first-sync line is the same event as the freshness chip.
    expect(en['sync.busyIssues']).toMatch(/…$/)
    expect(ko['sync.busyIssues']).toMatch(/…$/)
    expect(ja['sync.busyIssues']).toMatch(/…$/)
    const onboarding = readFileSync(join(WEB_SRC, 'components/shell/Onboarding.svelte'), 'utf8')
    expect(onboarding).toContain("t('sync.busyIssues')")
    expect(onboarding).not.toContain("t('onboarding.syncing')")
  })
})
