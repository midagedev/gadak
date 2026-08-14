/*
 * Issue Navigator — saved views store ([explore])
 *
 * Two layers (plan §5.3):
 *  - Personal views: localStorage (instant, no server) — not shared across devices
 *  - Team-shared views: server api(views/) — shows author; only owner can delete
 *
 * config is explore's ViewConfig serialization (opaque JSON). Server does not interpret it.
 */

import { t } from '../lib/i18n'
import * as api from '../lib/api'
import { STORAGE_KEYS } from '../lib/storage'
import type { SavedView, SourceView } from '../lib/types'
import type { ViewConfig } from '../lib/view-config'

const LS_KEY = STORAGE_KEYS.personalViews

export interface PersonalView {
  id: string
  name: string
  config: ViewConfig
  created_at: string
}

function loadPersonal(): PersonalView[] {
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (!raw) return []
    const arr = JSON.parse(raw) as PersonalView[]
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

function savePersonal(views: PersonalView[]): void {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(views))
  } catch (e) {
    console.warn('[views] 개인 뷰 저장 실패', e)
  }
}

class ViewsStore {
  personal = $state<PersonalView[]>([])
  team = $state<SavedView[]>([])
  source = $state<SourceView[]>([])
  teamLoaded = $state(false)

  init(): void {
    this.personal = loadPersonal()
    void this.loadTeam()
  }

  async loadTeam(): Promise<void> {
    try {
      const res = await api.getViews()
      this.team = res.views
      this.source = res.source ?? []
    } catch (e) {
      console.warn('[views] 팀 뷰 로드 실패', e)
    } finally {
      this.teamLoaded = true
    }
  }

  addPersonal(name: string, config: ViewConfig): void {
    const view: PersonalView = {
      id: `p-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`,
      name: name.trim() || t('view.unnamed'),
      config,
      created_at: new Date().toISOString(),
    }
    this.personal = [...this.personal, view]
    savePersonal(this.personal)
  }

  removePersonal(id: string): void {
    this.personal = this.personal.filter((v) => v.id !== id)
    savePersonal(this.personal)
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
