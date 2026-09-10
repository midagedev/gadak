import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { ApiError, type FetchLike } from '../api'
import {
  bindSessionIssue,
  deleteSession,
  inputLineRefusal,
  listSessions,
  placeSessionInput,
  renameSession,
  TERMINAL_INPUT_MAX_BYTES,
} from './sessions'

/*
 * GDK-1497 A6 — the phone's half of the terminal session surface.
 *
 * Four routes existed on the serve since GDK-1158/1195/1162 and the phone
 * reached none of them (mobile/src/screens/Shell.svelte opened one session
 * and showed it). What is pinned here is the wire contract of each verb —
 * method, path, body — because that is the half a phone cannot discover at
 * runtime: a POST to the wrong path is a 404 the roster poll papers over
 * two seconds later, and a body with the wrong field name is a 200 that
 * silently changes nothing.
 *
 * Same fake-fetch shape as ./api.test.ts: an injected FetchLike records the
 * call, so no server, no socket and no shell are involved.
 */

const TOKEN = '<terminal-token>'
const session = { endpoint: 'https://home.example.ts.net', token: TOKEN }

function fakeFetch(
  status: number,
  body: unknown,
): { fn: FetchLike; calls: { url: string; init: RequestInit }[] } {
  const calls: { url: string; init: RequestInit }[] = []
  const fn: FetchLike = async (url, init) => {
    calls.push({ url, init })
    if (status === 204) return new Response(null, { status })
    return new Response(body === null ? null : JSON.stringify(body), { status })
  }
  return { fn, calls }
}

function bearer(init: RequestInit): string | undefined {
  return (init.headers as Record<string, string> | undefined)?.Authorization
}

describe('listSessions', () => {
  it('GETs the roster and carries the row fields the strip reads', async () => {
    // term.Info, not the four-field shape the phone used to declare: the
    // name, the binding and the ordinal are what a row is called.
    const { fn, calls } = fakeFetch(200, {
      sessions: [
        { id: 'a', seq: 1, name: 'build', pid: 9, cols: 80, rows: 24 },
        { id: 'b', seq: 2, issue_key: 'GDK-1497', needs_attention: true },
      ],
    })
    const rows = await listSessions(session, fn)
    expect(rows.map((r) => r.id)).toEqual(['a', 'b'])
    expect(rows[0].name).toBe('build')
    expect(rows[1].issue_key).toBe('GDK-1497')
    expect(rows[1].needs_attention).toBe(true)
    expect(calls[0].init.method ?? 'GET').toBe('GET')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/')
    expect(calls[0].url).not.toContain(TOKEN)
    expect(bearer(calls[0].init)).toBe(`Bearer ${TOKEN}`)
  })

  it('reads an absent sessions field as an empty roster', async () => {
    const { fn } = fakeFetch(200, {})
    expect(await listSessions(session, fn)).toEqual([])
  })
})

describe('renameSession', () => {
  it('POSTs {name} to sessions/{id}/name/ and returns the updated row', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'a', seq: 1, name: 'build' })
    const row = await renameSession('a', 'build', session, fn)
    expect(row.name).toBe('build')
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/a/name/')
    expect(calls[0].init.body).toBe('{"name":"build"}')
    expect(bearer(calls[0].init)).toBe(`Bearer ${TOKEN}`)
  })

  it('clears with an empty name — the server reads empty as "no label"', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'a', seq: 1 })
    await renameSession('a', '', session, fn)
    expect(calls[0].init.body).toBe('{"name":""}')
  })

  it('percent-encodes the id', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'a/b' })
    await renameSession('a/b', 'x', session, fn)
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/a%2Fb/name/')
  })
})

describe('bindSessionIssue', () => {
  it('POSTs {issue_key} to sessions/{id}/issue/ — snake_case, as the server reads it', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'a', issue_key: 'GDK-1497' })
    const row = await bindSessionIssue('a', 'GDK-1497', session, fn)
    expect(row.issue_key).toBe('GDK-1497')
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/a/issue/')
    expect(calls[0].init.body).toBe('{"issue_key":"GDK-1497"}')
  })

  it('trims and upper-cases the key a thumb typed before it reaches the wire', async () => {
    // The server stores the key as given (terminal.go handleTerminalIssue):
    // it never checks it against the mirror, so ' gdk-1497 ' would become a
    // binding no card-to-shell join can ever match.
    const { fn, calls } = fakeFetch(200, { id: 'a', issue_key: 'GDK-1497' })
    await bindSessionIssue('a', '  gdk-1497 ', session, fn)
    expect(calls[0].init.body).toBe('{"issue_key":"GDK-1497"}')
  })

  it('clears the binding with an empty key', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'a' })
    await bindSessionIssue('a', '', session, fn)
    expect(calls[0].init.body).toBe('{"issue_key":""}')
  })
})

describe('placeSessionInput', () => {
  it('POSTs {text} to sessions/{id}/input/ and returns the placed byte count', async () => {
    const { fn, calls } = fakeFetch(200, { placed: 7 })
    expect(await placeSessionInput('a', 'ls -la', session, fn)).toBe(7)
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/a/input/')
    expect(calls[0].init.body).toBe('{"text":"ls -la"}')
  })

  it('refuses a newline before dialling — this route places a line, it does not run one', async () => {
    const { fn, calls } = fakeFetch(200, { placed: 0 })
    await expect(placeSessionInput('a', 'ls\n', session, fn)).rejects.toMatchObject({
      code: 'input_not_a_line',
    })
    // Nothing reached the wire: the refusal is the client's, not a round trip.
    expect(calls).toHaveLength(0)
  })

  it('refuses an empty line and one past the server bound, without dialling', async () => {
    const { fn, calls } = fakeFetch(200, { placed: 0 })
    await expect(placeSessionInput('a', '   ', session, fn)).rejects.toBeInstanceOf(ApiError)
    await expect(
      placeSessionInput('a', 'x'.repeat(TERMINAL_INPUT_MAX_BYTES + 1), session, fn),
    ).rejects.toMatchObject({ code: 'input_too_long' })
    expect(calls).toHaveLength(0)
  })

  it('counts the bound in bytes, not characters — the server reads len(req.Text)', async () => {
    // '가' is 3 UTF-8 bytes. A character count would let a legal-looking
    // line past the client and collect a 400 from the serve instead.
    const line = '가'.repeat(TERMINAL_INPUT_MAX_BYTES)
    expect(inputLineRefusal(line)).toBe('input_too_long')
    expect(inputLineRefusal('가'.repeat(10))).toBeNull()
  })
})

describe('deleteSession', () => {
  it('DELETEs sessions/{id}/ and reads 204 No Content as success', async () => {
    const { fn, calls } = fakeFetch(204, null)
    await expect(deleteSession('a', session, fn)).resolves.toBeUndefined()
    expect(calls[0].init.method).toBe('DELETE')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/a/')
  })
})

/*
 * The pane's half of the same round, as a source contract — the established
 * style in this tree for a .svelte with no DOM mount harness (KeyBar.test.ts,
 * input-machines.test.ts read Shell.svelte the same way).
 *
 * Three things are pinned, and each of them is a class rather than a symptom:
 *
 *   the epoch — every socket callback closes over the id it was opened for,
 *     so a socket the pane has left behind still knows how to reconnect
 *     itself. Before switching existed there was only ever one, and nothing
 *     had to say which was current. With a switch there are two for a moment:
 *     the old close lands after the new attach, `phase` is still 'live', and
 *     the old handler schedules a reconnect to the session the person just
 *     navigated away from — which then wins, silently, a beat later.
 *
 *   one owner of the routes — every verb goes through ./sessions. A fetch or
 *     a 'terminal/sessions' literal in the screen is a second spelling of a
 *     path, which is how a POST ends up 404 with a roster poll papering over
 *     it two seconds later.
 *
 *   no native confirm — window.confirm is unthemed, untranslated by our
 *     catalog, and on iOS reads as the OS asking rather than gadak. The
 *     confirmation for ending a shell lives inside the sheet.
 */
describe('GDK-1497 A6 — Shell wiring (source contract)', () => {
  const shell = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'screens', 'Shell.svelte'),
    'utf8',
  )
  /*
   * GDK-1767 (2026-09-11): the epoch — like the rest of the socket
   * skeleton — moved out of the screen into the shared driver
   * (web/src/lib/terminal/driver.ts), so the guard is pinned where it now
   * lives. The claim is unchanged, and its incident (a stale socket
   * scheduling the wrong session's reconnect) is the driver's own header
   * comment. FAIL-first against the pre-refactor screen:
   * scratch/sc-w13-webphone/mobile-unit-1767.log.
   */
  const driver = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '..', '..', '..', '..',
      'web', 'src', 'lib', 'terminal', 'driver.ts',
    ),
    'utf8',
  )

  it('stamps every attachment with an epoch and guards each callback with it', () => {
    expect(shell).toContain('createTerminalDriver')
    expect(driver).toMatch(/const gen = this\.#gen/)
    expect(driver).toMatch(/const mine = \(\): boolean => gen === this\.#gen/)
    // Detaching must move the counter, or a handler from the socket just
    // closed still reads as current. Bump before close(): close can call
    // back synchronously.
    const detach = driver.slice(driver.indexOf('detach(): void {'), driver.indexOf('dispose(): void {'))
    expect(detach).toContain('#gen += 1')
    // Every handler that touches pane state leads with the guard.
    for (const cb of ['onOpen: () =>', 'onBytes: (data) =>', 'onExit: (code) =>', 'onDropped: (reason) =>', 'onClose: (neverOpened) =>']) {
      const at = driver.indexOf(cb)
      expect(at, `${cb} must exist`).toBeGreaterThan(-1)
      const head = driver.slice(at, at + 200)
      expect(head, `${cb} must lead with the epoch guard`).toMatch(/\{\s*\n\s*if \(!mine\(\)\) return/)
    }
  })

  it('reaches every session route through ./sessions and spells no path itself', () => {
    for (const verb of [
      'listSessions',
      'renameSession',
      'bindSessionIssue',
      'placeSessionInput',
      'deleteSession',
      'nextSelectedAfterKill',
    ]) {
      expect(shell, `${verb} must be imported`).toContain(verb)
    }
    expect(shell).toMatch(/from '\.\.\/lib\/terminal\/sessions'/)
    expect(shell, 'no route literal in the screen').not.toContain('terminal/sessions/')
    expect(shell, 'no bare fetch in the screen').not.toMatch(/\bfetch\s*\(/)
  })

  it('confirms an end inside the sheet, never through the OS', () => {
    // The needle is assembled, the way web-boundary.test.ts assembles its
    // banned prefixes: spelled whole it matches the sentence in Shell.svelte
    // that explains why the call is not there.
    const nativeCall = new RegExp('\\b(?:window\\.)?' + 'conf' + 'irm\\s*\\(')
    expect(shell).not.toMatch(nativeCall)
    expect(shell).toContain("terminal.strip.killConfirm")
    // The confirm step is a mode of the row drawer, so it cannot be reached
    // without the row it belongs to being open.
    expect(shell).toMatch(/rowMode === 'kill'/)
  })

  it('names a session with the desktop words, from the shared derivation', () => {
    // sessionLabel / stripRows live in web/src/lib/terminal/strip.ts. A
    // second answer here is how two surfaces start calling one shell two
    // different things.
    expect(shell).toContain('stripRows(roster, keptSessionId')
    expect(shell).toContain('sessionLabel(current, defaultName)')
    expect(shell).toContain("t('terminal.strip.defaultName', { n })")
  })
})
