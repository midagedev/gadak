import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, request, errorMessage, type FetchLike } from './api'
import { inDialScope, loopbackAllowedHere } from './dial-scope'

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
    // `true` is the UNION axis — every allow entry in the corpus, the
    // whole truth a non-iOS target enforces (and what a caller outside a
    // tauri webview has no native reason to narrow). The per-platform
    // split is the GDK-2009 block below.
    for (const [url, expected] of TABLE) {
      expect(allowedByCapability(url), `capability verdict for ${url}`).toBe(expected)
      expect(inDialScope(url, true), `predicate verdict for ${url}`).toBe(expected)
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

/*
 * GDK-2009 gate: the platform axis. The corpus is a set of files, each
 * carrying its own platform list — default.json has none (every target),
 * dev-loopback.json rides its `platforms` array — and the ACL a target
 * actually enforces is the union of the files that name it. The predicate
 * must predict THAT, not the union of everything in the directory: on iOS
 * the shipped ACL is default.json alone, so the front refuses loopback
 * before the dial instead of passing it and folding the native refusal
 * into 'network' after (the defect this round closed). Everything here is
 * derived from the files — the platforms array is read, never restated,
 * so adding or removing an item in dev-loopback.json moves these rows
 * with it.
 */
describe('dial scope: the platform axis (GDK-2009)', () => {
  /** The allow set one target's ACL carries: a file with no `platforms`
   *  key rides every target; a file with one rides only where named. */
  const allowUrlsFor = (platform: string): string[] => {
    const urls: string[] = []
    for (const doc of readCapabilityDocs().values()) {
      if (doc.platforms && !doc.platforms.includes(platform)) continue
      urls.push(...httpAllowUrls(doc))
    }
    return urls
  }

  const loopbackPlatforms = (): string[] =>
    readCapabilityDocs().get('dev-loopback.json')?.platforms ?? []

  // iOS is under test precisely because the array does not name it. The
  // set is derived per call so a platforms edit cannot leave it stale.
  const platformsUnderTest = (): string[] => [...new Set([...loopbackPlatforms(), 'iOS'])]

  // One row per verdict class: tailnet names (in scope on every axis),
  // out-of-scope shapes (out on every axis), and the loopback rows where
  // the axis IS the verdict.
  const AXIS_ROWS = [
    'https://h.ts.net:8443/',
    'https://deep.home.example.ts.net:10000/',
    'http://h.ts.net:8443/',
    'https://ts.net.evil.com/',
    'http://[::1]:7877/',
    'not a url',
    'http://127.0.0.1:7777/',
    'http://localhost:5173/',
  ]

  it("every platform: TS verdict == that platform's ACL, verdict for verdict", () => {
    for (const platform of platformsUnderTest()) {
      const axis = loopbackPlatforms().includes(platform)
      const acl = (url: string): boolean =>
        allowUrlsFor(platform).some((p) => capabilityVerdict(p, url))
      for (const url of AXIS_ROWS) {
        expect(inDialScope(url, axis), `${platform} ${url}`).toBe(acl(url))
      }
    }
  })

  it('iOS refuses the loopback set before the dial — the rows GDK-2009 exists for', () => {
    const axis = loopbackPlatforms().includes('iOS')
    const acl = (url: string): boolean => allowUrlsFor('iOS').some((p) => capabilityVerdict(p, url))
    for (const url of [
      'http://127.0.0.1:7777/', // exactly what a dev pairing holds (the vite proxy port)
      'http://localhost:7777/',
      'http://localhost/', // loopback host on the scheme's default port
      'http://LOCALHOST:7777/', // the URL parser lowercases; loopback is loopback
      'https://127.0.0.1:8443/', // right host, wrong scheme — out of scope on every axis
    ]) {
      expect(inDialScope(url, axis), url).toBe(acl(url))
    }
  })
})

describe('loopbackAllowedHere(): the platform arm at runtime (GDK-2009)', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // hasTauri() (lib/runtime.ts) reads the window for the bridge marker;
  // Node has neither window nor navigator until a test stubs them.
  const inTauriWebview = (): void => {
    vi.stubGlobal('window', { __TAURI_INTERNALS__: {} })
  }

  it('outside a tauri webview there is no native ACL to predict: the union', () => {
    // The dev vite proxy and the hosted /m/ page never consult the judge
    // (their scope checks are skipped first); a direct caller asking
    // corpus questions gets the corpus union, and the axis splits live in
    // the predicate's parameter.
    expect(loopbackAllowedHere()).toBe(true)
  })

  it('inside the webview an iPhone/iPad UA refuses loopback — iOS is the narrow axis', () => {
    inTauriWebview()
    vi.stubGlobal('navigator', {
      userAgent:
        'Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/22C152',
    })
    expect(loopbackAllowedHere()).toBe(false)
    vi.stubGlobal('navigator', {
      userAgent:
        'Mozilla/5.0 (iPad; CPU OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/22C152',
    })
    expect(loopbackAllowedHere()).toBe(false)
  })

  it('a UA that names a dev-loopback platform keeps loopback', () => {
    inTauriWebview()
    vi.stubGlobal('navigator', {
      userAgent:
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/620.1.14 (KHTML, like Gecko)',
    })
    expect(loopbackAllowedHere()).toBe(true)
    vi.stubGlobal('navigator', {
      userAgent:
        'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
    })
    expect(loopbackAllowedHere()).toBe(true)
  })

  it('an unreadable UA inside the webview refuses loopback — narrow is the default', () => {
    // The failure direction the round spec pinned: inside the packaged
    // webview the judge may never answer loopback WIDE on missing
    // evidence. (The reverse miss — refusing loopback on some non-iOS UA
    // spelling — costs a dev build a named refusal, not a wrong dial.)
    inTauriWebview()
    vi.stubGlobal('navigator', { userAgent: '' })
    expect(loopbackAllowedHere()).toBe(false)
    vi.stubGlobal('navigator', undefined)
    expect(loopbackAllowedHere()).toBe(false)
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

  // 2026-09-10 — GDK-873: deep-link:default joined the set. Widened
  // deliberately, not to make a red test green: the grant lets JS subscribe
  // to URLs iOS has ALREADY routed to this bundle (the routing itself is
  // CFBundleURLTypes in src-tauri/Info.ios.plist), so it adds no dialable
  // destination — which is why the allow-URL union assertion above is
  // unchanged and still lists three entries. The exhaustive shape of this
  // list is the point: a grant that arrives without a line of reasoning
  // here turns it red.
  //
  // 2026-09-11 — GDK-897: websocket:default LEFT the set, and this test was
  // the red that proved the removal shipped. That grant was the hole: the
  // websocket plugin's permission carried no URL allowlist at all, so
  // anything in the webview could dial any host — the shell socket now goes
  // through the shell_ws_* Rust commands (src-tauri/src/shell.rs), which
  // validate against this same corpus's http list before connecting and
  // need no webview permission to do it. Narrowed, not re-widened: the
  // remaining four grants are unchanged, and the shell dial's own scope
  // gate is shell.rs's endpoint_in_scope tests plus the verdict table above.
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
      'deep-link:default',
      'http:default',
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
