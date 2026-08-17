/*
 * Desktop integrations wire contract (GDK-185).
 *
 * The Integrations settings tab reads a list and then watches a plain-text
 * stream whose last line is `exit=<code>`. Both halves are the kind of thing
 * that looks right on screen once and is wrong on the second run: a line
 * boundary that lands mid-chunk, an `installed` the server could not decide,
 * an exit code nobody parsed. So the whole judgement lives in
 * `lib/integrations.ts` as pure functions and is pinned here — the .svelte
 * file only paints. (This repo has no component-mount harness: the `unit`
 * vitest project runs `environment: 'node'` with no svelte plugin, so logic
 * left in a component is logic no test can reach.)
 */
import { describe, expect, test } from 'vitest'
import {
  actionLabelKind,
  endInstallStream,
  feedInstallStream,
  fetchIntegrations,
  installBlocked,
  integrationStatus,
  isVisibleSettingsTab,
  newInstallStream,
  normalizeIntegrations,
  parseExitLine,
  postInstall,
  postRunNote,
  readInstallStream,
  startFailureOutcome,
  splitLines,
  visibleSettingsTabs,
  type IntegrationItem,
} from './integrations'

const WIRE_ITEM = {
  id: 'raycast',
  title: 'Raycast extension',
  installed: true,
  detail: '~/.gadak/raycast-extension',
  command: 'gadak raycast install',
  prerequisite: { ok: true, message: '' },
}

/** A response body that hands `chunks` to the reader one at a time. */
function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  let i = 0
  return new ReadableStream({
    pull(controller) {
      if (i >= chunks.length) {
        controller.close()
        return
      }
      controller.enqueue(encoder.encode(chunks[i++]))
    },
  })
}

describe('normalizeIntegrations', () => {
  test('keeps the server order — the list is the server\'s, not ours', () => {
    const items = normalizeIntegrations({
      items: [
        { ...WIRE_ITEM, id: 'raycast' },
        { ...WIRE_ITEM, id: 'skill' },
        { ...WIRE_ITEM, id: 'mcp-claude' },
      ],
    })
    expect(items.map((i) => i.id)).toEqual(['raycast', 'skill', 'mcp-claude'])
  })

  test('reads one full item off the wire unchanged', () => {
    const [item] = normalizeIntegrations({ items: [WIRE_ITEM] })
    expect(item).toEqual({
      id: 'raycast',
      title: 'Raycast extension',
      installed: true,
      detail: '~/.gadak/raycast-extension',
      command: 'gadak raycast install',
      prerequisite: { ok: true, message: '' },
    } satisfies IntegrationItem)
  })

  test('installed: null survives as null — "unknown" is not "not installed"', () => {
    const items = normalizeIntegrations({
      items: [
        { ...WIRE_ITEM, id: 'a', installed: true },
        { ...WIRE_ITEM, id: 'b', installed: false },
        { ...WIRE_ITEM, id: 'c', installed: null },
        { ...WIRE_ITEM, id: 'd', installed: undefined },
        { ...WIRE_ITEM, id: 'e', installed: 'yes' },
      ],
    })
    expect(items.map((i) => i.installed)).toEqual([true, false, null, null, null])
  })

  test('an item with no id is dropped rather than rendered as a nameless card', () => {
    const items = normalizeIntegrations({
      items: [{ ...WIRE_ITEM, id: '' }, { ...WIRE_ITEM, id: 42 }, WIRE_ITEM, 'nope', null],
    })
    expect(items.map((i) => i.id)).toEqual(['raycast'])
  })

  test('missing strings default; title falls back to the id', () => {
    const [item] = normalizeIntegrations({ items: [{ id: 'skill' }] })
    expect(item.title).toBe('skill')
    expect(item.detail).toBe('')
    expect(item.command).toBe('')
    expect(item.prerequisite).toBeNull()
  })

  test('a failing prerequisite keeps its message; a malformed one gates nothing', () => {
    const items = normalizeIntegrations({
      items: [
        { ...WIRE_ITEM, id: 'a', prerequisite: { ok: false, message: 'Install the CLI first.' } },
        { ...WIRE_ITEM, id: 'b', prerequisite: { message: 'no ok field' } },
        { ...WIRE_ITEM, id: 'c', prerequisite: 'broken' },
      ],
    })
    expect(items[0].prerequisite).toEqual({ ok: false, message: 'Install the CLI first.' })
    expect(items[1].prerequisite).toBeNull()
    expect(items[2].prerequisite).toBeNull()
  })

  test('a body that is not a list is an empty list, not a crash', () => {
    expect(normalizeIntegrations(null)).toEqual([])
    expect(normalizeIntegrations({})).toEqual([])
    expect(normalizeIntegrations({ items: 'raycast' })).toEqual([])
    expect(normalizeIntegrations([WIRE_ITEM])).toEqual([])
  })
})

describe('splitLines', () => {
  test('complete lines come out, the partial one stays in the buffer', () => {
    expect(splitLines('', 'one\ntwo\nthr')).toEqual({ lines: ['one', 'two'], buffer: 'thr' })
  })

  test('a line split across two chunks is reassembled, not halved', () => {
    const first = splitLines('', 'inst')
    expect(first).toEqual({ lines: [], buffer: 'inst' })
    const second = splitLines(first.buffer, 'alling\n')
    expect(second).toEqual({ lines: ['installing'], buffer: '' })
  })

  test('CRLF loses the CR', () => {
    expect(splitLines('', 'one\r\ntwo\r\n').lines).toEqual(['one', 'two'])
  })

  test('blank lines are kept — they are output the command really printed', () => {
    expect(splitLines('', 'a\n\nb\n').lines).toEqual(['a', '', 'b'])
  })

  test('an empty chunk changes nothing', () => {
    expect(splitLines('half', '')).toEqual({ lines: [], buffer: 'half' })
  })
})

describe('parseExitLine', () => {
  test('reads the sentinel, including surrounding whitespace', () => {
    expect(parseExitLine('exit=0')).toBe(0)
    expect(parseExitLine('exit=1')).toBe(1)
    expect(parseExitLine('exit=127')).toBe(127)
    expect(parseExitLine('  exit=2\t')).toBe(2)
    expect(parseExitLine('exit=-1')).toBe(-1)
  })

  test('anything that merely mentions exit= is ordinary output', () => {
    expect(parseExitLine('exit=')).toBeNull()
    expect(parseExitLine('exit=ok')).toBeNull()
    expect(parseExitLine('the script will exit=0 on success')).toBeNull()
    expect(parseExitLine('exit=0 done')).toBeNull()
    expect(parseExitLine('')).toBeNull()
  })
})

describe('install stream state', () => {
  test('a clean run: output lines, exit 0, terminated', () => {
    let s = newInstallStream()
    s = feedInstallStream(s, 'installing…\ndone\n')
    expect(s.lines).toEqual(['installing…', 'done'])
    expect(s.exitCode).toBeNull()
    expect(s.terminated).toBe(false)
    s = endInstallStream(feedInstallStream(s, 'exit=0\n'))
    expect(s.exitCode).toBe(0)
    expect(s.terminated).toBe(true)
    // The sentinel is protocol, not program output — it must not land in the log.
    expect(s.lines).toEqual(['installing…', 'done'])
  })

  test('a line broken across chunks is neither lost nor duplicated', () => {
    let s = newInstallStream()
    for (const chunk of ['fetch', 'ing rayc', 'ast\nlink', 'ed\nexi', 't=0\n']) {
      s = feedInstallStream(s, chunk)
    }
    s = endInstallStream(s)
    expect(s.lines).toEqual(['fetching raycast', 'linked'])
    expect(s.exitCode).toBe(0)
    expect(s.terminated).toBe(true)
  })

  test('a non-zero exit is carried as the number, not a boolean', () => {
    const s = endInstallStream(feedInstallStream(newInstallStream(), 'permission denied\nexit=13\n'))
    expect(s.exitCode).toBe(13)
    expect(s.terminated).toBe(true)
    // Failure keeps the log: it is the only account of what went wrong.
    expect(s.lines).toEqual(['permission denied'])
  })

  test('end of stream flushes a trailing line that never got its newline', () => {
    let s = newInstallStream()
    s = feedInstallStream(s, 'last word')
    expect(s.lines).toEqual([])
    s = endInstallStream(s)
    expect(s.lines).toEqual(['last word'])
    expect(s.closed).toBe(true)
  })

  test('an unterminated exit= line is still the exit code once the stream closes', () => {
    const s = endInstallStream(feedInstallStream(newInstallStream(), 'ok\nexit=3'))
    expect(s.exitCode).toBe(3)
    expect(s.lines).toEqual(['ok'])
    expect(s.terminated).toBe(true)
  })
})

/*
 * The verdict is the LAST non-empty line of the stream and nothing else.
 *
 * Install commands print whatever they like, and a line that happens to read
 * `exit=0` in the middle of one must not be mistaken for the run finishing —
 * that is the single mistake that would turn a broken install into a green
 * check. So a sentinel-shaped line is parked, not judged, and anything that
 * arrives after it proves it was ordinary output. (Lead spec addendum,
 * 2026-08-17: "exit= 판정은 스트림의 마지막 라인만".)
 */
describe('the verdict is the last line only', () => {
  test('a mid-stream exit=0 followed by more output is output, not a verdict', () => {
    let s = feedInstallStream(newInstallStream(), 'exit=0\nstill working\n')
    // Not decided yet, and the echoed line is preserved verbatim in the log.
    expect(s.exitCode).toBeNull()
    expect(s.lines).toEqual(['exit=0', 'still working'])
    // The stream then dies without ever reporting a status.
    s = endInstallStream(s, { broken: true })
    expect(s.exitCode).toBeNull()
    expect(s.terminated).toBe(false)
    expect(s.lines).toEqual(['exit=0', 'still working'])
  })

  test('an echoed exit=0 does not survive as a verdict when the real one is non-zero', () => {
    const s = endInstallStream(
      feedInstallStream(newInstallStream(), 'the wrapper prints:\nexit=0\noops\nexit=7\n'),
    )
    expect(s.exitCode).toBe(7)
    expect(s.lines).toEqual(['the wrapper prints:', 'exit=0', 'oops'])
  })

  test('trailing blank lines do not dethrone the sentinel', () => {
    const s = endInstallStream(feedInstallStream(newInstallStream(), 'done\nexit=0\n\n\n'))
    expect(s.exitCode).toBe(0)
    expect(s.terminated).toBe(true)
    expect(s.lines).toEqual(['done'])
  })

  test('a sentinel split across chunks is still the last line', () => {
    let s = newInstallStream()
    for (const chunk of ['done\n', 'ex', 'it=', '5\n']) s = feedInstallStream(s, chunk)
    s = endInstallStream(s)
    expect(s.exitCode).toBe(5)
    expect(s.lines).toEqual(['done'])
  })
})

/*
 * A stream that stops without `exit=` — network error, server restart, the app
 * quitting mid-install — is neither success nor failure. Guessing either way is
 * the failure mode that matters: exit=0 was never seen, so nothing may turn
 * into a check mark.
 */
describe('a stream with no verdict', () => {
  test('a clean end without a sentinel reports no exit code', () => {
    const s = endInstallStream(feedInstallStream(newInstallStream(), 'half done\n'))
    expect(s.exitCode).toBeNull()
    expect(s.terminated).toBe(false)
    expect(s.closed).toBe(true)
    expect(s.broken).toBe(false)
    // The output so far is kept — it is what the user has to go on.
    expect(s.lines).toEqual(['half done'])
  })

  test('a socket that dies mid-install is marked broken, not finished', () => {
    const s = endInstallStream(feedInstallStream(newInstallStream(), 'copying\n'), { broken: true })
    expect(s.broken).toBe(true)
    expect(s.terminated).toBe(false)
    expect(s.exitCode).toBeNull()
  })

  test('no verdict is its own pill state, never "not installed"', () => {
    expect(
      integrationStatus({
        loading: false,
        running: false,
        installed: false,
        failedExit: null,
        resultUnknown: true,
      }),
    ).toBe('result-unknown')
    // Nor does it get quietly upgraded when the stored value happens to be true.
    expect(
      integrationStatus({
        loading: false,
        running: false,
        installed: true,
        failedExit: null,
        resultUnknown: true,
      }),
    ).toBe('result-unknown')
  })
})

describe('readInstallStream', () => {
  test('drives the reader to the end and reports every line once', async () => {
    const seen: string[][] = []
    const final = await readInstallStream(streamOf(['a\nb', 'b\n', 'exit=0\n']), (s) =>
      seen.push([...s.lines]),
    )
    expect(final.lines).toEqual(['a', 'bb'])
    expect(final.exitCode).toBe(0)
    expect(final.terminated).toBe(true)
    expect(final.closed).toBe(true)
    // The caller was told about progress before the end, or the log panel
    // would fill in one jump at completion.
    expect(seen.length).toBeGreaterThan(1)
    expect(seen[0]).toEqual(['a'])
  })

  test('a stream that errors mid-install resolves as broken — it never throws', async () => {
    // Delivered on the first pull, killed on the second: `error()` from within
    // start() would discard the queued chunk, which is stream semantics rather
    // than the case worth pinning (output seen before the break is kept).
    let pulls = 0
    const stream = new ReadableStream<Uint8Array>({
      pull(controller) {
        pulls += 1
        if (pulls === 1) {
          controller.enqueue(new TextEncoder().encode('copying files\n'))
          return
        }
        controller.error(new Error('server went away'))
      },
    })
    // No rejection: an unhandled one here would leave the row spinning on
    // "Installing…" with no way back.
    const final = await readInstallStream(stream, () => {})
    expect(final.broken).toBe(true)
    expect(final.closed).toBe(true)
    expect(final.terminated).toBe(false)
    expect(final.exitCode).toBeNull()
    expect(final.lines).toEqual(['copying files'])
  })

  test('multi-byte output split mid-character is decoded, not mojibake', async () => {
    const encoded = new TextEncoder().encode('설치 완료\n')
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoded.slice(0, 2))
        controller.enqueue(encoded.slice(2))
        controller.close()
      },
    })
    const final = await readInstallStream(stream, () => {})
    expect(final.lines).toEqual(['설치 완료'])
  })
})

describe('integrationStatus', () => {
  const base = { loading: false, running: false, installed: null, failedExit: null } as const

  test('a run in progress outranks the stale verdict under it', () => {
    expect(integrationStatus({ ...base, running: true, installed: false, failedExit: 1 })).toBe(
      'running',
    )
  })

  test('a non-zero exit is the headline until the next run', () => {
    expect(integrationStatus({ ...base, installed: true, failedExit: 1 })).toBe('failed')
  })

  test('exit 0 is not a failure', () => {
    expect(integrationStatus({ ...base, installed: true, failedExit: 0 })).toBe('installed')
  })

  test('the first fetch says checking, not "not installed"', () => {
    expect(integrationStatus({ ...base, loading: true })).toBe('checking')
  })

  test('installed / not installed / unknown are three states, not two', () => {
    expect(integrationStatus({ ...base, installed: true })).toBe('installed')
    expect(integrationStatus({ ...base, installed: false })).toBe('not-installed')
    expect(integrationStatus({ ...base, installed: null })).toBe('unknown')
  })
})

/*
 * What the card says once an attempt is over. The dangerous branch is the third
 * one: the command reported success and the re-check still cannot see it.
 */
describe('postRunNote', () => {
  test('no status reported → say the result is unknown', () => {
    expect(postRunNote({ terminated: false, exitCode: null, installedAfter: true })).toBe(
      'no-verdict',
    )
  })

  test('exit 0 but still undetected → report both, do not overrule the check', () => {
    expect(postRunNote({ terminated: true, exitCode: 0, installedAfter: false })).toBe(
      'ok-undetected',
    )
    // "Could not tell" is not detection either.
    expect(postRunNote({ terminated: true, exitCode: 0, installedAfter: null })).toBe(
      'ok-undetected',
    )
  })

  test('exit 0 and detected → nothing to explain', () => {
    expect(postRunNote({ terminated: true, exitCode: 0, installedAfter: true })).toBe('none')
  })

  test('a non-zero exit needs no note — the failed pill and the code say it', () => {
    expect(postRunNote({ terminated: true, exitCode: 13, installedAfter: false })).toBe('none')
  })
})

describe('startFailureOutcome', () => {
  test('409 is a run in flight, not an error', () => {
    expect(startFailureOutcome('already-running')).toEqual({
      foreignRunning: true,
      noteKind: 'busy',
    })
  })

  test('the other two are refusals, and the row is not left looking busy', () => {
    expect(startFailureOutcome('unknown-id')).toEqual({
      foreignRunning: false,
      noteKind: 'unknown-id',
    })
    expect(startFailureOutcome('unavailable')).toEqual({
      foreignRunning: false,
      noteKind: 'start-failed',
    })
  })

  test('a run we cannot watch still reads as running on the pill', () => {
    const { foreignRunning } = startFailureOutcome('already-running')
    expect(
      integrationStatus({
        loading: false,
        running: foreignRunning,
        installed: false,
        failedExit: null,
      }),
    ).toBe('running')
  })
})

describe('actionLabelKind', () => {
  test('re-running an installed integration is an update', () => {
    expect(actionLabelKind(true, null)).toBe('update')
  })

  test('a failed attempt turns the button into a retry', () => {
    expect(actionLabelKind(true, 1)).toBe('retry')
    expect(actionLabelKind(false, 7)).toBe('retry')
  })

  test('exit 0 leaves it an ordinary install/update', () => {
    expect(actionLabelKind(false, 0)).toBe('install')
    expect(actionLabelKind(true, 0)).toBe('update')
  })

  test('unknown installed state offers install', () => {
    expect(actionLabelKind(null, null)).toBe('install')
  })
})

describe('installBlocked', () => {
  const item = (prerequisite: IntegrationItem['prerequisite'], command = 'gadak x'): IntegrationItem => ({
    id: 'x',
    title: 'X',
    installed: false,
    detail: '',
    command,
    prerequisite,
  })

  test('a failing prerequisite blocks the button', () => {
    expect(installBlocked(item({ ok: false, message: 'need cli' }))).toBe(true)
  })

  test('a satisfied or absent prerequisite does not', () => {
    expect(installBlocked(item({ ok: true, message: '' }))).toBe(false)
    expect(installBlocked(item(null))).toBe(false)
  })

  test('no command means there is nothing to run', () => {
    expect(installBlocked(item(null, ''))).toBe(true)
  })
})

describe('desktop-only settings tabs', () => {
  const ALL = ['sync', 'sources', 'features', 'groups', 'members', 'fields', 'integrations'] as const

  test('the integrations tab is not offered outside the desktop app', () => {
    expect(visibleSettingsTabs(ALL, false)).toEqual([
      'sync',
      'sources',
      'features',
      'groups',
      'members',
      'fields',
    ])
  })

  test('the desktop app gets all of them', () => {
    expect(visibleSettingsTabs(ALL, true)).toEqual([...ALL])
  })

  test('a settings=integrations link opened in a browser is not a valid tab', () => {
    expect(isVisibleSettingsTab('integrations', ALL, false)).toBe(false)
    expect(isVisibleSettingsTab('integrations', ALL, true)).toBe(true)
  })

  test('ordinary tabs and unknown names are unaffected by the surface', () => {
    expect(isVisibleSettingsTab('sync', ALL, false)).toBe(true)
    expect(isVisibleSettingsTab('sync', ALL, true)).toBe(true)
    expect(isVisibleSettingsTab('nope', ALL, true)).toBe(false)
  })
})

describe('fetchIntegrations', () => {
  test('asks the desktop mux and nothing else', async () => {
    const calls: string[] = []
    const items = await fetchIntegrations(async (input) => {
      calls.push(String(input))
      return new Response(JSON.stringify({ items: [WIRE_ITEM] }), { status: 200 })
    })
    expect(calls).toEqual(['/desktop/integrations'])
    expect(items.map((i) => i.id)).toEqual(['raycast'])
  })

  test('a non-OK response is an error, not an empty list', async () => {
    await expect(
      fetchIntegrations(async () => new Response('nope', { status: 404 })),
    ).rejects.toThrow()
  })
})

describe('postInstall', () => {
  test('200 hands back the stream to read', async () => {
    const result = await postInstall(
      'raycast',
      async () => new Response(streamOf(['exit=0\n']), { status: 200 }),
    )
    expect('stream' in result).toBe(true)
  })

  test('posts to the id-scoped install path', async () => {
    const calls: [string, string | undefined][] = []
    await postInstall('mcp-claude', async (input, init) => {
      calls.push([String(input), init?.method])
      return new Response(streamOf(['exit=0\n']), { status: 200 })
    })
    expect(calls).toEqual([['/desktop/integrations/mcp-claude/install', 'POST']])
  })

  test('an id the server does not know is reported as such', async () => {
    const result = await postInstall('nope', async () => new Response('', { status: 404 }))
    expect(result).toEqual({ failure: 'unknown-id', status: 404 })
  })

  test('409 means a run is already going — not a failed install', async () => {
    const result = await postInstall('skill', async () => new Response('', { status: 409 }))
    expect(result).toEqual({ failure: 'already-running', status: 409 })
  })

  test('any other refusal is a plain unavailable', async () => {
    expect(await postInstall('skill', async () => new Response('', { status: 500 }))).toEqual({
      failure: 'unavailable',
      status: 500,
    })
    // A 200 with no body is not a stream we can read.
    expect(await postInstall('skill', async () => new Response(null, { status: 204 }))).toEqual({
      failure: 'unavailable',
      status: 204,
    })
  })

  test('a dead socket is a failure, not an exception through the caller', async () => {
    const result = await postInstall('skill', async () => {
      throw new Error('socket hang up')
    })
    expect(result).toEqual({ failure: 'unavailable', status: 0 })
  })
})
