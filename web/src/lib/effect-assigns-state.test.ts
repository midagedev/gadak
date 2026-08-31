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
 * the scan is what keeps it from growing back. The release-audit round
 * (2026-08-31) added the audit's residual files plus the panel that keys
 * DescriptionEditor, with two precision rules those files forced:
 *  - a write inside a `=> { … }` callback an effect registers is an event
 *    handler, not a synchronization with the effect's dependencies — the
 *    canonical ResizeObserver pattern (SearchBox) is not this defect class,
 *    and blankClosures keeps the scan usable on observer-heavy files;
 *  - ALLOWED names the two shapes that stay effects on purpose, each with
 *    its reason at the entry — a new effect writing any other name (or a
 *    second effect writing these) still fails.
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
  join(WEB_SRC, 'components/detail/DescriptionEditor.svelte'),
  join(WEB_SRC, 'components/detail/DetailPanel.svelte'),
  join(WEB_SRC, 'components/dashboard/DashboardView.svelte'),
  join(WEB_SRC, 'components/sidebar/SidebarNav.svelte'),
  join(WEB_SRC, 'components/list/SearchBox.svelte'),
]

const RUNE_NAME =
  /(?:let|const)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\$(?:state|derived)\b/g
const EFFECT_OPEN = /\$effect\s*\(\s*\(\s*\)\s*=>\s*\{/g

/** Deliberate exceptions — each names why the shape stays an effect. Each
 *  entry suppresses exactly ONE finding (withoutAllowed consumes it), so a
 *  second effect writing the same name still fails. Both entries were
 *  flagged by the scan before being listed (FAIL-first, 2026-08-31 round
 *  report); relaxing requires the reason at the entry, not silence. */
const ALLOWED: { file: string; name: string; why: string }[] = [
  {
    file: 'components/sidebar/SidebarNav.svelte',
    name: 'spacesOpen',
    why: 'the docs disclosure opens on arrival in a space; a derivation would take away collapse-while-reading (comment at the effect)',
  },
  {
    file: 'components/list/SearchBox.svelte',
    name: 'text',
    why: 'external q → input copy, guarded by focus; deriving from q snaps uncommitted IME text back on blur, so the sync stays until a round owns that behavior',
  },
]

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

/** Blank `=> { … }` bodies (length- and newline-preserving, so offsets and
 *  line numbers survive): a write inside a callback an effect registers is
 *  an event handler answering that event, not a synchronization with the
 *  effect's dependencies — the GDK-692 class is the latter. Arrows only;
 *  `function () { … }` callbacks are not blanked. */
function blankClosures(code: string): string {
  const out = code.split('')
  let i = 0
  while (i < code.length) {
    if (code[i] === '=' && code[i + 1] === '>') {
      let j = i + 2
      while (j < code.length && /\s/.test(code[j])) j++
      if (code[j] === '{') {
        const close = matchBrace(code, j)
        for (let k = j; k <= close; k++) if (code[k] !== '\n') out[k] = ' '
        i = close + 1
        continue
      }
      i += 2
      continue
    }
    i++
  }
  return out.join('')
}

function assignmentsIn(body: string, names: string[]): { name: string; offset: number }[] {
  const code = blankClosures(stripStringsAndComments(body))
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

/** Drop at most one finding per entry — the exception is one effect, not a
 *  name-shaped budget: a second effect assigning the same name produces a
 *  second finding and stays in the failure output. */
function withoutAllowed(findings: Finding[]): Finding[] {
  const used = new Set<number>()
  return findings.filter((f) => {
    const idx = ALLOWED.findIndex((a) => f.file.endsWith(a.file) && f.name === a.name)
    if (idx < 0) return true
    if (used.has(idx)) return true
    used.add(idx)
    return false
  })
}

describe('GDK-692 no $effect writes $state/$derived in scanned files', () => {
  test('no $effect body assigns a $state or $derived name from the same file', () => {
    const findings = withoutAllowed(scanFiles(SCANNED))
    expect(format(findings), format(findings)).toBe('(none)')
  })

  // Pins the precision contract blankClosures introduced: the reset shape
  // stays a finding, a write inside a callback the effect merely registers
  // does not. If a future narrowing eats the first half, this fails before
  // any scanned file gets the chance to regress quietly.
  test('the scan flags the reset shape and spares registered callbacks', () => {
    const src = [
      '<script lang="ts">',
      "  let { issueKey }: { issueKey: string } = $props()",
      '  let editing = $state(false)',
      '  let narrow = $state(false)',
      '  let el = $state<HTMLElement | null>(null)',
      '  $effect(() => {',
      '    void issueKey',
      '    editing = false',
      '  })',
      '  $effect(() => {',
      '    const ro = new ResizeObserver(() => {',
      '      narrow = el !== null',
      '    })',
      '    if (el) ro.observe(el)',
      '    return () => ro.disconnect()',
      '  })',
      '</script>',
    ].join('\n')
    const findings = scanSource(src, join(WEB_SRC, 'components/sample.svelte'))
    expect(findings.map((f) => f.name)).toEqual(['editing'])
  })

  // An exemption is one effect, not a budget: the second write of an
  // exempted name stays a failure. skipIf only for the day ALLOWED is empty.
  test.skipIf(ALLOWED.length === 0)('an exemption suppresses one finding, not a name-shaped budget', () => {
    const a = ALLOWED[0]!
    const two: Finding[] = [
      { file: `web/src/${a.file}`, line: 10, name: a.name },
      { file: `web/src/${a.file}`, line: 20, name: a.name },
    ]
    expect(withoutAllowed(two)).toHaveLength(1)
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
