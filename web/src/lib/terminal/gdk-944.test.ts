/*
 * GDK-944: the terminal pane must name why it cannot attach, retry a
 * first-connect socket that never opened, and only offer Enter-to-restart
 * where a restart can succeed.
 *
 * Mapping and retry live as plain functions in ./session so this suite can
 * exercise them without mounting TerminalPane.svelte (vitest unit is node,
 * no svelte plugin — same constraint as skeleton-grace.test.ts).
 *
 * GDK-989: the call-site scans that used to live here (indexOf + slice
 * order over the pane source) are gone — they broke on harmless refactors
 * and stayed green when the called result was ignored. What they pointed
 * at is the plain-function tests below; the pane wiring itself has no
 * unit-level assertion left (a mount harness is a lead-level call). The
 * skeleton scan stays: it pins shared-module usage, not a call order.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  TerminalHttpError,
  TERMINAL_RECONNECT_BACKOFF_MS,
  classifyCreateFail,
  createSession,
  droppedAllowsRestart,
  firstAttachRetryDelayMs,
  unavailableAllowsRestart,
} from './session'
import { shell } from '../i18n/messages/shell'

const HERE = dirname(fileURLToPath(import.meta.url))
const PANE = join(HERE, '../../components/terminal/TerminalPane.svelte')

function paneSrc(): string {
  return readFileSync(PANE, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GDK-944 classifyCreateFail', () => {
  test('terminal_unsupported (501) is unsupported, not a generic unavailable', () => {
    expect(classifyCreateFail(new TerminalHttpError(501, 'terminal_unsupported'))).toEqual({
      cause: 'unsupported',
      detail: null,
    })
  })

  test('scope_rejected, forbidden_host, pairing_rejected are the same forbidden cause', () => {
    for (const code of ['scope_rejected', 'forbidden_host', 'pairing_rejected'] as const) {
      const status = code === 'pairing_rejected' ? 401 : 403
      expect(classifyCreateFail(new TerminalHttpError(status, code)), code).toEqual({
        cause: 'forbidden',
        detail: null,
      })
    }
  })

  test('terminal_failed (500) is failed and carries the server message', () => {
    const err = new TerminalHttpError(500, 'terminal_failed', 'pty spawn: boom')
    expect(classifyCreateFail(err)).toEqual({
      cause: 'failed',
      detail: 'pty spawn: boom',
    })
  })

  test('fetch failure (status 0 / unreachable) is network', () => {
    expect(classifyCreateFail(new TerminalHttpError(0, 'unreachable'))).toEqual({
      cause: 'network',
      detail: null,
    })
  })
})

describe('GDK-944 first-attach retry', () => {
  test('the first never-opened afterCreate attach retries once using the reconnect backoff', () => {
    expect(firstAttachRetryDelayMs(0)).toBe(TERMINAL_RECONNECT_BACKOFF_MS[0])
    expect(firstAttachRetryDelayMs(1)).toBeNull()
    expect(firstAttachRetryDelayMs(2)).toBeNull()
  })
})

describe('GDK-944 restart is offered only where it can succeed', () => {
  test('unavailable is restartable except for unsupported', () => {
    expect(unavailableAllowsRestart('unsupported')).toBe(false)
    expect(unavailableAllowsRestart('forbidden')).toBe(true)
    expect(unavailableAllowsRestart('failed')).toBe(true)
    expect(unavailableAllowsRestart('network')).toBe(true)
  })

  test('dropped is restartable except for token_revoked', () => {
    expect(droppedAllowsRestart('token_revoked')).toBe(false)
    expect(droppedAllowsRestart('slow_client')).toBe(true)
    expect(droppedAllowsRestart('idle_timeout')).toBe(true)
    expect(droppedAllowsRestart('server_shutdown')).toBe(true)
    expect(droppedAllowsRestart('closed')).toBe(true)
  })
})

describe('GDK-944 createSession keeps the failMsg message', () => {
  test('500 terminal_failed surfaces body.message', async () => {
    vi.stubGlobal(
      'fetch',
      async () =>
        new Response(JSON.stringify({ error: 'terminal_failed', message: 'pty spawn: boom' }), {
          status: 500,
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    try {
      await createSession(80, 24)
      throw new Error('createSession must reject')
    } catch (err) {
      expect(err).toBeInstanceOf(TerminalHttpError)
      const http = err as TerminalHttpError
      expect(http.status).toBe(500)
      expect(http.code).toBe('terminal_failed')
      expect(http.serverMessage).toBe('pty spawn: boom')
    }
  })
})

/*
 * The copy this round added tells a locked-out user to run a command. The
 * first draft of it was `gadak pairing mint --scope terminal`, which exits
 * with "pairing mint requires --label NAME" — advice that fails the moment
 * it is followed, in three languages, with every other gate green.
 *
 * So the strings are checked against the flag `pairingMint` actually
 * requires, not just spell-checked. The command is not run: parsing it for
 * real would write a token.
 */
describe('GDK-944 the mint advice is a command that runs', () => {
  const MINT_KEYS = ['terminal.unavailable.forbidden', 'terminal.mintHint'] as const

  test('every locale spells a complete `pairing mint` invocation', () => {
    for (const key of MINT_KEYS) {
      const entry = shell[key] as Record<string, string>
      for (const [locale, text] of Object.entries(entry)) {
        const where = `${key}.${locale}`
        expect(text, where).toContain('gadak pairing mint')
        expect(text, `${where}: --label is required (cmd/gadak/pairing.go)`).toContain('--label')
        expect(text, `${where}: terminal is not the default scope`).toContain('--scope terminal')
      }
    }
  })
})

describe('GDK-944 first connect shows progress', () => {
  test('the pane uses the shared skeleton grace and LoadingState', () => {
    const src = paneSrc()
    expect(src).toMatch(/from ['"][^'"]*skeleton-grace\.svelte['"]/)
    expect(src).toContain('createSkeletonGrace')
    expect(src).toContain('.visible')
    expect(src).toContain('<LoadingState')
    expect(src).toContain('data-skeleton=')
  })
})
