/*
 * GDK-692: an $effect that writes $state / $derived is a synchronization
 * where a derivation would do. The value then depends on when the effect
 * ran, and this repo has shipped defects of that shape.
 *
 * vitest is environment:'node' with no svelte plugin on the unit project,
 * so a .svelte file cannot be mounted (FeaturesTab.test.ts). This scans
 * source: collect let/const names bound to $state / $derived, then fail
 * if an $effect body assigns to one of those names.
 *
 * Scoped to the files the owning rounds left clean. A repo-wide run is a
 * backlog report, not this gate — pre-existing violations elsewhere would
 * go red for reasons a round does not fix. GDK-817 added SpaceDocsView:
 * its root-reopening $effect (writing openDocs) was exactly this shape, and
 * the scan is what keeps it from growing back.
 *
 * Lives under web/src/lib/ because the scan covers component directories
 * (detail/, write/, settings/, docs/) and lib/ is the shared owner.
 */
import { readFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { emptyDraft, toSettings } from '../components/settings/draft'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const REPO = join(HERE, '../../..')

const SCANNED = [
  join(WEB_SRC, 'components/detail/LinkedIssues.svelte'),
  join(WEB_SRC, 'components/write/NewIssueDialog.svelte'),
  join(WEB_SRC, 'components/settings/SettingsDialog.svelte'),
  join(WEB_SRC, 'components/docs/SpaceDocsView.svelte'),
]

const RUNE_NAME =
  /(?:let|const)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\$(?:state|derived)\b/g
const EFFECT_OPEN = /\$effect\s*\(\s*\(\s*\)\s*=>\s*\{/g

export type Finding = { file: string; line: number; name: string }

function runeNames(src: string): string[] {
  const names: string[] = []
  RUNE_NAME.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = RUNE_NAME.exec(src))) names.push(m[1])
  return names
}

function lineAt(src: string, index: number): number {
  let n = 1
  for (let i = 0; i < index; i++) if (src[i] === '\n') n++
  return n
}

/** Index of the matching `}` for `{` at `open`, skipping strings and comments. */
function matchBrace(src: string, open: number): number {
  let depth = 0
  let i = open
  while (i < src.length) {
    const c = src[i]
    const n = src[i + 1]
    if (c === '/' && n === '/') {
      i = src.indexOf('\n', i)
      if (i < 0) break
      continue
    }
    if (c === '/' && n === '*') {
      i = src.indexOf('*/', i + 2)
      if (i < 0) break
      i += 2
      continue
    }
    if (c === "'" || c === '"' || c === '`') {
      const q = c
      i++
      while (i < src.length) {
        if (src[i] === '\\') {
          i += 2
          continue
        }
        if (src[i] === q) break
        i++
      }
      i++
      continue
    }
    if (c === '{') depth++
    else if (c === '}') {
      depth--
      if (depth === 0) return i
    }
    i++
  }
  throw new Error(`unclosed $effect block at ${lineAt(src, open)}`)
}

function effectBodies(src: string): { body: string; open: number }[] {
  const out: { body: string; open: number }[] = []
  EFFECT_OPEN.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = EFFECT_OPEN.exec(src))) {
    const open = m.index + m[0].length - 1
    const close = matchBrace(src, open)
    out.push({ body: src.slice(open + 1, close), open })
  }
  return out
}

function stripStringsAndComments(src: string): string {
  let out = ''
  let i = 0
  while (i < src.length) {
    const c = src[i]
    const n = src[i + 1]
    if (c === '/' && n === '/') {
      const end = src.indexOf('\n', i)
      const stop = end < 0 ? src.length : end
      out += src.slice(i, stop).replace(/[^\n]/g, ' ')
      i = stop
      continue
    }
    if (c === '/' && n === '*') {
      const end = src.indexOf('*/', i + 2)
      const stop = end < 0 ? src.length : end + 2
      out += src.slice(i, stop).replace(/[^\n]/g, ' ')
      i = stop
      continue
    }
    if (c === "'" || c === '"' || c === '`') {
      const q = c
      const start = i
      i++
      while (i < src.length) {
        if (src[i] === '\\') {
          i += 2
          continue
        }
        if (src[i] === q) {
          i++
          break
        }
        i++
      }
      out += src.slice(start, i).replace(/[^\n]/g, ' ')
      continue
    }
    out += c
    i++
  }
  return out
}

function assignmentsIn(body: string, names: string[]): { name: string; offset: number }[] {
  const code = stripStringsAndComments(body)
  const hits: { name: string; offset: number }[] = []
  for (const name of names) {
    const re = new RegExp(String.raw`\b${name}(?:\.[A-Za-z_][A-Za-z0-9_]*)*\s*=(?!=)`)
    const m = re.exec(code)
    if (m) hits.push({ name, offset: m.index })
  }
  return hits
}

export function scanSource(src: string, file: string): Finding[] {
  const names = runeNames(src)
  const rel = relative(REPO, file)
  const findings: Finding[] = []
  for (const { body, open } of effectBodies(src)) {
    for (const hit of assignmentsIn(body, names)) {
      findings.push({ file: rel, line: lineAt(src, open + 1 + hit.offset), name: hit.name })
    }
  }
  return findings
}

export function scanFiles(paths: string[]): Finding[] {
  return paths.flatMap((p) => scanSource(readFileSync(p, 'utf8'), p))
}

function format(findings: Finding[]): string {
  if (findings.length === 0) return '(none)'
  return findings.map((f) => `${f.file}:${f.line} assigns ${f.name}`).join('\n')
}

describe('GDK-692 no $effect writes $state/$derived in scanned files', () => {
  test('no $effect body assigns a $state or $derived name from the same file', () => {
    const findings = scanFiles(SCANNED)
    expect(format(findings), format(findings)).toBe('(none)')
  })
})

describe('GDK-692 confluence save payload follows the two-input rule', () => {
  test('spaces selected with the switch off save as enabled with those spaces', () => {
    const d = emptyDraft()
    d.confluenceOn = false
    d.spaces = ['ENG']
    expect(toSettings(d, true).confluence).toEqual({ enabled: true, spaces: ['ENG'] })
  })

  test('switch off and no spaces stays disabled with empty spaces', () => {
    const d = emptyDraft()
    d.confluenceOn = false
    d.spaces = []
    expect(toSettings(d, true).confluence).toEqual({ enabled: false, spaces: [] })
  })

  test('SourcesTab exposes the save-path value on the existing section', () => {
    const src = readFileSync(join(WEB_SRC, 'components/settings/SourcesTab.svelte'), 'utf8')
    expect(src).toMatch(/data-confluence-effective=\{confluenceEffective\}/)
    expect(src).toMatch(
      /onclick=\{\(\) => \{\s*draft\.confluenceOn = false[\s\S]*draft\.spaces = \[\]/,
    )
  })
})
