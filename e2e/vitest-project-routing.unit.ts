import { readdirSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import config from '../vitest.config'

/*
 * GDK-1475: vitest.config.ts routes web/src tests between two projects by
 * hand — `unit`, which has no svelte plugin, and `pages-store`, which does.
 * A test that imports a runes module (`*.svelte.ts`) has to be excluded from
 * the first list and added to the second, in two places, by a person.
 *
 * The round that filed this measured the alternative: giving `unit` the
 * plugin and deleting both lists. It is not free — four alternating paired
 * runs on the same tree put the merged config about 7% above the split on
 * CPU time (min 31.3s vs 29.2s, and every merged sample above every split
 * sample but one). So the split stays, and this lint takes over the part of
 * it that a person was holding.
 *
 * What a red vitest run already catches: a test that needs the plugin and
 * did not get it. It fails loudly on the first run (the GDK-786 comment in
 * vitest.config.ts is that repair). That class does not need a lint.
 *
 * What nothing catches today, and this does: a test claimed by NO project.
 * Drop a file from `unit`'s exclude list and forget to add it to
 * `pages-store`'s include list — or rename a test file and leave the old
 * name behind in either list — and the suite stays green while that file
 * stops being run at all. Silent coverage loss, no red anywhere.
 *
 * The check is therefore about routing, not about runes: every
 * web/src/**\/*.test.ts on disk must be claimed by exactly one project, and
 * every exact path either list names must exist.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(HERE, '..')
const WEB_SRC = join(ROOT, 'web/src')

type ProjectConfig = {
  test: { name: string; include?: string[]; exclude?: string[] }
}

/** The web-facing projects, in config order. e2e-guard owns this file, not web/src. */
function webProjects(): ProjectConfig[] {
  const projects = (config as unknown as { test: { projects: ProjectConfig[] } }).test.projects
  return projects.filter((p) => p.test.name !== 'e2e-guard')
}

/*
 * Vitest matches include/exclude with picomatch. picomatch is a transitive
 * dependency here, not a declared one, so rather than import it this
 * understands the two pattern shapes the config actually uses — an exact
 * path and a `**` glob — and throws on anything else. A lint that silently
 * fails to understand a pattern is worse than no lint.
 */
function patternToRegExp(pattern: string): RegExp {
  if (/[?[\]{}!()+@]/.test(pattern)) {
    throw new Error(
      `vitest-project-routing.unit.ts does not understand the glob ${pattern!} — ` +
        'teach it the new syntax rather than letting the routing go unchecked',
    )
  }
  let out = '^'
  for (let i = 0; i < pattern.length; i++) {
    const ch = pattern[i]
    if (ch === '*' && pattern[i + 1] === '*') {
      // `**/` spans any number of directories, including none.
      if (pattern[i + 2] === '/') {
        out += '(?:[^/]+/)*'
        i += 2
      } else {
        out += '.*'
        i += 1
      }
      continue
    }
    if (ch === '*') {
      out += '[^/]*'
      continue
    }
    out += ch.replace(/[.+^${}()|[\]\\]/g, '\\$&')
  }
  return new RegExp(`${out}$`)
}

function matchesAny(path: string, patterns: string[] | undefined): boolean {
  return (patterns ?? []).some((p) => patternToRegExp(p).test(path))
}

/** Repo-relative posix paths of every test file under web/src. */
function webTestFiles(dir = WEB_SRC, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules') continue
    const path = join(dir, name)
    if (statSync(path).isDirectory()) webTestFiles(path, out)
    else if (name.endsWith('.test.ts')) out.push(relative(ROOT, path).split(/[\\/]/).join('/'))
  }
  return out.sort()
}

function claimants(file: string): string[] {
  return webProjects()
    .filter((p) => matchesAny(file, p.test.include) && !matchesAny(file, p.test.exclude))
    .map((p) => p.test.name)
}

test('every web/src test file is claimed by exactly one vitest project', () => {
  const orphans: string[] = []
  const contested: string[] = []
  for (const file of webTestFiles()) {
    const owners = claimants(file)
    if (owners.length === 0) orphans.push(file)
    if (owners.length > 1) contested.push(`${file} → ${owners.join(', ')}`)
  }
  expect(
    orphans,
    'these test files are excluded from every vitest project, so nothing runs them —\n' +
      'a file dropped from one list must be added to the other:\n' +
      orphans.join('\n'),
  ).toEqual([])
  expect(
    contested,
    'these test files are claimed by more than one project, so they run twice —\n' +
      'the plugin-less run of a runes test is the red one:\n' +
      contested.join('\n'),
  ).toEqual([])
})

test('every exact path named in a project include/exclude list still exists', () => {
  const onDisk = new Set(webTestFiles())
  const stale: string[] = []
  for (const project of webProjects()) {
    for (const [list, patterns] of [
      ['include', project.test.include],
      ['exclude', project.test.exclude],
    ] as const) {
      for (const pattern of patterns ?? []) {
        if (pattern.includes('*')) continue
        if (!onDisk.has(pattern)) stale.push(`${project.test.name}.${list}: ${pattern}`)
      }
    }
  }
  expect(
    stale,
    'these list entries name a file that is not on disk — a rename or a delete\n' +
      'left the routing behind:\n' +
      stale.join('\n'),
  ).toEqual([])
})
