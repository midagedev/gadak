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
import { selection } from './selection.svelte'

/** Pages of one space, for the sidebar's grouped list. */
export interface SpaceGroup {
  space: string
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

  /** key → detail, for this tab's lifetime. */
  #details = new Map<string, PageDetail>()

  /** Pages grouped by space, spaces and titles both alphabetical. */
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
        pages: [...pages].sort((a, b) => a.title.localeCompare(b.title)),
      }))
      .sort((a, b) => a.space.localeCompare(b.space))
  })

  /** key → page. `parent_id` carries a sibling page's key, so one map resolves
   *  both the tree and any ancestor chain. */
  byKey = $derived(new Map(this.index.map((p) => [p.key, p])))

  /** Same grouping as `bySpace`, nested by `parent_id`. Siblings keep the
   *  alphabetical order they already have in `bySpace`. */
  treeBySpace = $derived.by<SpaceTree[]>(() => {
    const byKey = this.byKey
    return this.bySpace.map(({ space, pages: list }) => {
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

      return { space, count: list.length, roots }
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

  /** Open a page. Closing the issue panel keeps one detail surface at a time. */
  select(key: string): void {
    selection.clear()
    this.selectedKey = key
  }

  clear(): void {
    this.selectedKey = null
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
