/*
 * Sidebar section collapse + order (GDK-434 / GDK-435).
 *
 * Browser-local only — never the mirror, never the origin. Unknown / new
 * section ids from a later build append after the saved order, in default
 * sequence, so an upgrade that adds a section does not drop the user's order.
 *
 * Hydrate after config.json is loaded (SidebarNav instance init): a
 * constructor read would run at App import, before loadConfig, and write the
 * unscoped key while later saves used the site-partitioned one.
 */

import { STORAGE_KEYS } from '../lib/storage'

/** Default top-to-bottom order of SidebarNav-owned sections. */
const SECTION_IDS = [
  'builtin',
  'jira',
  'personal',
  'team',
  'dashboards',
  'docs',
  'workspaces',
] as const

export type SectionId = (typeof SECTION_IDS)[number]

/** HTML5 DnD type; `text/plain` is set alongside for browsers that drop custom types. */
export const SECTION_DND_TYPE = 'application/x-gadak-section'

const SECTION_ID_SET: ReadonlySet<string> = new Set(SECTION_IDS)

export function isSectionId(value: string): value is SectionId {
  return SECTION_ID_SET.has(value)
}

/**
 * In-flight HTML5 drag (opacity + drop line). Session-only — not persisted
 * with collapsedIds / order. SidebarNav owns the object and passes it into
 * each SidebarSection so siblings share one highlight without the persist store
 * carrying a value that dies on mouseup.
 */
export interface SectionDrag {
  readonly draggingId: SectionId | null
  readonly dropTargetId: SectionId | null
  start(id: SectionId): void
  hover(id: SectionId): void
  clear(): void
}

/**
 * Saved ids first (skip unknown / duplicates), then any default id not yet
 * present — new sections land at the end in catalog order.
 */
function mergeSectionOrder(saved: string[], defaults: readonly SectionId[]): SectionId[] {
  const seen = new Set<SectionId>()
  const out: SectionId[] = []
  for (const id of saved) {
    if (!isSectionId(id) || seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  for (const id of defaults) {
    if (seen.has(id)) continue
    out.push(id)
  }
  return out
}

function loadArray(key: string): string[] {
  try {
    if (typeof localStorage === 'undefined') return []
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

function saveArray(key: string, arr: string[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(arr))
  } catch (e) {
    console.warn(`[sidebar-sections] ${key} save failed`, e)
  }
}

class SidebarSectionsStore {
  collapsedIds = $state<string[]>([])
  order = $state<SectionId[]>([...SECTION_IDS])
  #loaded = false

  hydrate(): void {
    if (this.#loaded) return
    this.#loaded = true
    this.collapsedIds = loadArray(STORAGE_KEYS.sidebarSectionsCollapsed).filter(isSectionId)
    this.order = mergeSectionOrder(loadArray(STORAGE_KEYS.sidebarSectionsOrder), SECTION_IDS)
  }

  isCollapsed(id: SectionId): boolean {
    this.hydrate()
    return this.collapsedIds.includes(id)
  }

  toggle(id: SectionId): void {
    this.hydrate()
    this.collapsedIds = this.collapsedIds.includes(id)
      ? this.collapsedIds.filter((x) => x !== id)
      : [...this.collapsedIds, id]
    saveArray(STORAGE_KEYS.sidebarSectionsCollapsed, this.collapsedIds)
  }

  /**
   * Insert `source` at `target` the same way favorites.reorder does: after
   * target when moving down, before target when moving up.
   */
  reorder(source: SectionId, target: SectionId): void {
    this.hydrate()
    if (source === target) return
    const ordered = [...this.order]
    const sourceIndex = ordered.indexOf(source)
    const targetIndex = ordered.indexOf(target)
    if (sourceIndex < 0 || targetIndex < 0) return

    const [moved] = ordered.splice(sourceIndex, 1)
    const insertAt = ordered.indexOf(target) + (sourceIndex < targetIndex ? 1 : 0)
    ordered.splice(insertAt, 0, moved)
    this.order = ordered
    saveArray(STORAGE_KEYS.sidebarSectionsOrder, ordered)
  }

  /** Move one step among currently visible sections (hidden ones stay put). */
  move(id: SectionId, delta: -1 | 1, visible: readonly SectionId[]): void {
    this.hydrate()
    const from = visible.indexOf(id)
    if (from < 0) return
    const to = from + delta
    if (to < 0 || to >= visible.length) return
    this.reorder(id, visible[to])
  }
}

export const sidebarSections = new SidebarSectionsStore()
