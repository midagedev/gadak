/*
 * Runtime configuration.
 *
 * The built bundle must stay tenant-neutral: no Jira site URL, project key,
 * team label, or API base is allowed to be hardcoded in application code.
 * `scry serve` writes the effective config to `<base>config.json`, this module
 * loads it before mount, and everything else reads it through `config()`.
 *
 * Missing or unreachable config falls back to DEFAULTS, so the app still boots
 * (with optional surfaces switched off) when served as plain static files.
 */

/** Optional surfaces. Each one needs a server capability that may be absent. */
export interface ScryFeatures {
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

export interface ScryConfig {
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
   * True only for the public hosted demo: a static snapshot on someone else's
   * domain, where a service worker answers every write with 501. Nothing here
   * reaches a Jira site, so the UI must say it is a demo and must not offer to
   * take an API token — a credential box on a page like this invites a visitor
   * to paste a real token somewhere it can do them no good.
   */
  hostedDemo: boolean
  features: ScryFeatures
}

const DEFAULTS: ScryConfig = {
  apiBase: '/api/v1/issues/',
  authBase: '/api/v1/auth/',
  jiraBaseUrl: '',
  qaDashboardUrl: '',
  projects: [],
  groupLabels: {},
  groupColors: {},
  productByGroup: {},
  staleThresholdHours: 72,
  hostedDemo: false,
  features: {
    feed: false,
    push: false,
    deploy: false,
    qa: false,
    teamGroups: false,
  },
}

let current: ScryConfig = DEFAULTS

export function config(): ScryConfig {
  return current
}

/**
 * Whether an optional surface is switched on. Every consumer of a gated surface
 * (column, filter field, grouping mode, panel, network call) asks here — a flag
 * that nobody reads is a flag that does nothing.
 */
export function feature(name: keyof ScryFeatures): boolean {
  return current.features[name]
}

/** True on the public hosted demo. See ScryConfig.hostedDemo. */
export function isHostedDemo(): boolean {
  return current.hostedDemo
}

/**
 * Deep link to the issue on the configured Jira site. Returns null when no site
 * is configured, so callers render plain text instead of a broken link.
 */
export function jiraBrowseUrl(issueKey: string): string | null {
  const base = config().jiraBaseUrl.replace(/\/+$/, '')
  return base ? `${base}/browse/${encodeURIComponent(issueKey)}` : null
}

/** Vite `base` at runtime, always with a trailing slash. */
export function basePath(): string {
  const base = import.meta.env.BASE_URL || '/'
  return base.endsWith('/') ? base : `${base}/`
}

/**
 * Fetch `<base>config.json` and merge it over DEFAULTS. Never throws — a missing
 * or malformed file leaves the defaults in place so the shell still renders.
 */
export async function loadConfig(): Promise<ScryConfig> {
  try {
    const res = await fetch(`${basePath()}config.json`, { credentials: 'same-origin' })
    if (res.ok) {
      const raw = (await res.json()) as Partial<ScryConfig>
      current = {
        ...DEFAULTS,
        ...raw,
        features: { ...DEFAULTS.features, ...(raw.features ?? {}) },
      }
    }
  } catch {
    /* offline / not served by scry — keep defaults */
  }
  return current
}
