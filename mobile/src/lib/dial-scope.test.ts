import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { ApiError, request, errorMessage, type FetchLike } from './api'
import { inDialScope } from './dial-scope'

/*
 * GDK-1048 gate: the TS dial predicate and the `http:default` capability
 * allowlist are two spellings of one fact, and this file is what turns
 * their drift into a red test. It reads the capability JSON itself — not a
 * restatement — and asks both sides the same questions.
 *
 * GDK-1581 widened the corpus from one file to a directory: default.json
 * (every target, ts.net only) plus dev-loopback.json (loopback, pinned to
 * non-iOS platforms so the App Store binary's ACL never carries it). The
 * verdict parity below runs against the UNION of their allow entries; the
 * corpus-shape block pins the split itself — exact file set, exact
 * permission identifiers, exact allow sets per file, and exact platforms —
 * because a third file or a new permission would widen what the phone may
 * dial without any row here going red.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const CAPABILITIES_DIR = join(HERE, '../../src-tauri/capabilities')

interface CapabilityDoc {
  identifier?: string
  platforms?: string[]
  permissions: Array<{ identifier?: string; allow?: Array<{ url?: string }> } | string>
}

function readCapabilityDocs(): Map<string, CapabilityDoc> {
  const names = readdirSync(CAPABILITIES_DIR)
    .filter((n) => n.endsWith('.json'))
    .sort()
  const docs = new Map<string, CapabilityDoc>()
  for (const name of names) {
    const doc = JSON.parse(readFileSync(join(CAPABILITIES_DIR, name), 'utf8')) as CapabilityDoc
    expect(doc.permissions, `${name} must carry permissions`).toBeDefined()
    docs.set(name, doc)
  }
  expect(docs.size, 'the capability corpus is exactly two files').toBe(2)
  return docs
}

function httpAllowUrls(doc: CapabilityDoc): string[] {
  const entry = doc.permissions.find(
    (p): p is { identifier?: string; allow?: Array<{ url?: string }> } =>
      typeof p === 'object' && p.identifier === 'http:default',
  )
  expect(entry, 'capability must grant http:default').toBeDefined()
  const urls = (entry?.allow ?? [])
    .map((a) => a.url)
    .filter((u): u is string => typeof u === 'string')
  expect(urls.length, 'http:default must carry at least one allow url').toBeGreaterThan(0)
  return urls
}

const unionAllowUrls = (): string[] => {
  const urls: string[] = []
  for (const doc of readCapabilityDocs().values()) urls.push(...httpAllowUrls(doc))
  return urls
}

/*
 * Minimal URLPattern stand-in for the shapes the capability files use.
 *
 * Why not the real URLPattern: the repo's Node is pinned by .nvmrc to 20,
 * which has no global URLPattern (measured: v20.19.0 `typeof URLPattern`
 * is 'undefined'; the unflagged global arrives in Node 24), and neither the
 * root nor the mobile package.json carries a urlpattern polyfill. The round
 * spec forbids a new dependency, so the matcher lives here and covers
 * exactly what the capability files write:
 *   - shape `<scheme>://<host>` or `<scheme>://<host>:<port>`
 *   - host: a literal, or `*.<suffix>` where `*` spans dots —
 *     `deep.home.example.ts.net` matches `*.ts.net`, the bare apex
 *     `ts.net` does not
 *   - port: omitted admits the scheme's DEFAULT port only (this is the
 *     GDK-1048 trap), `*` admits any port including the default, a literal
 *     admits itself
 * Semantics were measured against Node 24's URLPattern (see the round
 * report). Any new shape in the corpus makes the parse assertion below
 * fail first, so this matcher cannot silently under-match.
 */
function capabilityVerdict(pattern: string, testUrl: string): boolean {
  const m = /^([a-z][a-z0-9+.-]+):\/\/([^/:]+)(?::(\*|\d+))?$/.exec(pattern)
  if (!m) throw new Error(`capability entry shape not covered by this matcher: ${pattern}`)
  const [, scheme, hostPat, portPat] = m
  let u: URL
  try {
    u = new URL(testUrl)
  } catch {
    return false
  }
  if (u.protocol !== `${scheme}:`) return false
  const host = u.hostname.toLowerCase()
  const hostOk = hostPat.startsWith('*.') ? host.endsWith(hostPat.slice(1)) : host === hostPat
  if (!hostOk) return false
  if (portPat === '*') return true
  const def = u.protocol === 'https:' ? '443' : u.protocol === 'http:' ? '80' : ''
  if (portPat === undefined) return u.port === '' || u.port === def
  return u.port === portPat || (u.port === '' && def === portPat)
}

const allowedByCapability = (url: string): boolean =>
  unionAllowUrls().some((p) => capabilityVerdict(p, url))

const sorted = (xs: string[]): string[] => [...xs].sort()

describe('dial scope: TS predicate and the capability corpus agree', () => {
  // Third column pins intent; the capability column is derived from the
  // files, the predicate column from the module — all three must agree.
  const TABLE: Array<[url: string, expected: boolean]> = [
    ['https://h.ts.net/', true],
    ['https://h.ts.net:8443/', true], // the GDK-1048 row: tailscale serve on 8443
    ['https://h.ts.net:443/', true], // explicit default port == omitted
    ['https://deep.home.example.ts.net:10000/', true], // wildcard spans dots; serve's other port
    ['http://127.0.0.1:7777/', true],
    ['http://localhost:5173/', true],
    ['https://evil.com/', false],
    ['http://h.ts.net:8443/', false], // plaintext to a tailnet name
    ['https://ts.net.evil.com/', false], // suffix trick
    ['https://ts.net/', false], // bare apex: not inside *.ts.net
    ['http://[::1]:7877/', false], // IPv6 loopback is not in the list
    ['ws://h.ts.net:8443/', false], // ws is not an http permission scheme
    ['not a url', false],
  ]

  it('every row: expected == capability URLPattern == inDialScope', () => {
    for (const [url, expected] of TABLE) {
      expect(allowedByCapability(url), `capability verdict for ${url}`).toBe(expected)
      expect(inDialScope(url), `predicate verdict for ${url}`).toBe(expected)
    }
  })

  it('every allow entry in the corpus is a shape this gate understands', () => {
    for (const pattern of unionAllowUrls()) {
      expect(() => capabilityVerdict(pattern, 'https://probe.invalid/'), pattern).not.toThrow()
    }
  })

  it('every allow entry admits at least one table row (no dead entries)', () => {
    // A new capability entry with no in-scope table row would pass verdict
    // parity vacuously — the TS predicate could simply never admit it. Each
    // entry must own at least one true row.
    for (const pattern of unionAllowUrls()) {
      const owns = TABLE.some(([url, expected]) => expected && capabilityVerdict(pattern, url))
      expect(owns, `allow entry ${pattern} must back at least one in-scope row`).toBe(true)
    }
  })
})

describe('capability corpus shape: what the shipped ACL is made of (GDK-1581)', () => {
  it('the union of allow entries mirrors the TS predicate entry for entry', () => {
    // The mirror direction of the verdict table: not just "the predicate
    // agrees with the files", but "the files say exactly what the predicate
    // says" — nothing granted that the predicate refuses, nothing refused
    // that the predicate grants.
    expect(sorted(unionAllowUrls())).toEqual([
      'http://127.0.0.1:*',
      'http://localhost:*',
      'https://*.ts.net:*',
    ])
  })

  it('the corpus is exactly default.json + dev-loopback.json, by identifier', () => {
    const docs = readCapabilityDocs()
    expect(sorted([...docs.keys()])).toEqual(['default.json', 'dev-loopback.json'])
    const ids = sorted([...docs.values()].map((d) => d.identifier ?? ''))
    expect(ids).toEqual(['default', 'dev-loopback'])
  })

  it('the permission identifier set is exactly the four grants', () => {
    const ids = new Set<string>()
    for (const doc of readCapabilityDocs().values()) {
      for (const p of doc.permissions) {
        ids.add(typeof p === 'string' ? p : (p.identifier ?? ''))
      }
    }
    expect(sorted([...ids])).toEqual([
      'barcode-scanner:default',
      'core:default',
      'http:default',
      'websocket:default',
    ])
  })

  it('default.json (the shipped ACL) is ts.net only — no loopback', () => {
    const docs = readCapabilityDocs()
    const def = docs.get('default.json')
    expect(def).toBeDefined()
    expect(sorted(httpAllowUrls(def!))).toEqual(['https://*.ts.net:*'])
    // And the split is real: loopback lives only in dev-loopback.json.
    const dev = docs.get('dev-loopback.json')
    expect(dev).toBeDefined()
    expect(sorted(httpAllowUrls(dev!))).toEqual(['http://127.0.0.1:*', 'http://localhost:*'])
  })

  it('dev-loopback is pinned to non-iOS targets only', () => {
    const dev = readCapabilityDocs().get('dev-loopback.json')
    expect(dev?.platforms, 'dev-loopback must name its platforms').toBeDefined()
    // Spellings are tauri-utils' Target serde names (platform.rs): macOS
    // and iOS are camelCase, the rest lowercase. The security claim under
    // test is the absence of iOS — that is what keeps loopback out of the
    // App Store binary's ACL (Tauri has no dev/release capability split).
    expect(sorted(dev!.platforms!)).toEqual(['android', 'linux', 'macOS', 'windows'])
    expect(dev!.platforms!).not.toContain('iOS')
  })
})

describe('request(): a scope refusal is named, not swallowed', () => {
  it('throws endpoint_out_of_scope before dialing an out-of-scope endpoint', async () => {
    const calls: unknown[] = []
    const fn: FetchLike = async (url, init) => {
      calls.push({ url, init })
      return new Response('{}', { status: 200 })
    }
    await expect(
      request('auth/me/', {
        session: { endpoint: 'https://evil.example.com', token: null },
        dev: false,
        fetchFn: fn,
      }),
    ).rejects.toMatchObject({ code: 'endpoint_out_of_scope', status: 0 })
    expect(calls, 'the dial must never happen').toEqual([])
  })

  it('dials an 8443 tailnet endpoint instead of reporting it as network', async () => {
    const fn: FetchLike = async () => new Response('{}', { status: 200 })
    const res = await request('auth/me/', {
      session: { endpoint: 'https://h.example.ts.net:8443', token: null },
      dev: false,
      fetchFn: fn,
    })
    expect(res.status).toBe(200)
  })

  it('dev requests ride the proxy without a scope check (relative URL)', async () => {
    const { fn, calls } = (() => {
      const calls: { url: string; init: RequestInit }[] = []
      return {
        calls,
        fn: (async (url, init) => {
          calls.push({ url: url as string, init })
          return new Response('{}', { status: 200 })
        }) as FetchLike,
      }
    })()
    const res = await request('auth/me/', {
      session: { endpoint: 'https://evil.example.com', token: null },
      dev: true,
      fetchFn: fn,
    })
    expect(res.status).toBe(200)
    expect(calls[0].url).toBe('/api/v1/auth/me/')
  })

  it("the scope error's copy is not the network copy", () => {
    const scope = errorMessage(new ApiError('endpoint_out_of_scope', 0))
    expect(scope).not.toBe(errorMessage(new ApiError('network', 0)))
    expect(scope.length).toBeGreaterThan(10)
  })
})
