import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from '@playwright/test'

/*
 * GDK-290: a waitForTimeout whose PASS means "N ms elapsed" is the flake class.
 * A duration wait is allowed only when the line (or the line immediately above)
 * names why the duration itself is the thing under test (CSS transition, poll
 * interval, elapsed-threshold). Sleep-to-hope-boot-finished is not that.
 *
 * demo/ hosted/ perf/ are their own suites (playwright.config testIgnore).
 * Nothing else is exempt: a helper's sleep flakes every spec that calls it,
 * so the one place a file-level exemption would hurt most is the helpers.
 */

const E2E = dirname(fileURLToPath(import.meta.url))
const SKIP_DIRS = new Set(['demo', 'hosted', 'perf', '.tmp', 'node_modules'])

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (name.endsWith('.ts')) out.push(p)
  }
  return out
}

function isComment(line: string | undefined): boolean {
  if (line === undefined) return false
  const t = line.trim()
  return t.startsWith('//') || t.startsWith('*') || t.startsWith('/*')
}

export function bareTimeouts(root = E2E): string[] {
  const hits: string[] = []
  for (const file of walk(root)) {
    const lines = readFileSync(file, 'utf8').split('\n')
    for (let i = 0; i < lines.length; i++) {
      if (!/\.waitForTimeout\s*\(/.test(lines[i])) continue
      if (lines[i].includes('//') || isComment(lines[i - 1])) continue
      hits.push(`${relative(root, file)}:${i + 1}: ${lines[i].trim()}`)
    }
  }
  return hits
}

test('every waitForTimeout in e2e/*.spec.ts names why the duration is the contract', () => {
  const hits = bareTimeouts()
  expect(
    hits,
    `bare waitForTimeout — add a why-comment on the same line or the line above:\n${hits.join('\n')}`,
  ).toEqual([])
})
