/*
 * Runtime configuration.
 *
 * The built bundle must stay tenant-neutral: no Jira site URL, project key,
 * team label, or API base is allowed to be hardcoded in application code.
 * `gadak serve` writes the effective config to `<base>config.json`, this module
 * loads it before mount, and everything else reads it through `config()`.
 *
 * Missing or unreachable config falls back to DEFAULTS, so the app still boots
 * (with optional surfaces switched off) when served as plain static files.
 */

import {
  isLocalOrigin,
  parseOriginType,
  parseTransport,
  parseWorkspaceKind,
  type OriginType,
  type Transport,
  type WorkspaceKind,
} from './workspace'
import type { UiTokenDoc } from './user-tokens'

export {
  isLocalOrigin,
  parseOriginType,
  parseTransport,
  parseWorkspaceKind,
  STANDALONE_INIT_COMMAND,
  WORKSPACE_KIND_CONNECTED,
  WORKSPACE_KIND_STANDALONE,
  ORIGIN_GADAK,
  ORIGIN_JIRA,
  ORIGIN_LINEAR,
  TRANSPORT_LOCAL,
  TRANSPORT_REMOTE,
  type OriginType,
  type Transport,
  type WorkspaceKind,
} from './workspace'

export const WINDOW_CHROME_NATIVE = 'native'
export const WINDOW_CHROME_TRAFFIC_LIGHTS_INSET = 'traffic-lights-inset'
export type WindowChrome =
  | typeof WINDOW_CHROME_NATIVE
  | typeof WINDOW_CHROME_TRAFFIC_LIGHTS_INSET

/** Optional surfaces. Each one needs a server capability that may be absent. */
export interface GadakFeatures {
  /** Personal activity feed (mentions, watched issues, assignment changes). */
  feed: boolean
  /** Deployment state per issue, sourced from an external CI/CD index. */
  deploy: boolean
  /** Test-management context per issue, sourced from an external QA tool. */
  qa: boolean
  /** Team/part taxonomy grouping (needs a directory that maps people to groups). */
  teamGroups: boolean
}

export interface GadakConfig {
  /** Base path of the issues REST API, trailing slash included. */
  apiBase: string
  /** Base path of the auth API, trailing slash included. */
  authBase: string
  /** Jira site origin, e.g. `https://your-team.atlassian.net`. Empty disables deep links. */
  jiraBaseUrl: string
  /** Optional external QA dashboard origin for issue-to-test-run links. Empty hides them. */
  qaDashboardUrl: string
  /** Project keys the mirror covers. Drives built-in view presets. */
  projects: string[]
  /** Group key -> display label, for the optional team taxonomy. */
  groupLabels: Record<string, string>
  /** Group key -> hex color, for avatar rings. */
  groupColors: Record<string, string>
  /** Group key -> product bucket, used by the `product` grouping mode. */
  productByGroup: Record<string, { key: string; label: string }>
  /** Hours in the current status before an unresolved issue counts as stale. */
  staleThresholdHours: number
  /**
   * Whether the Confluence source is configured. Not a page count: it answers
   * the question a mirror with zero documents cannot — "is this switched off,
   * or switched on and not filled yet". The sidebar tells those two apart with
   * it, so an empty DOCS section stops advertising a setup the user already did.
   */
  confluenceEnabled: boolean
  /**
   * True only for the public hosted demo: a static snapshot on someone else's
   * domain, where a service worker answers every write with 501. Nothing here
   * reaches a Jira site, so the UI must say it is a demo and must not offer to
   * take an API token — a credential box on a page like this invites a visitor
   * to paste a real token somewhere it can do them no good.
   */
  hostedDemo: boolean
  /**
   * True only inside the desktop app, which serves its own config.json. A
   * browser tab is not the app — chrome (title-bar inset vs native) is a
   * separate field, because not every desktop window hides its title bar.
   */
  desktop: boolean
  /**
   * Where the window controls live. `traffic-lights-inset` means they sit
   * inside the first content row (that row reserves their corner). `native`
   * means the platform draws a title bar — do not reserve the macOS
   * traffic-light gap. The desktop app always sends this next to `desktop`.
   * Documents that omit it keep the old meaning: desktop implied inset.
   */
  windowChrome: WindowChrome
  /**
   * gadak profile this document belongs to. Servers send `"default"` for the
   * unnamed profile (same as `gadak doctor --json`). Older documents omit it;
   * DEFAULTS fill `"default"`.
   */
  profile: string
  /**
   * GOOS of the process serving this document — the machine an upgrade would
   * happen on. Empty means unknown (static export, hosted demo, an older
   * server), and copy that names a platform command must stay silent rather
   * than guess: `brew upgrade --cask gadak` is wrong on Linux and Windows.
   */
  os: string
  /**
   * Workspace origin kind from the server (`origin.Describe`). Empty means
   * unknown (static export, hosted demo, an older server). Do not infer this
   * from an empty `jiraBaseUrl` — that is also true of the hosted demo.
   */
  workspaceKind: WorkspaceKind
  /**
   * Which tracker the origin is (GDK-1278). Empty means unknown — a static
   * export, the hosted demo, or a server older than the split.
   */
  originType: OriginType
  /**
   * Whether that origin runs in the server's own process or behind a serve
   * API. Empty means unknown. A loopback serve is 'remote': the axis is the
   * transport, not the distance.
   */
  transport: Transport
  /**
   * True when the server can reach the Jira-family origin — a site
   * credential, a local-origin workspace, or a pairing remote. It mirrors
   * `config.HasAtlassianCredential`, which is the same bool every write path
   * 409s on, so a surface that decides whether to send an origin request at
   * all agrees with the server instead of guessing (GDK-1090). Absent
   * (static export, hosted demo) is false — correct there.
   *
   * Not `me.identified`: that reads auth/me's email, which is empty on a
   * local-origin and on a paired workspace even though both write fine.
   */
  originWritable: boolean
  features: GadakFeatures
  /**
   * Server-merged color overrides (GDK-786/791): final per-palette CSS
   * variable map + data inks, already validated. Absent on an older server
   * or static export means "nothing overridden" — app.css defaults rule.
   */
  ui?: UiTokenDoc
  /**
   * Disk identity (mtime.size) of this profile's config.json. The ui-focus
   * poll carries it; when it moves, another surface wrote settings (CLI
   * `config set`, another tab) and the app refetches this document instead
   * of reloading. Absent on older servers disables that signal.
   */
  configVersion?: string
}

/** True only when the server said localOrigin. Unknown and connected are false. */
export function isLocalOriginWorkspace(): boolean {
  return isLocalOrigin(current)
}

/**
 * True when the origin can answer a Jira-family request — the client mirror
 * of `config.HasAtlassianCredential` (GDK-1090). Ask this before sending a
 * request whose only other outcome is 409 credential_required; do not
 * reconstruct the predicate from identity or from an empty site URL.
 */
export function originWritable(): boolean {
  return current.originWritable
}

const DEFAULTS: GadakConfig = {
  apiBase: '/api/v1/issues/',
  authBase: '/api/v1/auth/',
  jiraBaseUrl: '',
  qaDashboardUrl: '',
  projects: [],
  groupLabels: {},
  groupColors: {},
  productByGroup: {},
  staleThresholdHours: 72,
  confluenceEnabled: false,
  hostedDemo: false,
  desktop: false,
  windowChrome: WINDOW_CHROME_NATIVE,
  profile: 'default',
  os: '',
  workspaceKind: '',
  originType: '',
  transport: '',
  originWritable: false,
  features: {
    feed: false,
    deploy: false,
    qa: false,
    teamGroups: false,
  },
}

let current: GadakConfig = DEFAULTS

export function config(): GadakConfig {
  return current
}

/**
 * Whether an optional surface is switched on. Every consumer of a gated surface
 * (column, filter field, grouping mode, panel, network call) asks here — a flag
 * that nobody reads is a flag that does nothing.
 */
export function feature(name: keyof GadakFeatures): boolean {
  return current.features[name]
}

/** True on the public hosted demo. See GadakConfig.hostedDemo. */
export function isHostedDemo(): boolean {
  return current.hostedDemo
}

/** True inside the desktop app window. See GadakConfig.desktop. */
export function isDesktop(): boolean {
  return current.desktop
}

function parseWindowChrome(raw: unknown): WindowChrome | undefined {
  if (raw === WINDOW_CHROME_TRAFFIC_LIGHTS_INSET || raw === WINDOW_CHROME_NATIVE) {
    return raw
  }
  return undefined
}

/**
 * Chrome token for a config document. A missing field keeps the pre-GDK-207
 * meaning (desktop implied traffic lights in the content) so older mocks and
 * documents do not silently drop the inset. An explicit `native` wins.
 */
export function resolveWindowChrome(doc: {
  desktop?: boolean
  windowChrome?: unknown
}): WindowChrome {
  return parseWindowChrome(doc.windowChrome) ?? (doc.desktop ? WINDOW_CHROME_TRAFFIC_LIGHTS_INSET : WINDOW_CHROME_NATIVE)
}

/** Current window chrome. See GadakConfig.windowChrome. */
export function windowChrome(): WindowChrome {
  return current.windowChrome
}

/** True when the window controls sit in the first content row. */
export function trafficLightsInContent(): boolean {
  return current.windowChrome === WINDOW_CHROME_TRAFFIC_LIGHTS_INSET
}

/** Logo-row classes: 90px inset + drag only when traffic lights are in content. */
export function sidebarLogoRowClass(): string {
  return trafficLightsInContent() ? 'desktop-titlebar-row' : 'px-4'
}

/** Which shell is showing this bundle. Settings copy branches here. */
export type GadakSurface = 'serve' | 'desktop' | 'hosted'

/**
 * Single surface discriminator for settings copy and control visibility.
 * `hostedDemo` wins if both flags were ever set: the public snapshot is not
 * the desktop app. Callers that already used isDesktop()/isHostedDemo() for
 * chrome (title bar, write gate) keep those; new settings branches use this.
 */
export function surface(): GadakSurface {
  if (current.hostedDemo) return 'hosted'
  if (current.desktop) return 'desktop'
  return 'serve'
}

/**
 * Server verbs a deployment can actually answer. `feature()` says whether an
 * optional surface is switched on; this says whether the shell serving this
 * bundle has a server behind it at all. A static snapshot has none of them:
 * the in-page adapter serves issue JSON off disk and 404s every other path.
 *
 * Surfaces ask here BEFORE they render an entry point. A verb that cannot work
 * must not look like it can, and must not be discovered by failing at click
 * time — "the network is broken" is a lie when the deployment simply has no
 * server to ask. Adding a surface that needs the server? Ask here, not the
 * network.
 */
export const SERVER_VERBS = [
  /** Full-text search over issue/page bodies (`GET api/v1/issues/search/`). */
  'bodySearch',
  /** Mirrored Confluence pages (the snapshot carries issues only). */
  'docs',
  /** The server settings document (`GET/PUT api/v1/issues/settings/`). */
  'settings',
] as const

export type ServerVerb = (typeof SERVER_VERBS)[number]

export function hasServerVerb(_v: ServerVerb): boolean {
  return surface() !== 'hosted'
}

/**
 * All server verbs and whether this deployment answers them. Diagnostics only.
 */
export function serverVerbReport(): Record<ServerVerb, boolean> {
  const report = {} as Record<ServerVerb, boolean>
  for (const v of SERVER_VERBS) report[v] = hasServerVerb(v)
  return report
}

/**
 * Deep link to the issue on the configured Jira site. Returns null when no site
 * is configured, so callers render plain text instead of a broken link.
 */
export function jiraBrowseUrl(issueKey: string): string | null {
  const base = config().jiraBaseUrl.replace(/\/+$/, '')
  return base ? `${base}/browse/${encodeURIComponent(issueKey)}` : null
}

/**
 * Jira issue navigator for a saved filter (`?filter=<id>`), falling back to
 * the raw JQL when we only have the query text. Null with no site configured.
 */
export function jiraFilterUrl(filterId: string, jql?: string): string | null {
  const base = config().jiraBaseUrl.replace(/\/+$/, '')
  if (!base) return null
  const id = filterId.trim()
  if (id) return `${base}/issues/?filter=${encodeURIComponent(id)}`
  const q = (jql ?? '').trim()
  if (q) return `${base}/issues/?jql=${encodeURIComponent(q)}`
  return null
}

/**
 * App URL prefix, always with a trailing slash.
 *
 * Hosted /demo/ and /backlog/ share one Vite bundle (GDK-673), so this cannot
 * be compile-time `import.meta.env.BASE_URL` — that value is only the asset
 * emit (often `./`). The prefix is, in order:
 *   1. the document `<base href>` (injected per mount by the hosted build)
 *   2. this module's own URL, when Vite emitted a relative base — the JS
 *      lives at `{prefix}assets/…`, which is not `/w/<name>/` (gadak serve
 *      still ships assets at `/assets/` even on a workspace mount)
 *   3. compile-time BASE_URL for `gadak serve` / desktop (`/`)
 *
 * `/w/<name>/` is workspaceName(), not a base. Both read the page URL, but
 * this function never inspects `location.pathname`.
 */
export function basePath(): string {
  // Resolved once: hosted-fetch asks on every intercepted request, and the
  // mount cannot change without a navigation. Doing the DOM lookup and the
  // data-base-path write per call would put both on the request path.
  if (resolvedBase !== null) return resolvedBase
  resolvedBase = resolveBasePath()
  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.dataset.basePath = resolvedBase
  }
  return resolvedBase
}

let resolvedBase: string | null = null

function withTrailingSlash(path: string): string {
  return path.endsWith('/') ? path : `${path}/`
}

function isRelativeViteBase(base: string): boolean {
  return base === './' || base === '.' || base === ''
}

function resolveBasePath(): string {
  const fromTag = baseHrefFromDocument()
  if (fromTag) return fromTag
  const compiled = import.meta.env.BASE_URL || '/'
  if (isRelativeViteBase(compiled)) {
    return baseFromModuleUrl() ?? '/'
  }
  return withTrailingSlash(compiled)
}

function baseHrefFromDocument(): string | null {
  if (typeof document === 'undefined') return null
  const el = document.querySelector('base[href]')
  if (!el) return null
  const raw = el.getAttribute('href')
  if (raw == null || isRelativeViteBase(raw)) return null
  try {
    return withTrailingSlash(new URL(raw, 'http://gadak.invalid/').pathname)
  } catch {
    return raw.startsWith('/') ? withTrailingSlash(raw) : null
  }
}

/** `{origin}{prefix}assets/{file}` → `{prefix}`. `file:` is vitest; ignore it. */
function baseFromModuleUrl(): string | null {
  let href: string
  try {
    href = import.meta.url
  } catch {
    return null
  }
  if (!href || href.startsWith('file:')) return null
  try {
    const path = new URL(href).pathname
    const marker = '/assets/'
    const i = path.lastIndexOf(marker)
    if (i < 0) return null
    const prefix = path.slice(0, i + 1)
    return prefix === '' ? '/' : prefix
  } catch {
    return null
  }
}

/**
 * Workspace mount name when the app is served under /w/<name>/ (one gadak
 * process serving several profile mirrors), '' on the primary. Build-time
 * BASE_URL cannot know this — the same bundle serves every mount — so it is
 * derived from the URL path at runtime.
 */
export function workspaceName(): string {
  const m = window.location.pathname.match(/^\/w\/([A-Za-z0-9_-]+)(\/|$)/)
  return m ? m[1] : ''
}

/** Host of a Jira site URL, or '' when none. Used only as a cache partition. */
function siteHost(siteUrl: string): string {
  const raw = siteUrl.trim()
  if (!raw) return ''
  try {
    const u = new URL(raw.includes('://') ? raw : `https://${raw}`)
    return u.host.toLowerCase()
  } catch {
    return raw.toLowerCase().replace(/[^a-z0-9.-]+/g, '-')
  }
}

/** Stable profile token for cache keys and config.json. Empty/"default" → "default". */
export function profileName(raw: string | null | undefined): string {
  const p = (raw ?? '').trim()
  return !p || p === 'default' ? 'default' : p
}

/**
 * Cache partition for IndexedDB / localStorage. Distinct workspace mounts,
 * Jira sites, and (on `/`) named profiles must never share a pool.
 * `profile` is omitted when it is the default name so existing default-profile
 * caches keep their key; a named primary on the same origin+site does not.
 */
export function composeCacheScope(workspace: string, siteUrl: string, profile?: string): string {
  const parts: string[] = []
  const ws = workspace.trim()
  if (ws) parts.push(`ws:${ws}`)
  const host = siteHost(siteUrl)
  if (host) parts.push(`site:${host}`)
  if (!ws) {
    const p = profileName(profile ?? config().profile)
    if (p !== 'default') parts.push(`profile:${p}`)
  }
  return parts.join('|')
}

/** Active partition. Empty on the hosted demo (no site, no workspace). */
export function cacheScopeId(): string {
  return composeCacheScope(workspaceName(), config().jiraBaseUrl, config().profile)
}

/** Expose the active partition so a mixed-profile cache is visible immediately. */
export function applyCacheScopeDebug(): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.cacheScope = cacheScopeId() || 'primary'
}

/**
 * Where this page's config.json and API live: the workspace mount when
 * present, the build base otherwise.
 */
export function runtimeBase(): string {
  const ws = workspaceName()
  return ws ? `/w/${ws}/` : basePath()
}

/**
 * Fetch `<base>config.json` and merge it over DEFAULTS. Never throws — a missing
 * or malformed file leaves the defaults in place so the shell still renders.
 */
export async function loadConfig(): Promise<GadakConfig> {
  try {
    const res = await fetch(`${runtimeBase()}config.json`, { credentials: 'same-origin' })
    if (res.ok) {
      const raw = (await res.json()) as Partial<GadakConfig>
      current = {
        ...DEFAULTS,
        ...raw,
        features: { ...DEFAULTS.features, ...(raw.features ?? {}) },
        windowChrome: resolveWindowChrome(raw),
        workspaceKind: parseWorkspaceKind(raw.workspaceKind),
        originType: parseOriginType(raw.originType),
        transport: parseTransport(raw.transport),
        originWritable: raw.originWritable === true,
      }
    }
  } catch {
    /* offline / not served by gadak — keep defaults */
  }
  return current
}
