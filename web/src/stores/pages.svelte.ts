/*
 * Mirrored wiki pages (docs) store.
 *
 * Three things live here: the page index behind the sidebar DOCS section and
 * the main-column document views, the selected page for DocumentPanel, and the
 * page hits from the last server search (filters owns issue hits; this owns
 * page hits).
 *
 * The index is sliced four ways — viewed, updated, author, one space — because
 * those are the axes that survive for a single-user local mirror
 * (docs/project/UX_PRINCIPLES.md §6). The tree is one of them, not the entry point.
 *
 * Bodies are never persisted — the index is small and memory-only, and detail
 * is fetched per page and cached for the tab's lifetime. Nothing goes to
 * IndexedDB, so a doc never occupies the issue cache budget.
 */

import * as api from '../lib/api'
import { STORAGE_KEYS } from '../lib/storage'
import type { PageDetail, PageLite } from '../lib/types'
import { column } from './column.svelte'
import { me } from './me.svelte'
import { panel } from './panel.svelte'

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

/** Pages of one author, newest edit first. */
export interface AuthorGroup {
  /** '' when the mirror has no author — the row drops the clause, the header names it. */
  author: string
  pages: PageLite[]
}

/** Group key: author_id when present, else the display name (legacy rows). */
export function pageAuthorGroupKey(p: Pick<PageLite, 'author' | 'author_id'>): string {
  const id = (p.author_id ?? '').trim()
  return id || (p.author ?? '')
}

/** The three axes the document view offers. Parallel tabs, never merged
 *  (UX_PRINCIPLES §6): viewed is your return path, updated is everyone's
 *  activity, author answers "who wrote this". */
export type DocsTab = 'viewed' | 'updated' | 'author'

const DOCS_TABS: DocsTab[] = ['viewed', 'updated', 'author']

/** Last tab survives the tab/session — coming back to Documents lands where the
 *  user left it, the way Outline restores its home tab. */
function loadDocsTab(): DocsTab {
  try {
    const raw = localStorage.getItem(STORAGE_KEYS.docsTab)
    return DOCS_TABS.includes(raw as DocsTab) ? (raw as DocsTab) : 'viewed'
  } catch {
    return 'viewed'
  }
}

/** Millis, or null when the timestamp is missing or unparseable. Offsets differ
 *  between the mirror (Confluence, +09:00) and local visits (Z), so these are
 *  compared as instants, never as strings. */
function millis(iso: string | null | undefined): number | null {
  if (!iso) return null
  const t = Date.parse(iso)
  return Number.isFinite(t) ? t : null
}

/** Newest edit first; a page with no timestamp sorts last rather than to the top. */
function byUpdatedDesc(a: PageLite, b: PageLite): number {
  return (b.updated_at ?? '').localeCompare(a.updated_at ?? '')
}

class PagesStore {
  /** Whole page index (no bodies). Empty until loaded, or when the server has none. */
  index = $state<PageLite[]>([])
  /** Server answered — lets the sidebar tell "no docs" from "not asked yet". */
  loaded = $state(false)
  /** Index request in flight — the docs screens' loading window (GDK-1054). */
  loading = $state(false)
  /**
   * The index request failed. Failure is not emptiness: until this clears,
   * "no documents" copy must not stand in for an unanswered request (GDK-1054).
   */
  loadFailed = $state(false)
  /** Page hits from the last server search. Cleared with the query. */
  searchHits = $state<PageLite[]>([])
  /** The tabbed document view owns the main column instead of the issue list.
   *  A view onto the column union (GDK-821), not a latch of its own: one
   *  value already decides who holds the column. */
  get docsView(): boolean {
    return column.is('docs')
  }

  /** One space's document list owns the main column; the space key, or null. */
  get spaceView(): string | null {
    return column.keyOf('space')
  }

  /**
   * History view owns the main column. Same exclusive layer as docsView /
   * spaceView — all three are the column union now, so a fourth surface
   * (feed, dashboard) cannot hold the column beside them.
   */
  get historyView(): boolean {
    return column.is('history')
  }
  /** Which axis the document view is showing. */
  docsTab = $state<DocsTab>(loadDocsTab())
  /** Author group the By-author tab should scroll to once, set by an arrival
   *  from elsewhere (the person panel). Cleared as soon as it is honoured —
   *  it is a one-shot instruction, not a filter. */
  focusAuthor = $state<string | null>(null)
  /**
   * The label every document screen is narrowed to, or null.
   *
   * One label rather than a set: labels here are subject tags ("runbook",
   * "adr"), and two of them AND-ed is almost always empty, while OR-ing them
   * would need a rule on screen to explain itself. Clicking another label
   * moves the narrowing to it, which is the same gesture reading the same way
   * twice. It combines with the typed filter as AND — the text says what, the
   * label says which kind.
   */
  docsLabel = $state<string | null>(null)
  /** Tree mode on a space screen. Store-level because the URL restores it
   *  (`dview=tree`) and the space screen is remounted per space. */
  spaceTree = $state(false)

  /** key → detail, for this tab's lifetime. */
  #details = new Map<string, PageDetail>()

  /**
   * Bumped by `invalidateDetail` so an open DocumentPanel re-fetches after a
   * desktop browse resync (same role as write.detailNonce for issues).
   */
  detailNonce = $state(0)

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

  /** space key → homepage content id, empty ids dropped. Absent until a sync
   *  has stored spaces.homepage_id (demo.db stays empty on purpose). */
  spaceHomepages = $derived.by(() => {
    const ids = new Map<string, string>()
    for (const p of this.index) {
      const hp = p.space_homepage_id ?? ''
      if (hp && !ids.has(p.space_key)) ids.set(p.space_key, hp)
    }
    return ids
  })

  /** What a space is called on screen: its name, or the key until one arrives. */
  spaceLabel(spaceKey: string): string {
    return this.spaceNames.get(spaceKey) || spaceKey
  }

  /** Homepage content id for a space, or '' when the mirror has not learned it. */
  spaceHomepage(spaceKey: string): string {
    return this.spaceHomepages.get(spaceKey) || ''
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

  /** Whole index, newest edit first — the "Updated" tab. */
  recentlyUpdated = $derived.by<PageLite[]>(() => [...this.index].sort(byUpdatedDesc))

  /** key → page. `parent_id` carries a sibling page's key, so one map resolves
   *  both the tree and any ancestor chain. */
  byKey = $derived(new Map(this.index.map((p) => [p.key, p])))

  /** Documents this browser opened, newest visit first — the "Viewed" tab.
   *  Visits to pages the mirror has since dropped are skipped rather than shown
   *  as blanks. */
  recentlyViewed = $derived.by<PageLite[]>(() => {
    const out: PageLite[] = []
    for (const visit of me.recent) {
      if (visit.kind !== 'doc') continue
      const page = this.byKey.get(visit.key)
      if (page) out.push(page)
    }
    return out
  })

  /** Whole index grouped by author, groups ordered by their newest edit — the
   *  same recency-first rule the flat lists use. */
  byAuthor = $derived.by<AuthorGroup[]>(() => {
    const groups = new Map<string, PageLite[]>()
    for (const p of this.index) {
      const key = pageAuthorGroupKey(p)
      const list = groups.get(key)
      if (list) list.push(p)
      else groups.set(key, [p])
    }
    return [...groups.entries()]
      .map(([, list]) => ({
        author: list[0]?.author ?? '',
        pages: [...list].sort(byUpdatedDesc),
      }))
      .sort((a, b) => byUpdatedDesc(a.pages[0], b.pages[0]))
  })

  /** Documents edited since this browser last opened them. A page never opened
   *  is not unread — otherwise the whole mirror lights up and the mark means
   *  nothing. */
  unread = $derived.by<Set<string>>(() => {
    const out = new Set<string>()
    for (const visit of me.recent) {
      if (visit.kind !== 'doc') continue
      const seen = millis(visit.viewed_at)
      const edited = millis(this.byKey.get(visit.key)?.updated_at)
      if (seen !== null && edited !== null && edited > seen) out.add(visit.key)
    }
    return out
  })

  /** One space's pages, flat and newest edit first — the space screen's default. */
  inSpace(spaceKey: string): PageLite[] {
    return this.index.filter((p) => p.space_key === spaceKey).sort(byUpdatedDesc)
  }

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
    this.docsTab = loadDocsTab()
    this.loading = true
    this.loadFailed = false
    try {
      const res = await api.getPages()
      this.index = res.pages ?? []
      this.loaded = true
    } catch (e) {
      console.warn('[pages] 문서 목록 로드 실패', e)
      this.index = []
      // Failure is not emptiness: `loaded` stays false so the screen shows the
      // error line (not the empty copy) and a later init()/reload() retries.
      this.loadFailed = true
    } finally {
      this.loading = false
    }
  }

  /**
   * Re-read the index after a sync. init() is once-only, so without this the
   * first documents a mirror ever receives stay invisible until a reload —
   * which is exactly the moment someone has just switched Confluence on and is
   * watching to see whether it worked.
   */
  async reload(): Promise<void> {
    this.loading = true
    this.loadFailed = false
    try {
      const res = await api.getPages()
      this.index = res.pages ?? []
      this.loaded = true
      // Index refresh is a new snapshot — drop tab-lifetime bodies so an open
      // DocumentPanel cannot keep a pre-sync ADF tree.
      this.#details.clear()
      this.detailNonce++
    } catch (e) {
      console.warn('[pages] 문서 목록 갱신 실패', e)
      // Keep the stale index (the sidebar keeps its rows); the screens swap
      // the list for the error line until a retry succeeds.
      this.loadFailed = true
    } finally {
      this.loading = false
    }
  }

  /* ── The open page (DocumentPanel) ── */

  /** Which page the right panel is showing, or null when it is showing an issue,
   *  a person, or nothing. Read from the panel union rather than held here: one
   *  detail surface at a time is that value's shape, so opening a page closes an
   *  issue or a person with nothing to clear. */
  #selectedKey = $derived(panel.keyOf('doc'))

  get selectedKey(): string | null {
    return this.#selectedKey
  }

  /** Open a page. The visit joins the same recent list as issues, so the palette
   *  can offer both without a second history to keep. */
  select(key: string): void {
    panel.show('doc', key)
    me.recordRecent(key, 'doc')
  }

  clear(): void {
    panel.close('doc')
  }

  /* ── Main-column document views (tabbed list, or one space) ── */

  /** Any of this store's screens (tabbed docs, one space, history) holds
   *  the main column. The column union makes the membership list structural
   *  — a screen cannot own the column while reading "not open" (GDK-815
   *  was that hole: a getter that forgot a member). */
  get open(): boolean {
    return column.is('docs') || column.is('space') || column.is('history')
  }

  /** The tabbed view takes the column. Deliberately bare: boot `docs=1` and
   *  the URL binding restore an address, and an address has no opinion about
   *  the tab or a narrowing — those are this browser's, kept from the last
   *  visit the way the tab itself is. */
  openDocs(): void {
    column.show({ view: 'docs' })
  }

  /** Sidebar DOCS row. Toggle-off keeps the tab and any label narrowing —
   *  the next open lands where this one left off. */
  toggleDocs(): void {
    const wasOnlyDocs = this.docsView && this.spaceView === null
    if (wasOnlyDocs) column.show({ view: 'list' })
    else column.show({ view: 'docs' })
  }

  /** A space row: its flat document list takes the column from the tabbed view. */
  openSpace(spaceKey: string): void {
    // The flat list is how a page is found (UX_PRINCIPLES §6); arriving at a
    // space is not the request for a hierarchy, so each arrival starts flat —
    // including re-arriving at the space already up (a same-view show would
    // change nothing on its own).
    this.spaceTree = false
    column.show({ view: 'space', key: spaceKey })
  }

  /**
   * Leave this store's screens (docs, one space, history). Call sites that
   * mean "back to the issue list" go through here so a new surface cannot
   * stick the way docs once did. The resets are the leaving-screen's own —
   * they run even when another surface (feed, dashboard) holds the column,
   * because those screens are gone either way once they release it.
   */
  closeDocs(): void {
    this.focusAuthor = null
    this.spaceTree = false
    // The narrowing belongs to the screen that was left, not to the next one.
    this.docsLabel = null
    if (this.open) column.show({ view: 'list' })
  }

  openHistory(): void {
    this.spaceTree = false
    this.docsLabel = null
    this.focusAuthor = null
    column.show({ view: 'history' })
  }

  closeHistory(): void {
    if (this.historyView) column.show({ view: 'list' })
  }

  /** Narrow every document screen to one label, or clear it (null). */
  setDocsLabel(label: string | null): void {
    this.docsLabel = label
  }

  selectTab(tab: DocsTab): void {
    this.docsTab = tab
    this.focusAuthor = null
    try {
      localStorage.setItem(STORAGE_KEYS.docsTab, tab)
    } catch {
      /* private mode — the tab just does not survive the session */
    }
  }

  /** Arrive at the By-author tab already looking at one person's group. The
   *  axis is the tab's own — nothing new is filtered, the list is only scrolled
   *  to where the answer is, since author groups run in recency order and the
   *  one that was asked for can sit well down the page. */
  openDocsByAuthor(author: string): void {
    column.show({ view: 'docs' })
    this.selectTab('author')
    this.focusAuthor = author
  }

  /** How many mirrored pages belong to this author. Matches author_id first,
   *  then the display name so the person panel (which still passes a name)
   *  keeps landing on a group. */
  pagesByAuthorCount(author: string): number {
    if (!author) return 0
    return this.index.reduce((n, p) => {
      if ((p.author_id ?? '') === author || (p.author ?? '') === author) return n + 1
      return n
    }, 0)
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

  /** Drop one page detail and signal open panels to reload. */
  invalidateDetail(key: string): void {
    this.#details.delete(key)
    this.detailNonce++
  }

  setSearchHits(hits: PageLite[]): void {
    this.searchHits = hits
  }

  clearSearchHits(): void {
    if (this.searchHits.length) this.searchHits = []
  }
}

export const pages = new PagesStore()
