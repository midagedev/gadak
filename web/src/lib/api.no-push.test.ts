/*
 * GDK-711: the server pins notifications/config/ and
 * notifications/subscription/ at 404 (TestDeferredEndpointsAre404). A web
 * client that still names those paths is a second source of truth about a
 * capability the product refuses.
 *
 * Narrow form: scan web/src for those URL strings. A general "every client
 * URL has a mux handler" gate is a different round — dynamic paths, auth vs
 * issues bases, and desktop /desktop/ verbs do not parse honestly from a
 * string walk.
 *
 * vitest is environment:'node' (FeaturesTab.test.ts). This file skips itself
 * so the needles below are not a self-hit.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const SELF = fileURLToPath(import.meta.url)

const FORBIDDEN = ['notifications/config/', 'notifications/subscription/'] as const

function walkSource(dir: string, acc: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist') continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      walkSource(p, acc)
      continue
    }
    if (name.endsWith('.ts') || name.endsWith('.svelte') || name.endsWith('.js')) acc.push(p)
  }
  return acc
}

describe('GDK-711: web client does not call server-404 push endpoints', () => {
  test('no file under web/src references notifications/config/ or notifications/subscription/', () => {
    const hits: string[] = []
    for (const file of walkSource(WEB_SRC)) {
      if (file === SELF) continue
      const src = readFileSync(file, 'utf8')
      for (const needle of FORBIDDEN) {
        if (src.includes(needle)) hits.push(`${file.slice(WEB_SRC.length + 1)}: ${needle}`)
      }
    }
    expect(hits, hits.join('\n')).toEqual([])
  })
})
