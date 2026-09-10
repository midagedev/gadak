import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * GDK-1581 gate: the JS side of a Tauri plugin (@tauri-apps/plugin-x) and
 * its Rust crate (tauri-plugin-x) are versioned in lockstep upstream, and
 * the wire between them is only stable inside that lockstep. A `cargo
 * update` that leaves package-lock.json behind — or an npm bump that
 * leaves Cargo.lock behind — compiles green and then mismatches at runtime,
 * on the phone, where nothing reports it. This file turns that drift into
 * a red unit test before any build.
 *
 * Parity pairs are exactly the plugins with a JS binding in this app:
 * http, websocket, barcode-scanner, deep-link. Excluded on purpose:
 *   - tauri-plugin-secure-storage: Rust-only by design — lib.rs's
 *     token_get/token_set/token_del commands are the only door to the
 *     token, and no @tauri-apps/plugin-secure-storage is installed.
 *   - tauri-plugin-fs: Rust/build-side, no JS binding in dependencies.
 *   - @tauri-apps/api: the JS runtime, no version-paired crate.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const PACKAGE_LOCK = join(HERE, '../../package-lock.json')
const CARGO_LOCK = join(HERE, '../../src-tauri/Cargo.lock')

const PARITY_PAIRS: Array<{ npm: string; crate: string }> = [
  { npm: '@tauri-apps/plugin-http', crate: 'tauri-plugin-http' },
  { npm: '@tauri-apps/plugin-websocket', crate: 'tauri-plugin-websocket' },
  { npm: '@tauri-apps/plugin-barcode-scanner', crate: 'tauri-plugin-barcode-scanner' },
  { npm: '@tauri-apps/plugin-deep-link', crate: 'tauri-plugin-deep-link' },
]

function npmVersion(name: string): string {
  const lock = JSON.parse(readFileSync(PACKAGE_LOCK, 'utf8')) as {
    packages?: Record<string, { version?: string }>
  }
  const entry = lock.packages?.[`node_modules/${name}`]
  expect(entry, `${name} missing from mobile/package-lock.json`).toBeDefined()
  expect(entry?.version, `${name} has no resolved version`).toBeDefined()
  return entry!.version!
}

/** Minimal Cargo.lock reader: name → version from each [[package]]. */
function cargoVersions(): Map<string, string> {
  const text = readFileSync(CARGO_LOCK, 'utf8')
  const out = new Map<string, string>()
  for (const chunk of text.split('[[package]]')) {
    const name = /^name = "([^"]+)"/m.exec(chunk)?.[1]
    const version = /^version = "([^"]+)"/m.exec(chunk)?.[1]
    if (name && version) out.set(name, version)
  }
  return out
}

describe('plugin parity: npm and Cargo.lock carry the same plugin versions (GDK-1581)', () => {
  it.each(PARITY_PAIRS)('$crate matches @tauri-apps lockstep', ({ npm, crate }) => {
    const js = npmVersion(npm)
    const rs = cargoVersions().get(crate)
    expect(rs, `${crate} missing from mobile/src-tauri/Cargo.lock`).toBeDefined()
    expect(js, `${npm} ${js} and ${crate} ${rs} drifted — bump both or neither`).toBe(rs)
  })

  it('every JS plugin binding has a parity pair declared here', () => {
    // The guard against the guard rotting: if a new @tauri-apps/plugin-*
    // lands in package-lock.json without a row above, this goes red — the
    // alternative is a plugin silently outside the lockstep check.
    const lock = JSON.parse(readFileSync(PACKAGE_LOCK, 'utf8')) as {
      packages?: Record<string, unknown>
    }
    const installed = Object.keys(lock.packages ?? {})
      .filter((k) => k.startsWith('node_modules/@tauri-apps/plugin-'))
      .map((k) => k.replace('node_modules/', ''))
      .sort()
    expect(installed).toEqual(PARITY_PAIRS.map((p) => p.npm).sort())
  })
})
