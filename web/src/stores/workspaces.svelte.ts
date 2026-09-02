/*
 * The mountable workspaces of this serve, read once per page. One
 * process-wide fact (the sidebar switcher and the palette both list it), so
 * one fetch — the palette's contract is zero /api/ requests of its own
 * (palette.spec: "typing stays local"), and the sidebar already loads the
 * list at boot.
 */
import { getWorkspaces, type WorkspaceInfo } from '../lib/api'

class WorkspacesStore {
  list = $state<WorkspaceInfo[]>([])

  async load(): Promise<void> {
    this.list = await getWorkspaces()
  }
}

export const workspaces = new WorkspacesStore()
