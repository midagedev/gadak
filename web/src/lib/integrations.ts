/*
 * Desktop integrations — everything the Integrations settings tab knows
 * (GDK-185).
 *
 * The tab lists the agent surfaces gadak can install into (Raycast extension,
 * Claude Code skill, MCP server), says whether each one is already there, and
 * runs the install without hiding the command. The install answers as a
 * plain-text stream whose last line is `exit=<code>`, so the exit status is
 * data on the wire rather than something the UI infers from silence.
 *
 * All of that judgement lives here, not in the component: the `unit` vitest
 * project runs `environment: 'node'` with no svelte plugin, so logic left in a
 * `.svelte` file is logic no test can reach. `IntegrationsTab.svelte` only
 * paints what these functions return.
 *
 * Transport is same-origin `/desktop/*` and nothing else. Those routes exist
 * on the desktop app's own mux; a browser tab has no such server, which is why
 * the tab itself is desktop-only (see visibleSettingsTabs).
 */

/** GET the list. */
export const INTEGRATIONS_PATH = '/desktop/integrations'

/** POST an install for one id. */
export function installPath(id: string): string {
  return `${INTEGRATIONS_PATH}/${encodeURIComponent(id)}/install`
}

/** A condition that must hold before the install can run (CLI on PATH, …). */
export interface IntegrationPrerequisite {
  ok: boolean
  /** What the user has to do first. Shown verbatim; may be empty when ok. */
  message: string
}

export interface IntegrationItem {
  id: string
  title: string
  /**
   * null is a third answer, not a missing second one: the server looked and
   * could not tell. "Not installed" invites a click that may do the wrong
   * thing, so the two are kept apart all the way to the pill.
   */
  installed: boolean | null
  /** Where it lives / what was detected. A path, usually. */
  detail: string
  /** The command the install button runs, shown on screen. */
  command: string
  prerequisite: IntegrationPrerequisite | null
}

function asString(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

function normalizePrerequisite(v: unknown): IntegrationPrerequisite | null {
  if (typeof v !== 'object' || v === null) return null
  const raw = v as Record<string, unknown>
  // Without a boolean verdict there is nothing to gate on, and a card that
  // blocks its own button over a malformed field is worse than one that lets
  // the command report the problem itself.
  if (typeof raw.ok !== 'boolean') return null
  return { ok: raw.ok, message: asString(raw.message) }
}

/**
 * `GET /desktop/integrations` body → items, in the order the server sent them.
 *
 * The order is the server's (command-line-tool, then raycast when the host
 * offers it, skill, mcp-claude): it is the reading order of the setup, so
 * the UI must not re-sort it. Only macOS offers raycast. Anything unusable is
 * dropped rather than drawn as a nameless card.
 */
export function normalizeIntegrations(body: unknown): IntegrationItem[] {
  if (typeof body !== 'object' || body === null) return []
  const raw = (body as { items?: unknown }).items
  if (!Array.isArray(raw)) return []
  const items: IntegrationItem[] = []
  for (const entry of raw) {
    if (typeof entry !== 'object' || entry === null) continue
    const e = entry as Record<string, unknown>
    const id = asString(e.id)
    if (!id) continue
    items.push({
      id,
      title: asString(e.title) || id,
      installed: typeof e.installed === 'boolean' ? e.installed : null,
      detail: asString(e.detail),
      command: asString(e.command),
      prerequisite: normalizePrerequisite(e.prerequisite),
    })
  }
  return items
}

/* ── The install stream ── */

export interface LineSplit {
  lines: string[]
  /** The tail with no newline yet — the next chunk finishes it. */
  buffer: string
}

/**
 * Split a chunk into complete lines, keeping the unfinished tail.
 *
 * Chunk boundaries have nothing to do with line boundaries, so the leftover
 * has to survive between reads; the alternative (decode-and-render each chunk)
 * shows half-words and then repeats them.
 */
export function splitLines(buffer: string, chunk: string): LineSplit {
  const combined = buffer + chunk
  const parts = combined.split('\n')
  const rest = parts.pop() ?? ''
  return { lines: parts.map((l) => (l.endsWith('\r') ? l.slice(0, -1) : l)), buffer: rest }
}

/**
 * The exit sentinel, or null when the line is ordinary output.
 *
 * Anchored on purpose: a build log that mentions `exit=0` in a sentence must
 * not be read as the command finishing.
 */
export function parseExitLine(line: string): number | null {
  const m = /^exit=(-?\d+)$/.exec(line.trim())
  return m ? Number(m[1]) : null
}

/**
 * Drop ANSI CSI sequences from one line of command output.
 *
 * `npx ray develop` colors its output even into a pipe, so without this the
 * log panel shows `[36minfo[39m` literally (seen live, v0.15.1). The pre is
 * plain text — there is nothing downstream to interpret the codes.
 */
export function stripAnsi(line: string): string {
  // eslint-disable-next-line no-control-regex
  return line.replace(/\u001b\[[0-9;?]*[ -/]*[@-~]/g, '')
}

export interface InstallStreamState {
  /** Program output, sentinel excluded, in order. */
  lines: string[]
  /**
   * Lines held back from the log: `held[0]` parses as a sentinel and may turn
   * out to be the last line of the stream, `held[1…]` are blank lines that
   * followed it. If real output arrives after them they were ordinary output
   * all along and get committed to `lines`.
   */
  held: string[]
  /**
   * The exit code, decided only once the stream closes — the verdict is the
   * last non-empty line and nothing else. Null while running, and after a
   * stream that never reported one.
   */
  exitCode: number | null
  /** The stream ended on an `exit=` line: the command reported a status. */
  terminated: boolean
  /** The stream ended (cleanly or not). */
  closed: boolean
  /** The stream died mid-flight — socket error, server restart, killed app. */
  broken: boolean
  /** Partial line carried between chunks. */
  buffer: string
}

export function newInstallStream(): InstallStreamState {
  return {
    lines: [],
    held: [],
    exitCode: null,
    terminated: false,
    closed: false,
    broken: false,
    buffer: '',
  }
}

/**
 * Fold one decoded chunk in, returning the next state.
 *
 * A line that looks like the sentinel is parked rather than judged: install
 * scripts print all sorts of things, and one that happens to echo `exit=0`
 * halfway through must not be read as the command finishing. Only the last
 * non-empty line of the whole stream is a verdict, which is a fact this
 * function cannot know yet — so it parks the candidate and endInstallStream
 * decides. Anything arriving after a parked candidate proves it was output.
 */
export function feedInstallStream(state: InstallStreamState, chunk: string): InstallStreamState {
  const { lines, buffer } = splitLines(state.buffer, chunk)
  const out = [...state.lines]
  let held = [...state.held]
  for (const raw of lines) {
    const line = stripAnsi(raw)
    if (held.length > 0) {
      // Trailing blank lines do not dethrone a candidate: "last non-empty".
      if (line.trim() === '') {
        held.push(line)
        continue
      }
      out.push(...held)
      held = []
    }
    if (parseExitLine(line) !== null) held = [line]
    else out.push(line)
  }
  return { ...state, lines: out, held, buffer }
}

/**
 * Close the stream and decide the verdict.
 *
 * Flushes a trailing line that never got its newline — a server that ends the
 * response right after `exit=1` would otherwise leave the code unparsed and the
 * card stuck on "installing" — then reads the exit code off the last non-empty
 * line, if that is what it is. No sentinel means no verdict: `exitCode` stays
 * null and `terminated` false, which the UI must show as "result unknown"
 * rather than guessing either way.
 */
export function endInstallStream(
  state: InstallStreamState,
  opts?: { broken?: boolean },
): InstallStreamState {
  const flushed = state.buffer ? feedInstallStream(state, '\n') : state
  const exitCode = flushed.held.length > 0 ? parseExitLine(flushed.held[0]) : null
  return {
    ...flushed,
    // Trailing blanks after the sentinel are dropped: whitespace at the end of
    // a log is noise, and the sentinel itself is protocol, not output.
    held: [],
    buffer: '',
    closed: true,
    broken: opts?.broken ?? flushed.broken,
    exitCode,
    terminated: exitCode !== null,
  }
}

/**
 * Read a `POST …/install` body to the end, reporting progress as it arrives.
 *
 * Never throws. A socket that dies mid-install (server restart, app quit) tells
 * us nothing about what the command did, so it is an outcome the card has to
 * draw — `broken`, no verdict — not an exception for the click handler to drop
 * on the floor, which would leave the row spinning on "Installing…" forever.
 *
 * `onUpdate` is called per chunk so the log panel fills live rather than in
 * one jump at completion.
 */
export async function readInstallStream(
  body: ReadableStream<Uint8Array>,
  onUpdate: (state: InstallStreamState) => void,
): Promise<InstallStreamState> {
  const reader = body.getReader()
  // stream: true so a multi-byte character split across two chunks is decoded
  // once it is whole instead of becoming a replacement character.
  const decoder = new TextDecoder()
  let state = newInstallStream()
  let broken = false
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      state = feedInstallStream(state, decoder.decode(value, { stream: true }))
      onUpdate(state)
    }
    state = feedInstallStream(state, decoder.decode())
  } catch {
    broken = true
  } finally {
    try {
      reader.releaseLock()
    } catch {
      /* already released by an errored stream */
    }
  }
  state = endInstallStream(state, { broken })
  onUpdate(state)
  return state
}

/* ── What the card shows ── */

export type IntegrationStatus =
  | 'checking'
  | 'running'
  | 'failed'
  /** An attempt finished without reporting a status. Neither done nor failed. */
  | 'result-unknown'
  | 'installed'
  | 'not-installed'
  /** The server looked and could not tell. */
  | 'unknown'

export interface IntegrationStatusInput {
  /** A list fetch is in flight. */
  loading: boolean
  /** An install for this row is in flight — ours, or one the server reported busy. */
  running: boolean
  installed: boolean | null
  /** Exit code of the last install attempt in this session, if any. */
  failedExit: number | null
  /** That attempt ended with no `exit=` line: outcome genuinely unknown. */
  resultUnknown?: boolean
}

/**
 * The one pill state, in precedence order.
 *
 * A run in progress outranks everything: the previous verdict is about to be
 * replaced and showing it as current is the lie that makes people click twice.
 * An attempt with no verdict outranks the stored detection for the same reason
 * in reverse — the mirror image of guessing success is quietly re-showing
 * "Not installed" for a command that may well have worked.
 */
export function integrationStatus(input: IntegrationStatusInput): IntegrationStatus {
  if (input.running) return 'running'
  if (input.failedExit !== null && input.failedExit !== 0) return 'failed'
  if (input.resultUnknown === true) return 'result-unknown'
  if (input.loading) return 'checking'
  if (input.installed === true) return 'installed'
  if (input.installed === false) return 'not-installed'
  return 'unknown'
}

/**
 * What to say after an attempt has finished and (on success) the list has been
 * re-read. Kept out of the component so each branch is a test, not a click.
 */
export type PostRunNote =
  /** The stream never reported a status — say so and point at Re-check. */
  | 'no-verdict'
  /**
   * exit=0, and the re-check still does not see it. Do NOT overrule the
   * detection: registration can settle late, or the command can succeed at
   * less than the user asked for. Say both facts and leave the log up.
   */
  | 'ok-undetected'
  | 'none'

export function postRunNote(input: {
  terminated: boolean
  exitCode: number | null
  /** What the re-check said, or the stored value when there was no re-check. */
  installedAfter: boolean | null
}): PostRunNote {
  if (!input.terminated) return 'no-verdict'
  if (input.exitCode === 0 && input.installedAfter !== true) return 'ok-undetected'
  return 'none'
}

/** Which word the action button carries. */
export function actionLabelKind(
  installed: boolean | null,
  failedExit: number | null,
): 'install' | 'update' | 'retry' {
  if (failedExit !== null && failedExit !== 0) return 'retry'
  return installed === true ? 'update' : 'install'
}

/** True when the button must not be clickable: unmet prerequisite, or nothing to run. */
export function installBlocked(item: IntegrationItem): boolean {
  if (!item.command) return true
  return item.prerequisite !== null && !item.prerequisite.ok
}

/* ── Tab visibility ── */

/**
 * Settings tabs that only exist inside the desktop app, because the server
 * behind them only exists there. A browser tab asking `/desktop/integrations`
 * gets a 404 from `gadak serve`, so the tab must not be offered — including
 * via a `settings=integrations` URL somebody pasted out of the desktop app.
 */
export const DESKTOP_ONLY_SETTINGS_TABS: readonly string[] = ['integrations']

/** The tabs this surface may show, in the given order. */
export function visibleSettingsTabs<T extends string>(all: readonly T[], desktop: boolean): T[] {
  return all.filter((tab) => desktop || !DESKTOP_ONLY_SETTINGS_TABS.includes(tab))
}

/** Whether an incoming tab name is one this surface can open. */
export function isVisibleSettingsTab(
  value: string,
  all: readonly string[],
  desktop: boolean,
): boolean {
  if (!all.includes(value)) return false
  return desktop || !DESKTOP_ONLY_SETTINGS_TABS.includes(value)
}

/* ── Network ── */

/** The subset of `fetch` these two calls use, so tests can hand in their own. */
export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>

/** GET the list. Throws on a non-OK response — an empty list means "none", not "unreachable". */
export async function fetchIntegrations(fetchImpl: FetchLike = fetch): Promise<IntegrationItem[]> {
  const res = await fetchImpl(INTEGRATIONS_PATH)
  if (!res.ok) throw new Error(`GET ${INTEGRATIONS_PATH} → ${res.status}`)
  return normalizeIntegrations(await res.json())
}

export type InstallFailure =
  /** The server does not know this integration (404). */
  | 'unknown-id'
  /** One is already running (409) — the UI keeps showing the running state. */
  | 'already-running'
  /** Anything else, including no server at all. */
  | 'unavailable'

export type InstallStart =
  | { stream: ReadableStream<Uint8Array> }
  | { failure: InstallFailure; status: number }

export interface StartFailureOutcome {
  /**
   * An install really is running, just not one we can watch (409). The row must
   * keep saying "Installing…" with its button disabled — reporting this as an
   * error would invite a second click at the one moment a second run is wrong.
   */
  foreignRunning: boolean
  noteKind: 'busy' | 'unknown-id' | 'start-failed'
}

/** How a refused start reads on the card. */
export function startFailureOutcome(failure: InstallFailure): StartFailureOutcome {
  if (failure === 'already-running') return { foreignRunning: true, noteKind: 'busy' }
  if (failure === 'unknown-id') return { foreignRunning: false, noteKind: 'unknown-id' }
  return { foreignRunning: false, noteKind: 'start-failed' }
}

/**
 * Start an install. Never throws: a dead socket is one of the outcomes the
 * card has to draw, not an exception for the click handler to swallow.
 */
export async function postInstall(id: string, fetchImpl: FetchLike = fetch): Promise<InstallStart> {
  let res: Response
  try {
    res = await fetchImpl(installPath(id), { method: 'POST' })
  } catch {
    return { failure: 'unavailable', status: 0 }
  }
  if (res.status === 404) return { failure: 'unknown-id', status: 404 }
  if (res.status === 409) return { failure: 'already-running', status: 409 }
  if (!res.ok || res.body === null) return { failure: 'unavailable', status: res.status }
  return { stream: res.body }
}
