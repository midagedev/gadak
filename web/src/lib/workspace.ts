/*
 * Workspace origin kind — single owner for "is this standalone?".
 *
 * The server sends workspaceKind on config.json (origin.Describe). Surfaces
 * must ask here; they must not infer standalone from an empty jiraBaseUrl
 * (hosted demo and older documents also have no site) and must not scatter
 * `=== 'standalone'` comparisons.
 *
 * Creating a standalone workspace has two doors: the onboarding wizard's
 * "Start with no tracker" button (POST onboarding/standalone, GDK-377) and
 * the CLI verb below. Both seed the same workspace through the shared core
 * (internal/originbind.SeedStandalone). The sidebar and settings hints still
 * show the command form — it is the one that names the workspace.
 */

export const WORKSPACE_KIND_CONNECTED = 'connected'
export const WORKSPACE_KIND_STANDALONE = 'standalone'
export type WorkspaceKind =
  | typeof WORKSPACE_KIND_CONNECTED
  | typeof WORKSPACE_KIND_STANDALONE
  | ''

/** CLI that creates a standalone workspace. Flag lives in cmd/gadak/init.go; the wizard button is the GUI equivalent. */
export const STANDALONE_INIT_COMMAND = 'gadak --workspace <name> init --standalone'

export function parseWorkspaceKind(raw: unknown): WorkspaceKind {
  if (raw === WORKSPACE_KIND_STANDALONE || raw === WORKSPACE_KIND_CONNECTED) {
    return raw
  }
  return ''
}

/**
 * True only when the server said standalone. Unknown and connected are false.
 * `jiraBaseUrl` is ignored on purpose — an empty site is not this kind.
 */
export function isStandalone(cfg: unknown): boolean {
  if (!cfg || typeof cfg !== 'object') return false
  return (
    parseWorkspaceKind((cfg as { workspaceKind?: unknown }).workspaceKind) ===
    WORKSPACE_KIND_STANDALONE
  )
}
