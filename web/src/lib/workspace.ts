/*
 * Workspace origin kind — single owner for "is this standalone?".
 *
 * The server sends workspaceKind on config.json (origin.Describe). Surfaces
 * must ask here; they must not infer standalone from an empty jiraBaseUrl
 * (hosted demo and older documents also have no site) and must not scatter
 * `=== 'standalone'` comparisons.
 *
 * Creating a standalone workspace is still CLI-only this round
 * (cmd/gadak/init.go --standalone). The command below is what the UI shows.
 */

export const WORKSPACE_KIND_CONNECTED = 'connected'
export const WORKSPACE_KIND_STANDALONE = 'standalone'
export type WorkspaceKind =
  | typeof WORKSPACE_KIND_CONNECTED
  | typeof WORKSPACE_KIND_STANDALONE
  | ''

/** CLI that creates a standalone workspace. Flag lives in cmd/gadak/init.go. */
export const STANDALONE_INIT_COMMAND = 'gadak --profile <name> init --standalone'

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
