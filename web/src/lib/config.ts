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
   * True only inside the desktop app, which serves its own config.json. The app
   * hides the native title bar, so the window controls land on top of the first
   * row of the UI: that row has to reserve their corner and act as the drag
   * handle. A browser tab has neither, hence the flag rather than a media query.
   */
  desktop: boolean
  features: GadakFeatures
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
      }
    }
  } catch {
    /* offline / not served by gadak — keep defaults */
  }
  return current
}
