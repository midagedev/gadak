import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { basename, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test } from 'vitest'
import { assertServedArtifact, e2eServePort, servedStampPath } from './helpers'

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const ROOT = join(E2E_DIR, '..')
const PROBE = join(ROOT, 'web', 'src', '.gdk311-probe')
const DIGEST_SH = join(E2E_DIR, 'served-digest.sh')
const PORT_HELD = join(E2E_DIR, 'port-held.mjs')

function worktreeRoot(): string {
  return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd: ROOT, encoding: 'utf8' }).trim()
}

function sourceDigest(): string {
  const git = execFileSync('bash', [DIGEST_SH], { cwd: ROOT, encoding: 'utf8' }).trim()
  // Mirrors e2e/serve.sh: the stamp digest is the git line plus the fixture
  // being served (`seed=<basename>`; GADAK_SEED_DB defaults to demo.db). The
  // suite's own GADAK_E2E_SHELL never set here, so no shell= suffix either.
  const seed = basename(process.env.GADAK_SEED_DB ?? 'examples/demo.db')
  return `${git} seed=${seed}`
}

/**
 * A digest pinned for message-shape tests (GDK-723): what a mismatch names
 * does not depend on *which* digest mismatches, and every real digest is a
 * bash spawn of several git commands plus a sha256. Passed as both the
 * stamp's digest and the assert's expected digest, so shape tests prove the
 * mismatch is about the thing under test — worktree, pid — and not a digest
 * that happened to drift between two spawns.
 */
const FROZEN_DIGEST = 'frozen-digest-shape-only seed=demo.db'

function writeStamp(
  dir: string,
  stamp: { worktree: string; digest: string; pid?: number },
): string {
  const path = join(dir, 'stamp.json')
  writeFileSync(path, JSON.stringify(stamp) + '\n')
  return path
}

function thrownMessage(fn: () => void): string {
  let msg = ''
  expect(() => {
    try {
      fn()
    } catch (e) {
      msg = e instanceof Error ? e.message : String(e)
      throw e
    }
  }).toThrow(/stale/)
  return msg
}

describe('served-digest.sh', () => {
  afterEach(() => {
    if (existsSync(PROBE)) unlinkSync(PROBE)
  })

  test('prints HEAD plus a sha256 of the working-tree delta', () => {
    // The script's own contract is the bare git line; the `seed=` suffix is
    // serve.sh's composition, asserted through sourceDigest() below.
    const line = execFileSync('bash', [DIGEST_SH], { cwd: ROOT, encoding: 'utf8' }).trim()
    const head = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: ROOT, encoding: 'utf8' }).trim()
    expect(line).toMatch(new RegExp(`^${head} [0-9a-f]{64}$`))
  })

  test('changes when a build-input path is dirty', () => {
    const before = sourceDigest()
    writeFileSync(PROBE, 'uncommitted edit')
    const after = sourceDigest()
    expect(after).not.toBe(before)
    expect(after.slice(0, 40)).toBe(before.slice(0, 40))
  })
})

describe('servedStampPath', () => {
  test('is keyed on GADAK_E2E_PORT (default 7877)', () => {
    const serve = readFileSync(join(E2E_DIR, 'serve.sh'), 'utf8')
    const port = e2eServePort()
    expect(serve).toMatch(/PORT="\$\{GADAK_E2E_PORT:-7877\}"/)
    expect(port).toBe(process.env.GADAK_E2E_PORT || '7877')
    expect(servedStampPath()).toBe(join(process.env.TMPDIR || '/tmp', `gadak-e2e-served-${port}.json`))
  })

  test('serve.sh and the guard share e2e/served-digest.sh', () => {
    const serve = readFileSync(join(E2E_DIR, 'serve.sh'), 'utf8')
    const helpers = readFileSync(join(E2E_DIR, 'helpers.ts'), 'utf8')
    expect(serve).toMatch(/served-digest\.sh/)
    expect(helpers).toMatch(/served-digest\.sh/)
    expect(helpers).not.toMatch(/assertServedGitSha/)
  })

  // GDK-1757 ①③: the port probe must run before anything expensive — the
  // failure it reports is worth exactly nothing after a build has already
  // burned minutes — and the stamp must carry the serve pid the mismatch
  // message then names. Wiring assertions, because the behavior lives in a
  // shell script no other test can drive.
  test('serve.sh probes the port before building and stamps the serve pid (GDK-1757)', () => {
    const serve = readFileSync(join(E2E_DIR, 'serve.sh'), 'utf8')
    const probe = serve.indexOf('port-held.mjs')
    const build = serve.indexOf('go build')
    expect(probe).toBeGreaterThanOrEqual(0)
    expect(build).toBeGreaterThan(probe)
    expect(serve).toMatch(/STAMP_PID="\$\$"/)
  })

  // The wiring tests above read serve.sh as text, so a quote-unbalancing
  // edit inside its embedded node script passed them all and only surfaced
  // as the webServer dying with exit 126, minutes later, after the build.
  // An apostrophe in a shell-quoted heredoc comment is a one-character
  // failure mode; `bash -n` is the one-line owner of that whole class.
  test('serve.sh parses as shell (bash -n)', () => {
    const res = spawnSync('bash', ['-n', join(E2E_DIR, 'serve.sh')])
    expect(
      res.status,
      `bash -n rejected e2e/serve.sh:\n${res.stderr.toString()}`,
    ).toBe(0)
  })
})

describe('assertServedArtifact', () => {
  let tmp = ''

  afterEach(() => {
    if (existsSync(PROBE)) unlinkSync(PROBE)
    if (tmp) rmSync(tmp, { recursive: true, force: true })
    tmp = ''
  })

  test('throws when a build-input file is modified after the stamp (hole 1)', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const before = sourceDigest()
    const stampPath = writeStamp(tmp, { worktree: root, digest: before })
    writeFileSync(PROBE, 'uncommitted edit')
    const after = sourceDigest()
    expect(after).not.toBe(before)
    const msg = thrownMessage(() => assertServedArtifact({ stampPath, root }))
    expect(msg).toContain(`stamp worktree ${root}`)
    expect(msg).toContain(`digest ${before}`)
    expect(msg).toContain(`this worktree ${root}`)
    expect(msg).toContain(`digest ${after}`)
    expect(msg).toContain(`pkill -f '${root}/e2e/.tmp/gadak'`)
  })

  test('throws when the stamp worktree is not this worktree (hole 2)', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const other = '/tmp/gadak-e2e-other-worktree'
    const stampPath = writeStamp(tmp, { worktree: other, digest: FROZEN_DIGEST })
    const msg = thrownMessage(() =>
      assertServedArtifact({ stampPath, root, digest: FROZEN_DIGEST }),
    )
    // The digest is pinned identical on both sides: the mismatch is the
    // worktree alone — two worktrees at the same commit, the exact shape a
    // parallel round produces.
    expect(msg).toContain(`stamp worktree ${other}`)
    expect(msg).toContain(`this worktree ${root}`)
    expect(msg).toContain(`pkill -f '${other}/e2e/.tmp/gadak'`)
  })

  test('a stamp with a live pid names it and the kill command (GDK-1757 ②③)', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const other = '/tmp/gadak-e2e-other-worktree'
    // This test process is running, so the stamp's pid is a live one.
    const stampPath = writeStamp(tmp, { worktree: other, digest: FROZEN_DIGEST, pid: process.pid })
    const msg = thrownMessage(() =>
      assertServedArtifact({ stampPath, root, digest: FROZEN_DIGEST }),
    )
    expect(msg).toContain(`pid ${process.pid} (running)`)
    expect(msg).toContain(`kill ${process.pid}`)
  })

  test('a stamp whose pid has died says so and points at lsof (GDK-1757 ②③)', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const other = '/tmp/gadak-e2e-other-worktree'
    // A reaped child's pid is not running (recycling within milliseconds of
    // exit is not a real risk on any host this suite targets).
    const dead = spawnSync(process.execPath, ['-e', ''])
    expect(dead.pid).toBeTruthy()
    const stampPath = writeStamp(tmp, {
      worktree: other,
      digest: FROZEN_DIGEST,
      pid: dead.pid as number,
    })
    const msg = thrownMessage(() =>
      assertServedArtifact({ stampPath, root, digest: FROZEN_DIGEST, port: '8123' }),
    )
    expect(msg).toContain(`pid ${dead.pid} is no longer running`)
    expect(msg).toContain('lsof -nP -iTCP:8123 -sTCP:LISTEN')
  })

  test('passes when this worktree wrote the stamp and the digests agree', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const stampPath = writeStamp(tmp, { worktree: root, digest: FROZEN_DIGEST })
    expect(() => assertServedArtifact({ stampPath, root, digest: FROZEN_DIGEST })).not.toThrow()
  })

  test('missing stamp names this worktree and digest', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const stampPath = join(tmp, 'missing.json')
    const msg = thrownMessage(() =>
      assertServedArtifact({ stampPath, root, digest: FROZEN_DIGEST }),
    )
    expect(msg).toContain(stampPath)
    expect(msg).toContain(root)
    expect(msg).toContain(FROZEN_DIGEST)
    expect(msg).toContain("pkill -f 'e2e/.tmp/gadak'")
  })
})

describe('port-held.mjs (GDK-1757 ①③)', () => {
  let tmp = ''
  let server: ReturnType<typeof createServer> | null = null

  afterEach(() => {
    server?.close()
    server = null
    if (tmp) rmSync(tmp, { recursive: true, force: true })
    tmp = ''
  })

  function run(args: string[], env: NodeJS.ProcessEnv = process.env) {
    return spawnSync(process.execPath, [PORT_HELD, ...args], {
      encoding: 'utf8',
      env: { ...env, TMPDIR: tmp || env.TMPDIR },
      timeout: 15_000,
    })
  }

  test('exit 0 and silent when nothing listens', async () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-free-'))
    // Reserve then release an ephemeral port: the OS hands back a port that
    // was just free, so the probe should see exactly that.
    const held = await new Promise<number>((resolve) => {
      server = createServer()
      server.listen(0, '127.0.0.1', () => resolve((server?.address() as { port: number }).port))
    })
    await new Promise<void>((resolve) => server?.close(() => resolve()))
    const res = run([String(held)])
    expect(res.status).toBe(0)
    expect(res.stderr.trim()).toBe('')
  })

  test('exit 1 naming port, pid and the foreign worktree when a listener holds it', async () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-held-'))
    const port = await new Promise<number>((resolve) => {
      server = createServer()
      server.listen(0, '127.0.0.1', () => resolve((server?.address() as { port: number }).port))
    })
    const other = '/tmp/gadak-e2e-other-worktree'
    writeFileSync(
      join(tmp, `gadak-e2e-served-${port}.json`),
      JSON.stringify({ worktree: other, digest: FROZEN_DIGEST, pid: 4_194_303 }) + '\n',
    )
    const res = run([String(port)])
    expect(res.status).toBe(1)
    expect(res.stderr).toContain(`port ${port} is already listening`)
    expect(res.stderr).toContain('GDK-1757')
    expect(res.stderr).toContain(`worktree ${other}`)
    expect(res.stderr).toContain(`pkill -f '${other}/e2e/.tmp/gadak'`)
  })

  test('exit 2 on a malformed port argument', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-arg-'))
    expect(run(['not-a-port']).status).toBe(2)
    expect(run([]).status).toBe(2)
  })
})
