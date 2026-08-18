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

export const WINDOW_CHROME_NATIVE = 'native'
export const WINDOW_CHROME_TRAFFIC_LIGHTS_INSET = 'traffic-lights-inset'
export type WindowChrome =
  | typeof WINDOW_CHROME_NATIVE
  | typeof WINDOW_CHROME_TRAFFIC_LIGHTS_INSET

/** Optional surfaces. Each one needs a server capability that may be absent. */
export interface GadakFeatures {
  /** Personal activity feed (mentions, watched issues, assignment changes). */
  feed: boolean
  /** Web Push notifications for feed events. */
  push: boolean
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
  features: GadakFeatures
}

export const WORKSPACE_KIND_CONNECTED = 'connected'
export const WORKSPACE_KIND_STANDALONE = 'standalone'
export type WorkspaceKind =
  | typeof WORKSPACE_KIND_CONNECTED
  | typeof WORKSPACE_KIND_STANDALONE
  | ''

/** CLI that creates a standalone workspace. Flag lives in cmd/gadak/init.go. */
export const STANDALONE_INIT_COMMAND = 'gadak init --standalone'

export function parseWorkspaceKind(raw: unknown): WorkspaceKind {
  if (raw === WORKSPACE_KIND_STANDALONE || raw === WORKSPACE_KIND_CONNECTED) {
    return raw
  }
  return ''
}

/** True only when the server said standalone. Unknown and connected are false. */
export function isStandaloneWorkspace(): boolean {
  return current.workspaceKind === WORKSPACE_KIND_STANDALONE
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
  features: {
    feed: false,
    push: false,
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

export function parseWindowChrome(raw: unknown): WindowChrome | undefined {
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

/** Vite `base` at runtime, always with a trailing slash. */
export function basePath(): string {
  const base = import.meta.env.BASE_URL || '/'
  return base.endsWith('/') ? base : `${base}/`
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
export function siteHost(siteUrl: string): string {
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
      }
    }
  } catch {
    /* offline / not served by gadak — keep defaults */
  }
  return current
}
