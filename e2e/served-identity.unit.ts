import { execFileSync } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test } from 'vitest'
import { assertServedArtifact, e2eServePort, servedStampPath } from './helpers'

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const ROOT = join(E2E_DIR, '..')
const PROBE = join(ROOT, 'web', 'src', '.gdk311-probe')
const DIGEST_SH = join(E2E_DIR, 'served-digest.sh')

function worktreeRoot(): string {
  return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd: ROOT, encoding: 'utf8' }).trim()
}

function sourceDigest(): string {
  return execFileSync('bash', [DIGEST_SH], { cwd: ROOT, encoding: 'utf8' }).trim()
}

function writeStamp(dir: string, stamp: { worktree: string; digest: string }): string {
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
    const line = sourceDigest()
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
  test('is keyed on the PORT assignment in e2e/serve.sh', () => {
    const serve = readFileSync(join(E2E_DIR, 'serve.sh'), 'utf8')
    const port = e2eServePort()
    expect(serve).toMatch(new RegExp(`^PORT=${port}$`, 'm'))
    expect(servedStampPath()).toBe(join(process.env.TMPDIR || '/tmp', `gadak-e2e-served-${port}.json`))
  })

  test('serve.sh and the guard share e2e/served-digest.sh', () => {
    const serve = readFileSync(join(E2E_DIR, 'serve.sh'), 'utf8')
    const helpers = readFileSync(join(E2E_DIR, 'helpers.ts'), 'utf8')
    expect(serve).toMatch(/served-digest\.sh/)
    expect(helpers).toMatch(/served-digest\.sh/)
    expect(helpers).not.toMatch(/assertServedGitSha/)
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
    const digest = sourceDigest()
    const other = '/tmp/gadak-e2e-other-worktree'
    const stampPath = writeStamp(tmp, { worktree: other, digest })
    const msg = thrownMessage(() => assertServedArtifact({ stampPath, root }))
    expect(msg).toContain(`stamp worktree ${other}`)
    expect(msg).toContain(`digest ${digest}`)
    expect(msg).toContain(`this worktree ${root}`)
    expect(msg).toContain(`digest ${digest}`)
    expect(msg).toContain(`pkill -f '${other}/e2e/.tmp/gadak'`)
  })

  test('passes when this worktree wrote the stamp and the tree is unmodified', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const digest = sourceDigest()
    const stampPath = writeStamp(tmp, { worktree: root, digest })
    expect(() => assertServedArtifact({ stampPath, root })).not.toThrow()
  })

  test('missing stamp names this worktree and digest', () => {
    tmp = mkdtempSync(join(tmpdir(), 'gdk311-'))
    const root = worktreeRoot()
    const digest = sourceDigest()
    const stampPath = join(tmp, 'missing.json')
    const msg = thrownMessage(() => assertServedArtifact({ stampPath, root }))
    expect(msg).toContain(stampPath)
    expect(msg).toContain(root)
    expect(msg).toContain(digest)
    expect(msg).toContain("pkill -f 'e2e/.tmp/gadak'")
  })
})
