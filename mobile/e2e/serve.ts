// Single owner for the phone gate's servers: the two ports, the directory
// the built bundle is served from, and the stamp that proves the thing
// answering on those ports came from *this* worktree (GDK-1540).
//
// Both Playwright configs in mobile/ — playwright.config.ts (the gate) and
// shots.config.ts (the capture harness) — read this file and nothing else.
// Before it existed they each spelled `5182` and `7899` inline with
// `reuseExistingServer: true`, which meant two gates in two worktrees
// silently shared one server: whichever started first, serving its own
// tree's code to both. A gate that photographs another tree is worse than a
// gate that fails, so the ports are now env-openable and the stamp check
// below refuses an adopted server that cannot prove its provenance.
//
// The contract is the desktop suite's, ported: e2e/serve.sh writes a
// port-keyed stamp and e2e/helpers.ts assertServedArtifact() reads it back
// (GDK-672). The difference here is that the UI stamp travels *inside the
// served bundle* rather than in a side file, so what is checked is what the
// browser is actually being handed.
import type { PlaywrightTestConfig } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const E2E_DIR = dirname(fileURLToPath(import.meta.url))
const MOBILE_DIR = dirname(E2E_DIR)
const REPO_ROOT = dirname(MOBILE_DIR)
const GATE_SERVE_SH = join(E2E_DIR, 'gate-serve.sh')

/** Today's ports, kept as the unset default so an existing invocation still works. */
const DEFAULT_UI_PORT = '5182'
const DEFAULT_API_PORT = '7899'

/** The stamp the build drops into the bundle; served at the origin root. */
export const BUNDLE_STAMP_FILE = 'gadak-gate-stamp.json'

function readPort(name: string, raw: string | undefined, fallback: string): string {
  if (raw === undefined || raw === '') return fallback
  if (!/^[1-9][0-9]*$/.test(raw) || Number(raw) > 65535) {
    throw new Error(`${name} must be an integer 1-65535, got ${JSON.stringify(raw)}`)
  }
  return raw
}

/**
 * UI port: GADAK_MOBILE_E2E_PORT, default 5182.
 *
 * 5182 and not 5180, because 5180 is `npm run dev` and a developer's loop
 * must survive a gate run. Two gates that must not collide each set this.
 */
export function mobileUIPort(env: NodeJS.ProcessEnv = process.env): string {
  return readPort('GADAK_MOBILE_E2E_PORT', env.GADAK_MOBILE_E2E_PORT, DEFAULT_UI_PORT)
}

/**
 * API port: GADAK_MOBILE_API_PORT, default 7899.
 *
 * GADAK_SERVE_PORT is honoured as the older name for the same thing — it was
 * mobile/vite.config.ts's knob for the dev proxy target (mobile/DESIGN.md §9)
 * and pointing the proxy somewhere the gate is not would be a silent
 * mis-serve. GADAK_MOBILE_API_PORT wins when both are set.
 */
export function mobileAPIPort(env: NodeJS.ProcessEnv = process.env): string {
  const raw = env.GADAK_MOBILE_API_PORT || env.GADAK_SERVE_PORT
  const name = env.GADAK_MOBILE_API_PORT ? 'GADAK_MOBILE_API_PORT' : 'GADAK_SERVE_PORT'
  return readPort(name, raw, DEFAULT_API_PORT)
}

export function mobileUIOrigin(env: NodeJS.ProcessEnv = process.env): string {
  return `http://127.0.0.1:${mobileUIPort(env)}`
}

export function mobileAPIOrigin(env: NodeJS.ProcessEnv = process.env): string {
  return `http://127.0.0.1:${mobileAPIPort(env)}`
}

/** The specs import these two from ../playwright.config; keep the names. */
export const UI_ORIGIN = mobileUIOrigin()
export const SERVE_ORIGIN = mobileAPIOrigin()

function tmpDir(env: NodeJS.ProcessEnv = process.env): string {
  return env.TMPDIR || '/tmp'
}

/**
 * Where the gate's bundle is built and served from.
 *
 * Under TMPDIR, not mobile/dist: tauri.conf.json frontendDist is "../dist",
 * so a gate artifact parked there would ride into a packaged build. Keyed by
 * UI port so two gates never write each other's bytes, and re-emptied on
 * every build so nothing survives a run it did not belong to.
 */
export function gateOutDir(env: NodeJS.ProcessEnv = process.env): string {
  return join(tmpDir(env), `gadak-mobile-gate-${mobileUIPort(env)}`)
}

/** The demo binary and its stamp, port-keyed for the same reason. */
export function apiBinPath(env: NodeJS.ProcessEnv = process.env): string {
  return join(tmpDir(env), `gadak-mobile-api-${mobileAPIPort(env)}`)
}

export function apiStampPath(env: NodeJS.ProcessEnv = process.env): string {
  return join(tmpDir(env), `gadak-mobile-api-${mobileAPIPort(env)}.json`)
}

export type GateRole = 'ui' | 'api'

export type GateStamp = {
  role: GateRole
  worktree: string
  head: string
  dirty: boolean
  digest: string
  /** epoch ms the server wrote it; absent means "not written by gate-serve.sh". */
  builtAt?: number
  /** the bundle dir (ui) or binary path (api) this stamp describes. */
  outDir?: string
}

/** What this worktree would build right now. mobile/e2e/gate-serve.sh is the owner. */
export function expectedStamp(role: GateRole): GateStamp {
  const raw = execFileSync('bash', [GATE_SERVE_SH, 'stamp', role], {
    cwd: REPO_ROOT,
    encoding: 'utf8',
  })
  return parseStamp(raw, `${GATE_SERVE_SH} stamp ${role}`)
}

export function parseStamp(raw: string, where: string): GateStamp {
  let value: unknown
  try {
    value = JSON.parse(raw) as unknown
  } catch {
    throw new Error(
      `stale phone gate server: ${where} is not JSON. Something this gate did not start is answering there.`,
    )
  }
  const v = value as Partial<GateStamp> | null
  if (
    !v ||
    typeof v !== 'object' ||
    (v.role !== 'ui' && v.role !== 'api') ||
    typeof v.worktree !== 'string' ||
    v.worktree === '' ||
    typeof v.head !== 'string' ||
    v.head === '' ||
    typeof v.digest !== 'string' ||
    v.digest === '' ||
    typeof v.dirty !== 'boolean'
  ) {
    throw new Error(
      `stale phone gate server: ${where} is not a gate stamp {role, worktree, head, dirty, digest}. Something this gate did not start is answering there.`,
    )
  }
  // builtAt decides whether the digest is compared at all, so a wrong *type*
  // here is not cosmetic: it silently makes every bundle look adopted and
  // hands the concurrent-edit flake straight back. It crossed a shell env
  // var once and arrived as "1788779216886" — coerced rather than rejected,
  // because a stamp written by an older gate-serve.sh is still a valid stamp.
  const at = (v as { builtAt?: unknown }).builtAt
  if (at === undefined || at === null) return { ...(v as GateStamp), builtAt: undefined }
  const n = typeof at === 'number' ? at : Number(at)
  return { ...(v as GateStamp), builtAt: Number.isFinite(n) ? n : undefined }
}

/** `<12 chars of sha>` or `<12 chars of sha>+dirty` — the form the gate prints. */
export function stampText(stamp: GateStamp): string {
  return `${stamp.head.slice(0, 12)}${stamp.dirty ? '+dirty' : ''}`
}

/**
 * When this Playwright process started, in epoch ms. A stamp older than this
 * was written by a server we did not start — i.e. reuseExistingServer adopted
 * someone else's.
 */
export const PROCESS_START_MS = Date.now() - Math.round(process.uptime() * 1000)

export type CheckStampOpts = {
  expected: GateStamp
  served: GateStamp
  /** where the served stamp came from, for the message. */
  where: string
  /** how to stop the offending server, for the message. */
  remedy: string
  processStartMs?: number
}

/**
 * Refuse a server that cannot prove it is this worktree's.
 *
 * worktree, head and role are compared always: those are what separate two
 * trees, and none of them moves while a gate runs.
 *
 * `digest` and `dirty` are compared only when the stamp predates this
 * process — the adopted case. When we started the server ourselves the
 * bundle is by definition the one we just built, and the tree is allowed to
 * move under it afterwards: that is the whole point of serving a build
 * instead of a dev server, and re-checking the digest here would hand the
 * flake straight back (an agent saving a screen mid-run would turn a green
 * gate red again, from globalSetup this time).
 */
export function checkStamp(opts: CheckStampOpts): void {
  const { expected, served, where, remedy } = opts
  const startedByUs = typeof served.builtAt === 'number' && served.builtAt >= (opts.processStartMs ?? PROCESS_START_MS)
  const fail = (what: string): never => {
    throw new Error(
      `stale phone gate server: ${what}\n` +
        `  served (${where}): worktree ${served.worktree} head ${served.head} dirty ${served.dirty} digest ${served.digest}\n` +
        `  this worktree:      worktree ${expected.worktree} head ${expected.head} dirty ${expected.dirty} digest ${expected.digest}\n` +
        `  reuseExistingServer adopted it instead of starting one. ${remedy}`,
    )
  }
  if (served.role !== expected.role) fail(`role ${served.role}, wanted ${expected.role}.`)
  if (served.worktree !== expected.worktree) fail('it is serving another worktree.')
  if (served.head !== expected.head) fail('it was built at another commit.')
  if (!startedByUs && served.digest !== expected.digest) {
    fail('it predates this run and its sources differ from this tree.')
  }
  if (!startedByUs && served.dirty !== expected.dirty) {
    fail('it predates this run and was built from a differently-dirty tree.')
  }
}

/** The UI stamp, fetched from the running server — what the browser is handed. */
export async function fetchServedBundleStamp(
  origin: string = UI_ORIGIN,
): Promise<{ stamp: GateStamp; where: string }> {
  const url = `${origin}/${BUNDLE_STAMP_FILE}`
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(
      `stale phone gate server: GET ${url} answered ${res.status}. ` +
        `Whatever is listening on ${origin} was not started by mobile/e2e/gate-serve.sh. ` +
        `Stop it, or run this gate with GADAK_MOBILE_E2E_PORT set to a free port.`,
    )
  }
  return { stamp: parseStamp(await res.text(), url), where: url }
}

export async function assertServedBundle(env: NodeJS.ProcessEnv = process.env): Promise<GateStamp> {
  const origin = mobileUIOrigin(env)
  const { stamp, where } = await fetchServedBundleStamp(origin)
  checkStamp({
    expected: expectedStamp('ui'),
    served: stamp,
    where,
    remedy: `Stop it, or run with GADAK_MOBILE_E2E_PORT set to a free port.`,
  })
  return stamp
}

export function assertServedAPI(env: NodeJS.ProcessEnv = process.env): GateStamp {
  const path = apiStampPath(env)
  const origin = mobileAPIOrigin(env)
  if (!existsSync(path)) {
    throw new Error(
      `stale phone gate server: ${origin} is up but ${path} is missing. ` +
        `That demo server was not started by mobile/e2e/gate-serve.sh. ` +
        `Stop it, or run with GADAK_MOBILE_API_PORT set to a free port.`,
    )
  }
  const stamp = parseStamp(readFileSync(path, 'utf8'), path)
  checkStamp({
    expected: expectedStamp('api'),
    served: stamp,
    where: path,
    remedy: `Stop it (pkill -f '${stamp.outDir ?? apiBinPath(env)}'), or run with GADAK_MOBILE_API_PORT set to a free port.`,
  })
  return stamp
}

// TestConfigWebServer is not exported by name, so it is recovered from the
// array arm of the config field it types.
type WebServer = NonNullable<PlaywrightTestConfig['webServer']>
type WebServerEntry = Extract<WebServer, readonly unknown[]>[number]

/**
 * The gate's two servers. Shared by both Playwright configs so a change to
 * one cannot leave the other on a dev server.
 *
 * `reuseExistingServer` stays true — a developer with the demo already up
 * should not wait for another one — but it is now paired with the stamp
 * check in globalSetup, which runs after the webServer plugin
 * (node_modules/playwright/lib/runner/index.js:6003 createGlobalSetupTasks
 * puts plugin setup before globalSetups). Adoption is therefore always
 * followed by proof.
 */
export function gateWebServers(env: NodeJS.ProcessEnv = process.env): WebServerEntry[] {
  const uiPort = mobileUIPort(env)
  const apiPort = mobileAPIPort(env)
  const outDir = gateOutDir(env)
  return [
    {
      command: `bash ${GATE_SERVE_SH} api`,
      url: `${mobileAPIOrigin(env)}/healthz`,
      reuseExistingServer: true,
      timeout: 180_000,
      cwd: REPO_ROOT,
      stdout: 'pipe' as const,
      env: {
        GADAK_MOBILE_API_PORT: apiPort,
        GADAK_MOBILE_API_STAMP: apiStampPath(env),
        GADAK_MOBILE_API_BIN: apiBinPath(env),
      },
    },
    {
      // Health check on the stamp, not on `/`: if the file the provenance
      // check reads is not being served, nothing downstream is worth running.
      command: `bash ${GATE_SERVE_SH} ui`,
      url: `${mobileUIOrigin(env)}/${BUNDLE_STAMP_FILE}`,
      reuseExistingServer: true,
      timeout: 180_000,
      cwd: MOBILE_DIR,
      // The build and the stamp line land in the run log. There is no
      // `page reload <file>` to catch any more (GDK-1526's telemetry) —
      // a preview server has no watcher — but a build failure or a
      // surprising stamp is exactly the line that explains a whole red gate.
      stdout: 'pipe' as const,
      env: {
        GADAK_MOBILE_E2E_PORT: uiPort,
        GADAK_MOBILE_GATE_OUTDIR: outDir,
        GADAK_MOBILE_GATE_STAMP_FILE: BUNDLE_STAMP_FILE,
      },
    },
  ]
}

/**
 * The app reaches the mirror through the UI origin's own /api proxy, never
 * across origins (mobile/src/lib/api.ts: in DEV the request path is bare).
 * So the proxy target is load-bearing, and when it is wrong the symptom is
 * every API-touching spec timing out with nothing in the failure naming a
 * proxy. Measured, on this round's first run: `vite preview` had been left
 * in the repo root by a stray `cd`, loaded the *web* app's vite.config.ts
 * instead of the phone's, and proxied /api to 7777 — 7 specs died at 90 s
 * each and the only clue was a `[vite] http proxy error` line scrolled past
 * in the server log. One request here turns that into one sentence.
 */
async function assertProxyReachesAPI(env: NodeJS.ProcessEnv = process.env): Promise<void> {
  const url = `${mobileUIOrigin(env)}/api/v1/issues/bootstrap/`
  let status: number
  try {
    status = (await fetch(url)).status
  } catch (err) {
    throw new Error(
      `the phone gate's /api proxy is dead: GET ${url} threw ${String(err)}. ` +
        `It must reach the demo server on 127.0.0.1:${mobileAPIPort(env)} — check the proxy target in mobile/vite.config.ts, and that the preview server loaded *that* config and not the repo root's.`,
    )
  }
  if (status !== 200) {
    throw new Error(
      `the phone gate's /api proxy answered ${status} for ${url}. ` +
        `It must reach the demo server on 127.0.0.1:${mobileAPIPort(env)} — check the proxy target in mobile/vite.config.ts, and that the preview server loaded *that* config and not the repo root's.`,
    )
  }
}

/**
 * Both configs' globalSetup. Proves provenance and reachability, then prints
 * the one line that answers "what was I actually looking at" when a capture
 * surprises someone a day later.
 */
export default async function globalSetup(): Promise<void> {
  const ui = await assertServedBundle()
  assertServedAPI()
  await assertProxyReachesAPI()
  console.log(
    `served ${ui.outDir ?? gateOutDir()} stamp ${stampText(ui)} ui :${mobileUIPort()} api :${mobileAPIPort()}`,
  )
}
