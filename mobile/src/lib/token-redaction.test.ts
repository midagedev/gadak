import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  ApiError,
  apiHeaders,
  apiUrl,
  absoluteApiUrl,
  configureApi,
  errorMessage,
  request,
  requestBlob,
  type FetchLike,
} from './api'

/*
 * GDK-804, the phone half: the pairing token never reaches console output,
 * an error message, or anything a person can read off the device.
 *
 * secure.ts already says "the token never appears in a log or an error" in
 * prose. This makes it measurable, on the two layers the Go side uses:
 *
 *  ① Behavioral — drive the transport with a real token configured, across
 *    every failure mode it has (network throw, JSON error body, non-JSON
 *    error body, out-of-scope endpoint, blob failure, 304), with every
 *    console method spied. Assert nothing was printed carrying the token,
 *    and that the thrown ApiError's message/stack does not either. An
 *    ApiError is what the UI renders and what an unhandled rejection prints,
 *    so its text is console output one step removed.
 *
 *  ② Source — no console.* call anywhere in mobile/src (outside tests) takes
 *    a token-shaped identifier. ① only covers the paths it drives; a new
 *    module that logs its own session would sail past it. The webview has no
 *    log rotation and Safari's remote inspector shows the whole buffer, so
 *    "only in dev" is not a mitigation.
 *
 * The distinctive token below is a literal that cannot occur by accident, so
 * a substring hit is always a real leak and never a coincidence.
 */

const TOKEN = 'gdk-pairing-plaintext-DO-NOT-LOG-4f7a91c2'
const ENDPOINT = 'https://home.tailnet.example.com'

type ConsoleMethod = 'log' | 'info' | 'warn' | 'error' | 'debug' | 'trace'
const METHODS: ConsoleMethod[] = ['log', 'info', 'warn', 'error', 'debug', 'trace']

let printed: string[]

beforeEach(() => {
  printed = []
  for (const m of METHODS) {
    vi.spyOn(console, m).mockImplementation((...args: unknown[]) => {
      printed.push(args.map(render).join(' '))
    })
  }
  configureApi({ endpoint: ENDPOINT, token: TOKEN })
})

afterEach(() => {
  vi.restoreAllMocks()
  configureApi({ endpoint: '', token: null })
})

/** Renders an argument the way a console does — objects included, since a
 *  logged `{ session }` leaks exactly as loudly as a logged string. */
function render(v: unknown): string {
  if (v instanceof Error) return `${v.name}: ${v.message}\n${v.stack ?? ''}`
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

/** A fetch that answers with a fixed status and body. */
function answering(status: number, body: string | null, headers: Record<string, string> = {}): FetchLike {
  return async () => new Response(body, { status, headers })
}

/** Captures what a call printed and what it threw, as one searchable blob. */
async function drive(label: string, run: () => Promise<unknown>): Promise<string> {
  let thrown = ''
  try {
    await run()
  } catch (err) {
    thrown = render(err)
    if (err instanceof ApiError) thrown += ` code=${err.code} status=${err.status} msg=${err.serverMessage ?? ''}`
    thrown += ` ui=${errorMessage(err)}`
  }
  return `[${label}] printed=${printed.join(' | ')} thrown=${thrown}`
}

describe('GDK-804 — the pairing token never reaches the phone’s console', () => {
  // Each case is a way the transport can fail. dev:true keeps the vite-proxy
  // branch (no @tauri-apps/plugin-http import) while still attaching the
  // Bearer — apiHeaders does that on both branches.
  const cases: { name: string; run: () => Promise<unknown> }[] = [
    {
      name: 'the server is unreachable',
      run: () =>
        request('issues/bootstrap/', {
          dev: true,
          fetchFn: async () => {
            throw new Error(`connect ECONNREFUSED ${ENDPOINT}`)
          },
        }),
    },
    {
      name: 'a JSON error body',
      run: () =>
        request('issues/bootstrap/', {
          dev: true,
          fetchFn: answering(401, JSON.stringify({ error: 'pairing_rejected', reason: 'unknown' })),
        }),
    },
    {
      name: 'a non-JSON error body (a proxy page)',
      run: () => request('issues/bootstrap/', { dev: true, fetchFn: answering(502, '<html>bad gateway</html>') }),
    },
    {
      name: 'a 200 whose body is not JSON',
      run: () => request('issues/bootstrap/', { dev: true, fetchFn: answering(200, 'not json at all') }),
    },
    {
      name: 'a 403 on a write',
      run: () =>
        request('issues/NMB-1/comment/', {
          dev: true,
          method: 'POST',
          body: { text: 'hi' },
          fetchFn: answering(403, JSON.stringify({ error: 'forbidden_origin' })),
        }),
    },
    {
      name: 'a packaged endpoint outside the dial scope',
      run: () =>
        request('issues/bootstrap/', {
          dev: false,
          session: { endpoint: 'https://somewhere-else.example.com', token: TOKEN },
          fetchFn: answering(200, '{}'),
        }),
    },
    {
      name: 'attachment bytes that fail to read',
      run: () => requestBlob('issues/NMB-1/attachments/9/content/', { dev: true, fetchFn: answering(404, JSON.stringify({ error: 'not_found' })) }),
    },
    {
      name: 'a conditional GET answering 304',
      run: () => request('issues/bootstrap/', { dev: true, etag: 'W/"abc"', fetchFn: answering(304, null) }),
    },
    {
      name: 'a successful bootstrap',
      run: () => request('issues/bootstrap/', { dev: true, fetchFn: answering(200, JSON.stringify({ issues: [] })) }),
    },
  ]

  for (const c of cases) {
    it(`is silent about the token when ${c.name}`, async () => {
      const seen = await drive(c.name, c.run)
      expect(seen).not.toContain(TOKEN)
      // The transport prints nothing at all today. Asserted separately from
      // the token probe so a future round that adds a legitimate diagnostic
      // gets a clear, separate failure to think about rather than a silent
      // widening of what "no leak" means.
      expect(printed).toEqual([])
    })
  }

  it('still puts the Bearer on the wire — the redaction is of output, not of auth', () => {
    // The counterweight: a test that only asserts absence stays green if the
    // token stops being sent at all, which would be a much worse bug.
    expect(apiHeaders(TOKEN, false)['Authorization']).toBe(`Bearer ${TOKEN}`)
    expect(apiHeaders(null, false)['Authorization']).toBeUndefined()
  })

  it('keeps the token out of the URLs a person can copy', () => {
    // GDK-1503 makes an attachment URL copyable to anywhere. If the token
    // ever rode a query parameter, that tap would paste the pairing.
    const paths = ['issues/bootstrap/', 'issues/NMB-1/attachments/9/content/']
    for (const p of paths) {
      expect(apiUrl(ENDPOINT, p, false)).not.toContain(TOKEN)
      expect(apiUrl(ENDPOINT, p, true)).not.toContain(TOKEN)
      expect(absoluteApiUrl(p, { endpoint: ENDPOINT, dev: false, origin: 'http://127.0.0.1:5182' })).not.toContain(TOKEN)
      expect(absoluteApiUrl(p, { endpoint: ENDPOINT, dev: true, origin: 'http://127.0.0.1:5182' })).not.toContain(TOKEN)
    }
  })

  it('maps every refusal to copy that names no credential', () => {
    for (const code of ['pairing_rejected', 'scope_rejected', 'forbidden_host', 'forbidden_origin', 'network']) {
      const msg = errorMessage(new ApiError(code, 401))
      expect(msg).not.toContain(TOKEN)
      expect(msg).not.toMatch(/Bearer/i)
    }
  })
})

/* ── ② the source rule ── */

const SRC = join(dirname(fileURLToPath(import.meta.url)), '..')

/** Every non-test source file under mobile/src. */
function sources(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) {
      sources(full, out)
      continue
    }
    if (!/\.(ts|svelte)$/.test(name) || /\.test\.ts$/.test(name)) continue
    out.push(full)
  }
  return out
}

/**
 * Names that hold, or plausibly hold, a credential. `session` is included
 * because ApiSession carries the token as a field, so `console.log(session)`
 * prints it even though the word "token" never appears in that line.
 */
const CREDENTIAL_IDENT = /\b(token|bearer|credential|secret|apiKey|session|pairing)\b/i

/**
 * Matches a console call and the balance-aware slice of its arguments. The
 * Go side of this gate learned the hard way that a single-line matcher
 * misses gofmt-wrapped calls; prettier wraps long calls here for the same
 * reason, so the scan follows parens across lines.
 */
function consoleCalls(src: string): { line: number; text: string }[] {
  const lines = src.split('\n')
  const found: { line: number; text: string }[] = []
  let depth = 0
  let start = 0
  let buf = ''
  for (let i = 0; i < lines.length; i++) {
    let line = lines[i]!
    const trimmed = line.trim()
    if (trimmed.startsWith('//') || trimmed.startsWith('*')) continue
    if (depth === 0) {
      const m = /\bconsole\s*\.\s*\w+\s*\(/.exec(line)
      if (!m) continue
      start = i + 1
      buf = ''
      line = line.slice(m.index + m[0].length - 1)
    }
    buf += trimmed + ' '
    depth += (line.match(/\(/g)?.length ?? 0) - (line.match(/\)/g)?.length ?? 0)
    if (depth <= 0) {
      depth = 0
      found.push({ line: start, text: buf.trim() })
    }
  }
  return found
}

describe('GDK-804 — no phone source logs a credential', () => {
  const files = sources(SRC)

  it('reads the whole phone source tree', () => {
    // A glob that matched nothing would make the rule below a green no-op.
    expect(files.length).toBeGreaterThan(20)
  })

  it('has no console call taking a token-shaped identifier', () => {
    const offenders: string[] = []
    for (const file of files) {
      for (const call of consoleCalls(readFileSync(file, 'utf8'))) {
        if (CREDENTIAL_IDENT.test(call.text)) {
          offenders.push(`${relative(SRC, file)}:${call.line} — ${call.text}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })
})
