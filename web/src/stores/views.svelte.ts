/*
 * Issue Navigator — 저장 뷰 스토어 ([explore])
 *
 * 2계층(plan §5.3):
 *  - 개인 뷰: localStorage (즉시, 서버 불필요) — 다른 기기와 공유 안 됨
 *  - 팀 공유 뷰: 서버 api(views/) — 작성자 표시, 본인 소유만 삭제
 *
 * config 는 explore 의 ViewConfig 직렬화(불투명 JSON). 서버는 해석하지 않는다.
 */

import { t } from '../lib/i18n'
import * as api from '../lib/api'
import type { SavedView } from '../lib/types'
import type { ViewConfig } from '../lib/view-config'

const LS_KEY = 'issue-nav:personal-views'

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
  teamLoaded = $state(false)

  init(): void {
    this.personal = loadPersonal()
    void this.loadTeam()
  }

  async loadTeam(): Promise<void> {
    try {
      const res = await api.getViews()
      this.team = res.views
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
    const view = await api.createView(
      name.trim() || t('view.unnamed'),
      config as unknown as Record<string, unknown>,
    )
    this.team = [...this.team, view]
  }

  async removeTeam(id: string): Promise<void> {
    await api.deleteView(id)
    this.team = this.team.filter((v) => v.id !== id)
  }
}

export const views = new ViewsStore()
