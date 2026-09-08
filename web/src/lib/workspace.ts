/*
 * Workspace origin kind — single owner for "is this built-in?".
 *
 * The server sends workspaceKind on config.json (origin.Describe). Surfaces
 * must ask here; they must not infer built-in from an empty jiraBaseUrl
 * (hosted demo and older documents also have no site) and must not scatter
 * `=== 'standalone'` comparisons.
 *
 * Creating a built-in workspace has two doors: the onboarding wizard's
 * "Start with no tracker" button (POST onboarding/standalone, GDK-377) and
 * the CLI verb below. Both seed the same workspace through the shared core
 * (internal/originbind.SeedBuiltIn). The sidebar and settings hints still
 * show the command form — it is the one that names the workspace.
 */

export const WORKSPACE_KIND_CONNECTED = 'connected'
export const WORKSPACE_KIND_STANDALONE = 'standalone'
export type WorkspaceKind =
  | typeof WORKSPACE_KIND_CONNECTED
  | typeof WORKSPACE_KIND_STANDALONE
  | ''

/*
 * The two axes workspaceKind conflated (GDK-1278): which tracker the origin
 * is, and whether it is reached in-process or across a serve API. A paired
 * workspace is why they had to split — its kind is 'connected' while its
 * origin is gadak's own tracker on another machine. Both come from the
 * server; neither is ever inferred here.
 */
export const ORIGIN_JIRA = 'jira'
/**
 * Jira Server / Data Center (GDK-1635). A type of its own, not a flag on
 * ORIGIN_JIRA: the two share a name and little else. Where they genuinely
 * agree — browse URLs — use isJiraFamily(); everywhere else branch on the
 * type, because that is the difference the split exists to keep visible.
 */
export const ORIGIN_JIRA_SERVER = 'jira-server'
export const ORIGIN_LINEAR = 'linear'
export const ORIGIN_GADAK = 'gadak'
export type OriginType =
  | typeof ORIGIN_JIRA
  | typeof ORIGIN_JIRA_SERVER
  | typeof ORIGIN_LINEAR
  | typeof ORIGIN_GADAK
  | ''

export function isJiraFamily(t: OriginType): boolean {
  return t === ORIGIN_JIRA || t === ORIGIN_JIRA_SERVER
}

export const TRANSPORT_LOCAL = 'local'
export const TRANSPORT_REMOTE = 'remote'
export type Transport = typeof TRANSPORT_LOCAL | typeof TRANSPORT_REMOTE | ''

export function parseOriginType(raw: unknown): OriginType {
  if (raw === ORIGIN_JIRA || raw === ORIGIN_JIRA_SERVER || raw === ORIGIN_LINEAR || raw === ORIGIN_GADAK) {
    return raw
  }
  return ''
}

export function parseTransport(raw: unknown): Transport {
  if (raw === TRANSPORT_LOCAL || raw === TRANSPORT_REMOTE) {
    return raw
  }
  return ''
}

/** CLI that creates a workspace with a gadak origin. Flag lives in cmd/gadak/init.go; the wizard button is the GUI equivalent. `--standalone` is still accepted there, but only one name is taught. */
export const STANDALONE_INIT_COMMAND = 'gadak --workspace <name> init --local'

export function parseWorkspaceKind(raw: unknown): WorkspaceKind {
  if (raw === WORKSPACE_KIND_STANDALONE || raw === WORKSPACE_KIND_CONNECTED) {
    return raw
  }
  return ''
}

/**
 * True only when the server said builtIn. Unknown and connected are false.
 * `jiraBaseUrl` is ignored on purpose — an empty site is not this kind.
 */
export function isBuiltIn(cfg: unknown): boolean {
  if (!cfg || typeof cfg !== 'object') return false
  return (
    parseWorkspaceKind((cfg as { workspaceKind?: unknown }).workspaceKind) ===
    WORKSPACE_KIND_STANDALONE
  )
}
