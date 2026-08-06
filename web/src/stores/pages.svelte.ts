/*
 * Mirrored wiki pages (docs) store.
 *
 * Three things live here: the page index for the sidebar DOCS section, the
 * selected page for DocumentPanel, and the page hits from the last server
 * search (filters owns issue hits; this owns page hits).
 *
 * Bodies are never persisted — the index is small and memory-only, and detail
 * is fetched per page and cached for the tab's lifetime. Nothing goes to
 * IndexedDB, so a doc never occupies the issue cache budget.
 */

import * as api from '../lib/api'
import type { PageDetail, PageLite } from '../lib/types'
import { me } from './me.svelte'
import { selection } from './selection.svelte'

/** Pages of one space, for the sidebar's grouped list. Grouping is by key; the
 *  name only ever decides what the row reads. */
export interface SpaceGroup {
  space: string
  /** Human-readable space name, '' when the mirror has not learned it. */
  name: string
  pages: PageLite[]
}

/** One page and the pages filed under it. `depth` saves the sidebar recursion
 *  from threading an indent level through the snippet. */
export interface PageNode {
  page: PageLite
  depth: number
  children: PageNode[]
}

/** One space's pages as a tree, plus the flat total for the sidebar badge. */
export interface SpaceTree {
  space: string
  name: string
  count: number
  roots: PageNode[]
}

class PagesStore {
  /** Whole page index (no bodies). Empty until loaded, or when the server has none. */
  index = $state<PageLite[]>([])
  /** Server answered — lets the sidebar tell "no docs" from "not asked yet". */
  loaded = $state(false)
  /** Open page (DocumentPanel). Mutually exclusive with an open issue. */
  selectedKey = $state<string | null>(null)
  /** Page hits from the last server search. Cleared with the query. */
  searchHits = $state<PageLite[]>([])
  /** "Recently updated" is open in the main column instead of the issue list. */
  recentView = $state(false)

  /** key → detail, for this tab's lifetime. */
  #details = new Map<string, PageDetail>()

  /** space key → name, empty names dropped. A space the mirror has not learned
   *  a name for is absent here, which is what `spaceLabel` falls back on. */
  spaceNames = $derived.by(() => {
    const names = new Map<string, string>()
    for (const p of this.index) {
      const name = p.space_name ?? ''
      if (name && !names.has(p.space_key)) names.set(p.space_key, name)
    }
    return names
  })

  /** What a space is called on screen: its name, or the key until one arrives. */
  spaceLabel(spaceKey: string): string {
    return this.spaceNames.get(spaceKey) || spaceKey
  }

  /** Pages grouped by space, spaces (by display name) and titles alphabetical. */
  bySpace = $derived.by<SpaceGroup[]>(() => {
    const groups = new Map<string, PageLite[]>()
    for (const p of this.index) {
      const list = groups.get(p.space_key)
      if (list) list.push(p)
      else groups.set(p.space_key, [p])
    }
    return [...groups.entries()]
      .map(([space, pages]) => ({
        space,
        name: this.spaceNames.get(space) ?? '',
        pages: [...pages].sort((a, b) => a.title.localeCompare(b.title)),
      }))
      .sort((a, b) => (a.name || a.space).localeCompare(b.name || b.space))
  })

  /** Whole index, newest edit first — the "Recently updated" view. Pages with no
   *  timestamp sort last rather than to the top. */
  recentlyUpdated = $derived.by<PageLite[]>(() =>
    [...this.index].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '')),
  )

  /** key → page. `parent_id` carries a sibling page's key, so one map resolves
   *  both the tree and any ancestor chain. */
  byKey = $derived(new Map(this.index.map((p) => [p.key, p])))

  /** Same grouping as `bySpace`, nested by `parent_id`. Siblings keep the
   *  alphabetical order they already have in `bySpace`. */
  treeBySpace = $derived.by<SpaceTree[]>(() => {
    const byKey = this.byKey
    return this.bySpace.map(({ space, name, pages: list }) => {
      // A page whose parent is missing from the mirror (never synced, or in
      // another space) hangs at the top instead of disappearing.
      const children = new Map<string, PageLite[]>()
      const rootPages: PageLite[] = []
      for (const p of list) {
        const parentKey = p.parent_id ?? ''
        const parent = parentKey ? byKey.get(parentKey) : undefined
        if (!parent || parent.space_key !== space) {
          rootPages.push(p)
          continue
        }
        const kids = children.get(parentKey)
        if (kids) kids.push(p)
        else children.set(parentKey, [p])
      }

      // `visited` both bounds the recursion on a parent cycle (a → b → a) and
      // marks what a root can reach.
      const visited = new Set<string>()
      const build = (page: PageLite, depth: number): PageNode => {
        visited.add(page.key)
        const kids = (children.get(page.key) ?? []).filter((c) => !visited.has(c.key))
        return { page, depth, children: kids.map((c) => build(c, depth + 1)) }
      }
      const roots = rootPages.map((p) => build(p, 0))
      // Pages inside a cycle are reachable from no root — surface them rather
      // than drop them from the nav.
      for (const p of list) if (!visited.has(p.key)) roots.push(build(p, 0))

      return { space, name, count: list.length, roots }
    })
  })

  /** Ancestor chain of a page, outermost first — the breadcrumb between the
   *  space and the page itself. Empty for a root, or for a page the index
   *  never loaded. */
  ancestors(key: string): PageLite[] {
    const chain: PageLite[] = []
    const seen = new Set<string>([key])
    let cur = this.byKey.get(key)
    while (cur?.parent_id) {
      const parent = this.byKey.get(cur.parent_id)
      if (!parent || seen.has(parent.key)) break
      seen.add(parent.key)
      chain.unshift(parent)
      cur = parent
    }
    return chain
  }

  /** Load the index once. A server without the endpoint (404) just leaves it empty. */
  async init(): Promise<void> {
    if (this.loaded) return
    try {
      const res = await api.getPages()
      this.index = res.pages ?? []
    } catch (e) {
      console.warn('[pages] 문서 목록 로드 실패', e)
      this.index = []
    } finally {
      this.loaded = true
    }
  }

  /** Open a page. Closing the issue panel keeps one detail surface at a time.
   *  The visit joins the same recent list as issues, so the palette can offer
   *  both without a second history to keep. */
  select(key: string): void {
    selection.clear()
    this.selectedKey = key
    me.recordRecent(key, 'doc')
  }

  clear(): void {
    this.selectedKey = null
  }

  /* ── "Recently updated" main-column view ── */

  toggleRecent(): void {
    this.recentView = !this.recentView
  }

  closeRecent(): void {
    this.recentView = false
  }

  /** Row data for the open page — header renders before the body arrives. */
  lite(key: string): PageLite | undefined {
    return this.index.find((p) => p.key === key) ?? this.#details.get(key)
  }

  /** Cached detail, or one fetch. Throws api.ApiError like the issue detail cache. */
  async detail(key: string): Promise<PageDetail> {
    const hit = this.#details.get(key)
    if (hit) return hit
    const d = await api.getPageDetail(key)
    this.#details.set(key, d)
    return d
  }

  setSearchHits(hits: PageLite[]): void {
    this.searchHits = hits
  }

  clearSearchHits(): void {
    if (this.searchHits.length) this.searchHits = []
  }
}

export const pages = new PagesStore()
