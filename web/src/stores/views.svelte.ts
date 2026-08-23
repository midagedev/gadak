/*
 * Issue Navigator — saved views store ([explore])
 *
 * Two layers, one product choice:
 *  - Server api(views/) when a server is behind the app — follows the user
 *  - localStorage when there is no server (hosted demo) or as a fallback
 *
 * config is explore's ViewConfig serialization (opaque JSON). Server does not interpret it.
 *
 * GDK-437: with a server behind the app, the server layer is where a view
 * belongs. Every boot hands leftover localStorage rows (ids not yet in the
 * absorb set) to the server. localStorage is never cleared on success
 * (rollback safety); the absorbed ids only stop the same view listing twice.
 */

import { t } from '../lib/i18n'
import * as api from '../lib/api'
import { isHostedDemo } from '../lib/config'
import { STORAGE_KEYS } from '../lib/storage'
import type { SavedView, SourceView, ViewsResponse } from '../lib/types'
import type { ViewConfig } from '../lib/view-config'

function personalViewsKey(): string {
  return STORAGE_KEYS.personalViews
}

/** Absorb flag: ids already handed to the server. */
function absorbedIdsKey(): string {
  return `${personalViewsKey()}-absorbed`
}

/** null = never written here; otherwise the absorbed id list (may be empty). */
function readAbsorbedIds(): string[] | null {
  try {
    const raw = localStorage.getItem(absorbedIdsKey())
    if (raw === null) return null
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return null
  }
}

function writeAbsorbedIds(ids: string[]): void {
  try {
    localStorage.setItem(absorbedIdsKey(), JSON.stringify(ids))
  } catch {
    /* private mode — leftover rows retry next boot */
  }
}

export interface PersonalView {
  id: string
  name: string
  config: ViewConfig
  created_at: string
}

function loadPersonal(): PersonalView[] {
  try {
    const raw = localStorage.getItem(personalViewsKey())
    if (!raw) return []
    const arr = JSON.parse(raw) as PersonalView[]
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

function savePersonal(views: PersonalView[]): void {
  try {
    localStorage.setItem(personalViewsKey(), JSON.stringify(views))
  } catch (e) {
    console.warn('[views] 개인 뷰 저장 실패', e)
  }
}

class ViewsStore {
  personal = $state<PersonalView[]>([])
  team = $state<SavedView[]>([])
  source = $state<SourceView[]>([])
  teamLoaded = $state(false)

  /** localStorage truth. `personal` is its visible slice: rows already handed
   *  to the server are hidden there, not deleted — a rollback keeps every
   *  row, and the sidebar lists each view once. */
  #raw: PersonalView[] = []
  #absorbed = new Set<string>()

  init(): void {
    this.#raw = loadPersonal()
    this.#absorbed = new Set(readAbsorbedIds() ?? [])
    this.personal = this.#visible()
    void this.loadTeam()
  }

  #visible(): PersonalView[] {
    return this.#raw.filter((v) => !this.#absorbed.has(v.id))
  }

  async loadTeam(): Promise<void> {
    try {
      let res: ViewsResponse = await api.getViews()
      // The hosted demo has no server to write to — never try to absorb there.
      if (!isHostedDemo()) {
        const merged = await this.#absorbPending()
        if (merged) res = merged
      }
      this.team = res.views
      this.source = res.source ?? []
    } catch (e) {
      console.warn('[views] 팀 뷰 로드 실패', e)
    } finally {
      this.teamLoaded = true
    }
  }

  /** Every boot (GDK-437): hand leftover localStorage views (ids not yet in
   *  the absorb set) to the server. A failed POST leaves those ids unrecorded
   *  so the next boot retries. localStorage is never cleared. Returns the
   *  merged document when an absorb happened (the GET above predates it),
   *  null otherwise. */
  async #absorbPending(): Promise<ViewsResponse | null> {
    const pending = this.#raw.filter((v) => !this.#absorbed.has(v.id))
    if (pending.length === 0) return null
    let res: ViewsResponse
    try {
      res = await api.absorbViews(pending)
    } catch {
      return null // server refused/unreachable — retry next boot
    }
    for (const v of pending) this.#absorbed.add(v.id)
    writeAbsorbedIds([...this.#absorbed])
    this.personal = this.#visible()
    return res
  }

  addPersonal(name: string, config: ViewConfig): void {
    const view: PersonalView = {
      id: `p-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`,
      name: name.trim() || t('view.unnamed'),
      config,
      created_at: new Date().toISOString(),
    }
    this.#raw = [...this.#raw, view]
    this.personal = [...this.personal, view]
    savePersonal(this.#raw)
  }

  removePersonal(id: string): void {
    this.#raw = this.#raw.filter((v) => v.id !== id)
    this.personal = this.personal.filter((v) => v.id !== id)
    savePersonal(this.#raw)
  }

  async addTeam(name: string, config: ViewConfig): Promise<void> {
    const view = await api.createView(name.trim() || t('view.unnamed'), config)
    this.team = [...this.team, view]
  }

  async removeTeam(id: string): Promise<void> {
    await api.deleteView(id)
    this.team = this.team.filter((v) => v.id !== id)
  }
}

export const views = new ViewsStore()
