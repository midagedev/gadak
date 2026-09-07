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
 * The scan walks every runes source under web/src and mobile/src (GDK-1464,
 * 2026-09-07). It used to name nine files by hand, which made the gate a
 * property of its own list rather than of the codebase: SessionStrip shipped
 * an $effect writing `snapshot` and the gate stayed green, because the file
 * was not listed AND the declaration carried a type annotation the name
 * regex did not admit. Both halves are closed here — the annotation is
 * optional in RUNE_NAME, and the list is a walk. Everything the walk finds
 * that is not this defect class is an ALLOWED entry with its reason, so the
 * exceptions are readable instead of invisible.
 *
 * GDK-817 added SpaceDocsView: its root-reopening $effect (writing openDocs)
 * was exactly this shape, and the scan is what keeps it from growing back.
 * The release-audit round (2026-08-31) contributed two precision rules that
 * observer-heavy and reset-shaped files forced:
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
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { emptyDraft, toSettings } from '../components/settings/draft'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const REPO = join(HERE, '../../..')

/** The two trees that compile runes. mobile/ has its own tsconfig and its
 *  own lockfile, so no root gate reads it (CLAUDE.md) — which is exactly why
 *  a scan that means to be a property of the codebase has to name it here. */
const ROOTS = [WEB_SRC, join(REPO, 'mobile/src')]

/** Every runes source under a root. `.svelte` and `.svelte.ts` are the only
 *  two file kinds where `$state` is a rune rather than an identifier, so a
 *  plain `.ts` file cannot hold this defect and is not read. Sorted, so a
 *  failure list is in the same order on every machine. */
function runeFiles(dir: string): string[] {
  const out: string[] = []
  const entries = readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  )
  for (const e of entries) {
    const p = join(dir, e.name)
    if (e.isDirectory()) out.push(...runeFiles(p))
    else if (e.name.endsWith('.svelte') || e.name.endsWith('.svelte.ts')) out.push(p)
  }
  return out
}

const SCANNED = ROOTS.flatMap(runeFiles)

/*
 * The type annotation is optional (GDK-1464). Without it,
 * `let snapshot: {…} | null = $state(null)` was not a rune name at all and
 * every write to it inside an effect was invisible — 18 declarations across
 * 11 files were outside the old form. `=>` is admitted inside the annotation
 * so a callback type (`Record<string, () => void>`) does not end the match at
 * its arrow; a newline still does, so a bare `let x: Foo` on one line cannot
 * reach forward to an unrelated `= $state(` on the next.
 */
const RUNE_NAME =
  /(?:let|const)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?::(?:[^=\n]|=>)*)?=\s*\$(?:state|derived)\b/g
const EFFECT_OPEN = /\$effect\s*\(\s*\(\s*\)\s*=>\s*\{/g

type Exception = { file: string; name: string; why: string }

/** One reason, several names in one file that share it. Still one entry per
 *  name — the expansion is for reading, not a budget: an effect writing one
 *  of these names twice still produces a second, unsuppressed finding. */
function shared(file: string, why: string, ...names: string[]): Exception[] {
  return names.map((name) => ({ file, name, why }))
}

/** Deliberate exceptions — each names why the shape stays an effect. Each
 *  entry suppresses exactly ONE finding (withoutAllowed consumes it), so a
 *  second effect writing the same name still fails. Every entry was flagged
 *  by the scan before being listed (FAIL-first: the 2026-08-31 round report
 *  for the first two, the GDK-1464 red run for the rest); relaxing requires
 *  the reason at the entry, not silence.
 *
 *  The reasons fall into four shapes, and none of them is "a derivation would
 *  do": a reset the arrival of a new key demands, an IO result with no input
 *  to derive from, a clock or debounce whose output IS the state, and a latch
 *  recording that an effect already fired. A new effect writing any name not
 *  listed here is still a failure. */
const ALLOWED: Exception[] = [
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
  {
    file: 'components/detail/DocumentPanel.svelte',
    name: 'postedDetail',
    why: 'reset on key change: the write-through overlay a comment POST returned is dropped when the open page changes, and nothing is left to derive it from',
  },
  {
    file: 'components/detail/IssueFields.svelte',
    name: 'prefetchKey',
    why: 'a once-per-key latch whose whole job is to record that the quiet editmeta prefetch already fired for this issue',
  },
  {
    file: 'components/list/IssueList.svelte',
    name: 'scrollTop',
    why: 'the scroll position is this effect’s output, not its input — a real view change scrolls to top, and a derivation would fight the person scrolling',
  },
  {
    file: 'components/list/SessionStrip.svelte',
    name: 'snapshot',
    why: 'the session-start count is a snapshot taken once at the boundary (G3); a derivation over the pool would grow it with every mid-session delta and re-say itself on remount',
  },
  {
    file: 'components/settings/SourcesTab.svelte',
    name: 'turnOnArmed',
    why: 'a two-click arm, disarmed by the state that makes the second click unnecessary — an event outcome, not a value with inputs',
  },
  ...shared(
    'components/write/CommentComposer.svelte',
    'per-issue draft hydration: the body, its mentions and its attachments are read from localStorage when the issue key changes, so these are IO results and the guards around them',
    'hydrating',
    'text',
    'mentions',
    'attachments',
    'escConsumed',
  ),
  ...shared(
    'components/write/CommentComposer.svelte',
    'the reply request prefixes a mention into text a person is editing — an edit to their draft, which no derivation may own',
    'text',
    'mentions',
  ),
  ...shared(
    'lib/resource.svelte.ts',
    'the key went away: the fetch resource drops its last response so a closed panel cannot re-show it — this module IS the sync between a key and a request',
    'data',
    'errorKind',
    'loading',
  ),
  {
    file: 'lib/skeleton-grace.svelte.ts',
    name: 'visible',
    why: 'the grace timer’s output: the skeleton appears after SKELETON_GRACE_MS of pending, which is a clock, not a derivation',
  },
  ...shared(
    'lib/user-search.svelte.ts',
    'a debounced search: the rows arrive from the network on a timer, and `searching` is that request’s own flag',
    'results',
    'searching',
  ),
  ...shared(
    'mobile/src/screens/Detail.svelte',
    'reset on key change: everything the previous issue left on screen is cleared before the new one is fetched, and that fetch is the effect',
    'detail',
    'detailError',
    'sheetOpen',
    'comment',
    'sendError',
    'pending',
    'transitions',
    'applying',
    'failedId',
    'transitionError',
  ),
  ...shared(
    'mobile/src/screens/PageDetail.svelte',
    'reset on key change: the previous page’s body and error are cleared before the new page is fetched',
    'detail',
    'detailError',
  ),
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
 *  name-shaped budget: a third effect assigning a twice-listed name produces
 *  a finding that stays in the failure output.
 *
 *  The match takes the first UNUSED entry (GDK-1464, 2026-09-07). It used to
 *  take the first matching entry and then refuse it as already spent, which
 *  silently collapsed two entries for the same file+name into the suppressing
 *  power of one — so a file with two deliberate effects on the same name
 *  could not be described at all, however many reasons were written down.
 *  Capacity still equals the number of entries; only the lookup is fixed.
 *  Entries are consumed in source order, so when one file lists a name twice
 *  the reasons must be written in the order the effects appear. */
function withoutAllowed(findings: Finding[], usedOut?: Set<number>): Finding[] {
  const used = usedOut ?? new Set<number>()
  return findings.filter((f) => {
    const idx = ALLOWED.findIndex(
      (a, i) => !used.has(i) && f.file.endsWith(a.file) && f.name === a.name,
    )
    if (idx < 0) return true
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

  /*
   * The blind spot GDK-1464 closed, pinned from both sides.
   *
   * `snapshot` is SessionStrip's shape verbatim: a type annotation between
   * the name and the `=`. Under the old RUNE_NAME it was not a rune name at
   * all, so the effect writing it was invisible and the gate was green over
   * a live instance of the defect. `handlers` carries an arrow inside the
   * annotation, which is where a lazier "anything but =" would stop.
   */
  test('a type annotation does not hide a $state name from the scan', () => {
    const src = [
      '<script lang="ts">',
      '  let snapshot: { since: string } | null = $state(null)',
      '  let handlers: Record<string, () => void> = $state({})',
      '  let plain = $state(0)',
      '  $effect(() => {',
      '    snapshot = { since: "now" }',
      '    handlers = {}',
      '    plain = 1',
      '  })',
      '</script>',
    ].join('\n')
    const findings = scanSource(src, join(WEB_SRC, 'components/sample.svelte'))
    expect(findings.map((f) => f.name).sort()).toEqual(['handlers', 'plain', 'snapshot'])
  })

  // A bare annotation on its own line must not reach across the newline and
  // capture an unrelated declaration below it.
  test('an uninitialised annotated let does not swallow the next declaration', () => {
    const src = [
      '<script lang="ts">',
      '  let el: HTMLElement | null',
      '  let count = $state(0)',
      '  $effect(() => {',
      '    count = 1',
      '  })',
      '</script>',
    ].join('\n')
    expect(scanSource(src, join(WEB_SRC, 'components/sample.svelte')).map((f) => f.name)).toEqual([
      'count',
    ])
  })

  /*
   * The list is the codebase, not a list. The hand-maintained SCANNED is what
   * let SessionStrip ship the defect, so the property under test is now that
   * the walk reaches both trees and every runes file kind — a future edit that
   * quietly narrows it back to a handful fails here first.
   */
  test('the scan walks both runes trees, components and .svelte.ts alike', () => {
    const rel = SCANNED.map((f) => relative(REPO, f))
    expect(rel).toContain('web/src/components/list/SessionStrip.svelte')
    expect(rel).toContain('web/src/lib/resource.svelte.ts')
    expect(rel.some((f) => f.startsWith('mobile/src/'))).toBe(true)
    expect(rel.every((f) => f.endsWith('.svelte') || f.endsWith('.svelte.ts'))).toBe(true)
    // Nine was the hand-list. Any plausible walk is far past it.
    expect(rel.length).toBeGreaterThan(100)
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

  /*
   * No dead exemptions. An entry that suppresses nothing is a claim about
   * code that no longer exists — and the next person to read the list has no
   * way to tell it from a live one. When a round turns one of these effects
   * into a derivation, deleting its entry is part of that round; this says so
   * out loud, and names the entry to delete.
   */
  test('every ALLOWED entry suppresses a real finding', () => {
    const used = new Set<number>()
    withoutAllowed(scanFiles(SCANNED), used)
    const dead = ALLOWED.map((a, i) => ({ a, i }))
      .filter(({ i }) => !used.has(i))
      .map(({ a }) => `${a.file} / ${a.name}`)
    expect(dead, `stale exemptions — the effect is gone, delete the entry:\n${dead.join('\n')}`).toEqual([])
  })

  /* Capacity equals the number of entries. CommentComposer has two effects
   * writing `text` for two different reasons and both are listed, so two
   * findings must clear and a third must not (GDK-1464). */
  test('two entries for one file+name suppress two findings and no more', () => {
    const dup = ALLOWED.filter((a) => a.file.endsWith('CommentComposer.svelte') && a.name === 'text')
    expect(dup).toHaveLength(2)
    const three: Finding[] = [10, 20, 30].map((line) => ({
      file: `web/src/${dup[0]!.file}`,
      line,
      name: 'text',
    }))
    expect(withoutAllowed(three)).toHaveLength(1)
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
