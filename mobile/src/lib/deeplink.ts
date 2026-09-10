/*
 * GDK-873 — `gadak://` links, from iOS to this app.
 *
 * An agent, the desktop app, or a chat message hands over a link
 * (`gadak views open --json` prints one as `deep_link`, cmd/gadak/views.go)
 * and one tap opens that issue here. This module is the pure half: text in,
 * a decision out. Nothing here touches the store, the network, or Tauri —
 * see entry.ts for the wiring, which is what makes this testable without a
 * simulator.
 *
 * ── The grammar is NOT owned here ──
 *
 * internal/deeplink/deeplink.go owns it. This is a second implementation of
 * the same rules because the phone cannot run Go, and a second
 * implementation of a grammar is precisely the drift this repo has already
 * paid for once. So the two are pinned to one table:
 * internal/deeplink/testdata/vectors.json is emitted by the Go parser and
 * consumed, case for case, by deeplink.test.ts. Changing the grammar means
 * regenerating that file, which turns this suite red until parseGadakUrl
 * agrees.
 *
 * The parse is hand-written rather than built on `new URL()` on purpose.
 * Measured while writing this: WHATWG URL and Go's net/url disagree on two
 * inputs that matter here — WHATWG normalizes `/w/../x` down to `/x`, so a
 * traversal Go refuses would have been accepted, and it splits `view:7777`
 * into host+port, so a link with a port would have parsed as the `view`
 * action instead of being refused. Both are exactly the class of input a
 * hostile page would try.
 *
 * ── Security posture (inherited from the Go package comment) ──
 *
 * Any web page can embed a gadak:// link, so the worst one may achieve is
 * that the user briefly looks at the wrong thing. The scheme carries no
 * verb: a link says where to go, never what to do. Nothing in this file may
 * write, submit, pair, or dial.
 *
 * The specific risk this module refuses (GDK-873): a link carrying
 * `/w/<profile>` names a workspace mount on someone's *desktop*. The phone
 * has no profile concept at all — it holds one paired host and the issues
 * that host serves (there is no `/w/` anywhere in mobile/src) — so a
 * profile-bearing link cannot be verified here and must not be treated as
 * this phone's. Opening `GDK-119` from a stranger's workspace on this
 * phone's mirror would show a different issue with the same key, silently.
 */

/** The scheme, matching internal/deeplink's `Scheme` constant. */
export const SCHEME = 'gadak'

/* ── grammar limits, mirroring internal/deeplink/deeplink.go ── */

const MAX_INPUT_LEN = 2048
const MAX_PARAMS = 32
const MAX_VALUE_LEN = 512
const MAX_KEY_LEN = 64

const ACTION_PATTERN = /^[a-z][a-z0-9-]{0,31}$/
const PROFILE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/
const SUBJECT_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.@+-]{0,127}$/
const KEY_PATTERN = new RegExp(`^[a-z][a-z0-9_.]{0,${MAX_KEY_LEN - 1}}$`)

/** The path segment that introduces a profile — the web UI's /w/<name>. */
const PROFILE_SEGMENT = 'w'

/** The only action the Go parser knows by name, and the one agents emit. */
export const ACTION_VIEW = 'view'

/** A parsed link. Field for field the Go `Link` struct. */
export interface GadakLink {
  /** Lowercased. An unrecognised one is a link from a newer gadak. */
  action: string
  /** '' means the primary mirror. */
  profile: string
  /** '' when the action needs none. */
  subject: string
  /** The raw query, verbatim: never decoded, re-encoded, or re-ordered. */
  hash: string
}

/**
 * Why a link was refused.
 *
 * `not_gadak` and `malformed` are the Go parser's two error classes and are
 * kept distinct for the same reason: another app's scheme reaching a shared
 * handler must pass in silence, while a link that IS ours and was refused
 * should say so.
 */
export type ParseFailure = 'not_gadak' | 'malformed'

export type ParseResult =
  | { ok: true; link: GadakLink }
  | { ok: false; failure: ParseFailure; why: string }

function no(failure: ParseFailure, why: string): ParseResult {
  return { ok: false, failure, why }
}

/**
 * Parses a `gadak://` URL. Pure: no I/O, no store, no globals.
 *
 * Returns `not_gadak` for a URL that is not ours (stay silent) and
 * `malformed` for one that is ours and refused (say so).
 */
export function parseGadakUrl(raw: string): ParseResult {
  const prefix = `${SCHEME}:`
  if (raw.slice(0, prefix.length).toLowerCase() !== prefix) {
    return no('not_gadak', 'not a gadak:// URL')
  }
  if (raw.length > MAX_INPUT_LEN) {
    return no('malformed', `input is ${raw.length} bytes, over the ${MAX_INPUT_LEN}-byte limit`)
  }
  // The grammar carries no fragment. Checked on the raw string so a bare
  // trailing '#' is caught too — the Go parser makes the same note.
  if (raw.includes('#')) return no('malformed', 'fragment not allowed')

  let rest = raw.slice(prefix.length)
  if (!rest.startsWith('//')) {
    // The opaque form (`gadak:view?…`) leaves the action empty in Go, which
    // then fails the action pattern. Same outcome, said plainly.
    return no('malformed', 'action "" is not a plain lowercase word')
  }
  rest = rest.slice(2)

  // The authority runs to the first '/' or '?'.
  let end = rest.length
  for (let i = 0; i < rest.length; i++) {
    const c = rest[i]
    if (c === '/' || c === '?') {
      end = i
      break
    }
  }
  const authority = rest.slice(0, end)
  const after = rest.slice(end)

  if (authority.includes('@')) return no('malformed', 'userinfo not allowed')
  // The port stays in the authority (Go keeps it in u.Host), so "view:7777"
  // fails the pattern rather than parsing as the "view" action.
  const action = authority.toLowerCase()
  if (!ACTION_PATTERN.test(action)) {
    return no('malformed', `action ${JSON.stringify(authority)} is not a plain lowercase word`)
  }

  const q = after.indexOf('?')
  const rawPath = q < 0 ? after : after.slice(0, q)
  const query = q < 0 ? '' : after.slice(q + 1)

  // Go validates the DECODED path, so an encoded separator can only add
  // segments — which the segment count and per-segment patterns then reject.
  let path: string
  try {
    path = decodeURIComponent(rawPath)
  } catch {
    return no('malformed', 'unparseable URL: bad percent-encoding in the path')
  }

  const split = splitPath(path)
  if ('why' in split) return no('malformed', split.why)

  const badQuery = validateQuery(query)
  if (badQuery) return no('malformed', badQuery)

  return { ok: true, link: { action, profile: split.profile, subject: split.subject, hash: query } }
}

/**
 * Reads the optional profile and subject out of the decoded path.
 *
 * Accepted: '', '/', '/w/<profile>', '/<subject>', '/w/<profile>/<subject>',
 * each with an optional trailing slash. A leading 'w' segment is reserved as
 * the profile introducer, so a subject may not be the single letter 'w'.
 */
function splitPath(path: string): { profile: string; subject: string } | { why: string } {
  const trimmed = path.replace(/^\/+/, '').replace(/\/+$/, '')
  if (trimmed === '') return { profile: '', subject: '' }
  const seg = trimmed.split('/')
  if (seg[0] === PROFILE_SEGMENT) {
    if (seg.length < 2) return { why: `empty profile after /${PROFILE_SEGMENT}/` }
    if (seg.length > 3) return { why: `path ${JSON.stringify(path)} has more segments than the grammar allows` }
    const profile = checkIdent('profile', seg[1]!, PROFILE_PATTERN)
    if ('why' in profile) return profile
    if (seg.length === 3) {
      const subject = checkIdent('subject', seg[2]!, SUBJECT_PATTERN)
      if ('why' in subject) return subject
      return { profile: profile.value, subject: subject.value }
    }
    return { profile: profile.value, subject: '' }
  }
  if (seg.length > 1) {
    return { why: `path ${JSON.stringify(path)} is not "/w/<profile>", "/<subject>", or both` }
  }
  const subject = checkIdent('subject', seg[0]!, SUBJECT_PATTERN)
  if ('why' in subject) return subject
  return { profile: '', subject: subject.value }
}

/**
 * Rejects the traversal names before the pattern. Both patterns already
 * exclude them by requiring a leading alphanumeric; the explicit check
 * states the invariant so a later pattern edit cannot quietly reopen it.
 */
function checkIdent(what: string, s: string, pattern: RegExp): { value: string } | { why: string } {
  if (s === '.' || s === '..') {
    return { why: `${what} ${JSON.stringify(s)} is a directory reference, not a name` }
  }
  if (!pattern.test(s)) return { why: `${what} ${JSON.stringify(s)} is not a plausible ${what}` }
  return { value: s }
}

/**
 * Enforces the size limits on the raw query. It never inspects what a
 * parameter means — there is deliberately no key allowlist, because the keys
 * the UI understands are owned by web/src/lib/view-config and a second copy
 * would drift silently in the direction that matters.
 */
function validateQuery(q: string): string | null {
  if (q === '') return null
  for (let i = 0; i < q.length; i++) {
    const c = q.charCodeAt(i)
    if (c < 0x20 || c === 0x7f) {
      return `control byte 0x${c.toString(16).padStart(2, '0')} in query`
    }
  }
  const parts = q.split('&')
  if (parts.length > MAX_PARAMS) {
    return `${parts.length} parameters, over the limit of ${MAX_PARAMS}`
  }
  const seen = new Set<string>()
  for (const part of parts) {
    const eq = part.indexOf('=')
    const key = eq < 0 ? part : part.slice(0, eq)
    const value = eq < 0 ? '' : part.slice(eq + 1)
    if (!KEY_PATTERN.test(key)) return `parameter key ${JSON.stringify(key)} does not match the key shape`
    if (value.length > MAX_VALUE_LEN) {
      return `value for ${JSON.stringify(key)} is ${value.length} bytes, over the ${MAX_VALUE_LEN}-byte limit`
    }
    if (seen.has(key)) return `duplicate parameter key ${JSON.stringify(key)}`
    seen.add(key)
  }
  return null
}

/* ── what the phone does with a parsed link ── */

/**
 * What a link asks this app to show. `open_issue` is the only target today,
 * which is a fact about the phone's screens rather than a limit of the
 * scheme: the phone has no router, only `app.detail` and a tab
 * (lib/store.svelte.ts), so an issue is the one thing that can be addressed.
 */
export type DeepLinkTarget = { kind: 'open_issue'; key: string }

/**
 * Why a well-formed link was not acted on.
 *
 * `unsupported_action` is deliberately distinct from a malformed link, the
 * same split desktop/deeplink.go makes: the grammar is fixed but the set of
 * actions grows, so this is what an older build says when it meets a newer
 * link — an upgrade prompt, not "your link is wrong".
 */
export type ResolveRefusal =
  | 'unsupported_action'
  | 'other_workspace'
  | 'no_subject_expected'
  | 'no_target'

export type ResolveResult =
  | { ok: true; target: DeepLinkTarget }
  | { ok: false; refusal: ResolveRefusal; why: string }

/**
 * The issue key inside a view hash. Matches the desk's `issue` place param
 * (web/src/lib/url-state.ts registers it), and the value is checked against
 * the shape a tracker key has so a hash param cannot smuggle a path.
 */
const ISSUE_KEY_PATTERN = /^[A-Za-z][A-Za-z0-9_]{0,31}-[0-9]{1,10}$/

/**
 * Turns a parsed link into what this app should show.
 *
 * Only `view` is honoured, and only its `issue=` param — that is what
 * `gadak views open` emits and what the phone can actually display. A view
 * link with filters but no issue is well-formed and simply has no phone
 * target: the phone has no filtered-list URL to land on.
 */
export function resolveDeepLink(link: GadakLink): ResolveResult {
  if (link.action !== ACTION_VIEW) {
    return { ok: false, refusal: 'unsupported_action', why: `this link needs a newer gadak (action "${link.action}")` }
  }
  if (link.profile !== '') {
    // The refusal this issue exists to make. See the module comment.
    return {
      ok: false,
      refusal: 'other_workspace',
      why: `this link is for the "${link.profile}" workspace on a computer; this phone is paired to one host and has no workspaces`,
    }
  }
  if (link.subject !== '') {
    return { ok: false, refusal: 'no_subject_expected', why: `a view link takes no subject, got "${link.subject}"` }
  }
  const key = issueKeyFromHash(link.hash)
  if (!key) {
    return { ok: false, refusal: 'no_target', why: 'this link points at a filtered list, which the phone cannot open' }
  }
  return { ok: true, target: { kind: 'open_issue', key } }
}

/**
 * Reads `issue=<KEY>` out of a view hash. Uses URLSearchParams for the
 * decoding only — the hash has already passed the grammar's shape and size
 * checks above, so this is not a second parser of the link.
 */
export function issueKeyFromHash(hash: string): string | null {
  if (hash === '') return null
  const raw = new URLSearchParams(hash).get('issue')
  if (!raw) return null
  const key = raw.trim()
  return ISSUE_KEY_PATTERN.test(key) ? key : null
}

/**
 * The whole decision in one call: text from the OS, an action or a reason.
 * Exported as the single entry point so the wiring has nothing to decide.
 */
export function decideDeepLink(
  raw: string,
): { ok: true; target: DeepLinkTarget } | { ok: false; reason: ParseFailure | ResolveRefusal; why: string } {
  const parsed = parseGadakUrl(raw)
  if (!parsed.ok) return { ok: false, reason: parsed.failure, why: parsed.why }
  const resolved = resolveDeepLink(parsed.link)
  if (!resolved.ok) return { ok: false, reason: resolved.refusal, why: resolved.why }
  return { ok: true, target: resolved.target }
}
