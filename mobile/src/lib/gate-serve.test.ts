// GDK-1540 — a *config contract*, not a behaviour test.
//
// It asserts three things no running server is needed to prove: that the two
// Playwright configs in mobile/ take their ports from one owner
// (mobile/e2e/serve.ts) rather than spelling them inline, that neither of
// them starts a dev server any more, and that the provenance check refuses a
// server this worktree did not build. mobile/e2e/reload-isolation.spec.ts is
// the other half — it proves the behaviour against a live server.
//
// The failure this closes is silent: before it, playwright.config.ts and
// shots.config.ts each hardcoded 5182/7899 with `reuseExistingServer: true`,
// so two gates shared one server and one of them was quietly looking at the
// other's tree. Nothing was red. A drift back to that — one config edited,
// the other not — has to be caught by a test, because it cannot be caught by
// looking at a green run.
import { describe, expect, it } from 'vitest'
import gateConfig from '../../playwright.config'
import shotsConfig from '../../shots.config'
import {
  BUNDLE_STAMP_FILE,
  apiBinPath,
  apiStampPath,
  checkStamp,
  gateOutDir,
  mobileAPIPort,
  mobileUIPort,
  parseStamp,
  stampText,
  type GateStamp,
} from '../../e2e/serve'

const CONFIGS: Array<[string, typeof gateConfig]> = [
  ['playwright.config.ts', gateConfig],
  ['shots.config.ts', shotsConfig],
]

function webServers(config: typeof gateConfig) {
  const ws = config.webServer
  expect(Array.isArray(ws), 'webServer must be the shared array from e2e/serve.ts').toBe(true)
  return ws as Array<{ url?: string; command: string; env?: Record<string, string> }>
}

describe('both Playwright configs read one owner for the gate servers', () => {
  it.each(CONFIGS)('%s serves the UI on the owner port', (_name, config) => {
    const ui = webServers(config).find((s) => s.url?.includes(`:${mobileUIPort()}`))
    expect(ui, `no webServer on the UI port ${mobileUIPort()}`).toBeTruthy()
    expect(ui?.url).toBe(`http://127.0.0.1:${mobileUIPort()}/${BUNDLE_STAMP_FILE}`)
    expect(ui?.env?.GADAK_MOBILE_E2E_PORT).toBe(mobileUIPort())
    expect(ui?.env?.GADAK_MOBILE_GATE_OUTDIR).toBe(gateOutDir())
  })

  it.each(CONFIGS)('%s serves the API on the owner port', (_name, config) => {
    const api = webServers(config).find((s) => s.url?.includes(`:${mobileAPIPort()}`))
    expect(api, `no webServer on the API port ${mobileAPIPort()}`).toBeTruthy()
    expect(api?.url).toBe(`http://127.0.0.1:${mobileAPIPort()}/healthz`)
    expect(api?.env?.GADAK_MOBILE_API_PORT).toBe(mobileAPIPort())
    expect(api?.env?.GADAK_MOBILE_API_STAMP).toBe(apiStampPath())
    expect(api?.env?.GADAK_MOBILE_API_BIN).toBe(apiBinPath())
  })

  // The whole point of GDK-1540: no watcher in the gate path. `npx vite` with
  // no subcommand *is* the dev server, and that is what both configs used to
  // run. Everything now goes through e2e/gate-serve.sh, which builds first.
  it.each(CONFIGS)('%s starts no dev server', (_name, config) => {
    for (const server of webServers(config)) {
      expect(server.command).toContain('gate-serve.sh')
      expect(server.command).not.toMatch(/\bvite\b(?!\s+(build|preview))/)
    }
  })

  it.each(CONFIGS)('%s runs the provenance check before the specs', (_name, config) => {
    expect(String(config.globalSetup)).toMatch(/e2e\/serve\.ts$/)
  })

  // Two gates in one tree still share mobile/test-results unless the output
  // dir is keyed too, and Playwright empties that dir at start — one gate
  // would delete the other's traces mid-run.
  it.each(CONFIGS)('%s keys its output dir on the ports', (_name, config) => {
    expect(String(config.outputDir)).toContain(`${mobileUIPort()}-${mobileAPIPort()}`)
  })

  it('serves the bundle from outside mobile/dist, which tauri packages', () => {
    // tauri.conf.json frontendDist is "../dist"; a gate artifact parked
    // there would ride into a packaged build.
    expect(gateOutDir()).not.toContain('/mobile/dist')
    expect(gateOutDir()).toContain(mobileUIPort())
  })
})

describe('the ports come from the environment', () => {
  it('defaults to today values when unset', () => {
    expect(mobileUIPort({})).toBe('5182')
    expect(mobileAPIPort({})).toBe('7899')
  })

  it('takes GADAK_MOBILE_E2E_PORT / GADAK_MOBILE_API_PORT', () => {
    expect(mobileUIPort({ GADAK_MOBILE_E2E_PORT: '5191' })).toBe('5191')
    expect(mobileAPIPort({ GADAK_MOBILE_API_PORT: '7931' })).toBe('7931')
  })

  it('still honours GADAK_SERVE_PORT, the older name for the proxy target', () => {
    expect(mobileAPIPort({ GADAK_SERVE_PORT: '7801' })).toBe('7801')
    expect(mobileAPIPort({ GADAK_SERVE_PORT: '7801', GADAK_MOBILE_API_PORT: '7931' })).toBe('7931')
  })

  it('rejects a value that is not a port', () => {
    expect(() => mobileUIPort({ GADAK_MOBILE_E2E_PORT: 'abc' })).toThrow(/must be an integer/)
    expect(() => mobileAPIPort({ GADAK_MOBILE_API_PORT: '70000' })).toThrow(/must be an integer/)
  })
})

const MINE: GateStamp = {
  role: 'ui',
  worktree: '/repo/gadak',
  head: 'a'.repeat(40),
  dirty: true,
  digest: 'deadbeef',
  builtAt: 2_000,
}

const check = (served: GateStamp, expected: GateStamp = MINE, processStartMs = 1_000) =>
  checkStamp({ expected, served, where: 'test', remedy: 'stop it', processStartMs })

describe('the stamp check refuses a server this worktree did not build', () => {
  it('accepts its own', () => {
    expect(() => check({ ...MINE })).not.toThrow()
  })

  it('refuses another worktree', () => {
    expect(() => check({ ...MINE, worktree: '/repo/gadak-other' })).toThrow(/another worktree/)
  })

  it('refuses another commit', () => {
    expect(() => check({ ...MINE, head: 'b'.repeat(40) })).toThrow(/another commit/)
  })

  it('refuses the wrong role', () => {
    expect(() => check({ ...MINE, role: 'api' })).toThrow(/role api/)
  })

  // builtAt < processStartMs means reuseExistingServer adopted a server that
  // was already up: its bundle predates this run, so its sources must match.
  it('refuses an adopted server whose sources differ', () => {
    expect(() => check({ ...MINE, builtAt: 500, digest: 'cafe' })).toThrow(/predates this run/)
  })

  it('refuses an adopted server with no stamp time at all', () => {
    const { builtAt: _drop, ...noTime } = MINE
    expect(() => check({ ...noTime, digest: 'cafe' } as GateStamp)).toThrow(/predates this run/)
  })

  // The tolerance that buys the isolation: we built this bundle at 2000, the
  // run started at 1000, and an agent edited a screen at 2500. The bytes the
  // browser holds are still ours — failing here would hand the flake back.
  it('tolerates the tree moving under a bundle it built itself', () => {
    expect(() => check({ ...MINE, digest: 'moved-after-the-build' })).not.toThrow()
  })

  it('refuses a body that is not a stamp', () => {
    expect(() => parseStamp('<!doctype html>', 'x')).toThrow(/not JSON/)
    expect(() => parseStamp('{"role":"ui"}', 'x')).toThrow(/not a gate stamp/)
  })

  // builtAt crosses a shell environment variable on its way out of
  // gate-serve.sh, and once arrived as a string. `typeof x === 'number'`
  // then read every bundle as adopted, which re-armed the exact flake this
  // ticket removes: an agent saving a screen mid-run moved the digest and
  // globalSetup failed the gate. Both shapes must mean the same number.
  it('reads builtAt as a number whether the server wrote one or a string', () => {
    const body = (at: string) =>
      `{"role":"ui","worktree":"/w","head":"abc","dirty":true,"digest":"d","builtAt":${at}}`
    expect(parseStamp(body('1788779216886'), 'x').builtAt).toBe(1788779216886)
    expect(parseStamp(body('"1788779216886"'), 'x').builtAt).toBe(1788779216886)
    expect(parseStamp(body('"not-a-time"'), 'x').builtAt).toBeUndefined()
  })

  it('tolerates the tree moving under a bundle whose builtAt came as a string', () => {
    const served = parseStamp(
      `{"role":"ui","worktree":"${MINE.worktree}","head":"${MINE.head}","dirty":true,"digest":"moved","builtAt":"2000"}`,
      'x',
    )
    expect(() => check(served)).not.toThrow()
  })
})

describe('the line the gate prints', () => {
  it('reads <sha>+dirty on a dirty tree and bare sha on a clean one', () => {
    expect(stampText(MINE)).toBe(`${'a'.repeat(12)}+dirty`)
    expect(stampText({ ...MINE, dirty: false })).toBe('a'.repeat(12))
  })
})
