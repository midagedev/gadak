/*
 * The in-app browser pane: tabs, the rectangle they occupy, and the resync that
 * follows one closing.
 *
 * Atlassian forbids iframes, so an original page can only be shown by a native
 * WKWebView the desktop app layers over this document. That view has no chrome
 * and no idea what a tab is — everything visual belongs to the SPA, and this
 * module is where that side of it lives:
 *
 *   - which tabs exist, which one is showing, whether the pane is on screen
 *   - the rectangle the native views must occupy, in this document's coordinates
 *   - the targeted resync that runs when a tab closes or the window regains
 *     focus, so an issue edited in Jira is current again by the time the pane
 *     gets out of the way
 *
 * A native view draws over every SPA pixel inside its rectangle, which makes
 * "what is visible" a decision only this side can make. `resolveBrowseStack`
 * is the single owner: an open palette / dialog / media viewer hides the
 * native view *and* yields the SPA chrome below the dialog tier; a toast
 * leaves the page up and shrinks the reported rectangle so the toast host
 * stays visible. `nativeActive` is what that decision POSTs to
 * /desktop/browse/activate.
 *
 * Installed only in desktop mode; browser `gadak serve` never reaches any of it
 * (`adopt` is the single entry point and returns immediately off desktop).
 */

import * as api from './api'
import * as db from './db'
import { isDesktop } from './config'
import { invalidate } from './detail-cache.svelte'
import { issues } from '../stores/issues.svelte'
import { me } from '../stores/me.svelte'
import { pages } from '../stores/pages.svelte'
import { write } from '../stores/write.svelte'
import type { IssueLite } from './types'
import { resolveBrowseStack, type BrowseStack } from './browse-stack'

/** Matches classifyAtlassianLink kind — kept local to avoid a cycle with desktop-links. */
export type BrowseKind = 'issue' | 'page' | 'other'

/** One native tab, as the tab strip renders it. Title and URL are live: the
 *  page can navigate itself, and the poll below is how we hear about it. */
export interface BrowseTab {
  id: string
  title: string
  url: string
}

/** The pane rectangle in this document's viewport, CSS px, y from the top. */
export interface FrameRect {
  x: number
  y: number
  w: number
  h: number
}

interface BrowseSession {
  kind: BrowseKind
  key: string | null
}

/**
 * Fast enough that ⌘W (a native menu item the SPA never sees) reads as
 * immediate, cheap enough to leave running: it is one local request, and only
 * while a tab is open.
 */
const POLL_MS = 1000
const FOCUS_THROTTLE_MS = 15_000
/** Frame reports coalesce over a drag; the native side only needs the last one. */
const FRAME_DEBOUNCE_MS = 50

function post(path: string, body: unknown): void {
  void fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).catch(() => {
    /* the app is the only server here; a failed local POST has no recovery */
  })
}

class BrowseStore {
  /** Open tabs in strip order. */
  tabs = $state<BrowseTab[]>([])
  /** Which tab the pane shows. Survives the pane closing — the tabs do too. */
  activeId = $state('')
  /** Whether the pane occupies the detail area. */
  paneOpen = $state(false)

  #dialogOpen = $state(false)
  #toastVisible = $state(false)

  /** id → what was opened there, for the resync when it closes. */
  #sessions = new Map<string, BrowseSession>()
  /** `issue:NMB-1` / `page:123` → last resync, for the focus throttle. */
  #lastResyncAt = new Map<string, number>()

  #pollTimer: ReturnType<typeof setInterval> | null = null
  /** Bumped on every local tab mutation so a poll that raced it is discarded. */
  #gen = 0

  #lastSentActive: string | null = null
  #frameTimer: ReturnType<typeof setTimeout> | null = null
  #pendingFrame: FrameRect | null = null
  #lastSentFrame: FrameRect | null = null

  /** True only inside the desktop app. Nothing here renders or installs off it. */
  get enabled(): boolean {
    return isDesktop()
  }

  get activeTab(): BrowseTab | null {
    return this.tabs.find((t) => t.id === this.activeId) ?? null
  }

  /**
   * The re-entry pill is on screen: tabs exist but the pane is put away. The
   * issue list reserves scroll room for it — the pill floats over the list's
   * bottom-left corner, and the last row's checkbox must stay reachable.
   */
  get pillVisible(): boolean {
    return this.tabs.length > 0 && !this.paneOpen
  }

  /**
   * The stacking decision for the current pane / dialog / toast facts.
   * Inspectable from the shell (`data-browse-*` on <html>) and from
   * `browse.stack` in a debugger.
   */
  stack: BrowseStack = $derived.by(() =>
    resolveBrowseStack({
      paneOpen: this.paneOpen,
      dialogOpen: this.#dialogOpen,
      toastVisible: this.#toastVisible,
    }),
  )

  /**
   * The tab the native layer should be showing — "" for none. The pane being
   * off screen and an SPA overlay being up are the same answer for the same
   * reason: a native view would be covering something it must not cover.
   */
  get nativeActive(): string {
    if (!this.stack.nativeVisible) return ''
    return this.tabs.some((t) => t.id === this.activeId) ? this.activeId : ''
  }

  /**
   * Set by the shell whenever a full-surface SPA overlay or the toast host
   * changes. This is the only writer of the stacking inputs.
   */
  setSurface(flags: { dialogOpen: boolean; toastVisible: boolean }): void {
    if (this.#dialogOpen === flags.dialogOpen && this.#toastVisible === flags.toastVisible) {
      return
    }
    this.#dialogOpen = flags.dialogOpen
    this.#toastVisible = flags.toastVisible
    this.#syncNative()
  }

  /**
   * Remember a tab the app just created (POST /desktop/browse answered 201) and
   * bring the pane forward. Creation activates the tab natively, so the sync
   * below is usually a no-op — unless an overlay is up, in which case it is the
   * hide that stops the new page drawing over it.
   */
  adopt(id: string, url: string, kind: BrowseKind, key: string | null): void {
    if (!this.enabled) return
    this.#gen++
    this.#sessions.set(id, { kind, key })
    if (key && (kind === 'issue' || kind === 'page')) {
      me.recordRecent(key, kind === 'page' ? 'doc' : 'issue')
    }
    if (!this.tabs.some((t) => t.id === id)) {
      this.tabs = [...this.tabs, { id, title: '', url }]
    }
    this.activeId = id
    this.paneOpen = true
    this.#lastSentActive = id
    this.#syncNative()
    this.#startPoll()
  }

  /** Tab strip click. */
  activate(id: string): void {
    if (this.activeId === id && this.paneOpen) return
    this.activeId = id
    this.paneOpen = true
    this.#syncNative()
  }

  /**
   * Put the pane away without touching the tabs — they are still open, and the
   * indicator brings them back. The tab being left is resynced now rather than
   * on the focus throttle: this is the moment the person asked to see Gadak's
   * copy of what they were just looking at.
   */
  hidePane(): void {
    if (!this.paneOpen) return
    this.paneOpen = false
    this.#syncNative()
    const sess = this.#sessions.get(this.activeId)
    if (sess) void this.#resync(sess)
  }

  /** The re-entry affordance. Noop with nothing to come back to. */
  showPane(): void {
    if (this.tabs.length === 0) return
    if (!this.tabs.some((t) => t.id === this.activeId)) {
      this.activeId = this.tabs[this.tabs.length - 1].id
    }
    this.paneOpen = true
    this.#syncNative()
  }

  /** Close one tab: locally first so the strip reacts to the click, then the
   *  native teardown and the resync for what was open there. */
  closeTab(id: string): void {
    const idx = this.tabs.findIndex((t) => t.id === id)
    if (idx < 0) return
    this.#gen++
    const sess = this.#sessions.get(id)
    this.#sessions.delete(id)
    this.tabs = this.tabs.filter((t) => t.id !== id)
    if (this.activeId === id) {
      this.activeId = this.tabs[idx]?.id ?? this.tabs[idx - 1]?.id ?? ''
    }
    if (this.tabs.length === 0) {
      this.paneOpen = false
      this.#stopPoll()
    }
    this.#syncNative()
    post('/desktop/browse/close', { id })
    if (sess) void this.#resync(sess)
  }

  /** Current tab in the system browser — the escape hatch out of the pane. */
  openActiveExternally(): void {
    const url = this.activeTab?.url
    if (url) post('/desktop/open', { url })
  }

  /**
   * Report where the pane's content box is. Called on mount, on resize and on
   * every poll tick — a layout change that moves the box without resizing it
   * (the detail panel opening beneath) fires no ResizeObserver, and the native
   * view has no way to notice on its own.
   */
  reportFrame(rect: FrameRect): void {
    if (!this.enabled) return
    const r = {
      x: Math.round(rect.x),
      y: Math.round(rect.y),
      w: Math.round(rect.w),
      h: Math.round(rect.h),
    }
    if (r.w <= 0 || r.h <= 0) return
    this.#pendingFrame = r
    if (this.#frameTimer !== null) return
    this.#frameTimer = setTimeout(() => {
      this.#frameTimer = null
      const f = this.#pendingFrame
      this.#pendingFrame = null
      if (!f) return
      const last = this.#lastSentFrame
      if (last && last.x === f.x && last.y === f.y && last.w === f.w && last.h === f.h) return
      this.#lastSentFrame = f
      post('/desktop/browse/frame', f)
    }, FRAME_DEBOUNCE_MS)
  }

  // ── native activation ──

  #syncNative(): void {
    if (!this.enabled) return
    const want = this.nativeActive
    if (want === this.#lastSentActive) return
    this.#lastSentActive = want
    post('/desktop/browse/activate', { id: want })
  }

  // ── polling ──

  #startPoll(): void {
    if (this.#pollTimer !== null) return
    this.#pollTimer = setInterval(() => {
      void this.#poll()
    }, POLL_MS)
  }

  #stopPoll(): void {
    if (this.#pollTimer === null) return
    clearInterval(this.#pollTimer)
    this.#pollTimer = null
  }

  /**
   * Reconcile with the native side. Two things only it knows: the live title and
   * URL of each page, and that ⌘W closed a tab — the app's Close Tab menu item
   * never reaches this document.
   */
  async #poll(): Promise<void> {
    if (this.tabs.length === 0) {
      this.#stopPoll()
      return
    }
    const gen = this.#gen
    let body: { open?: BrowseTab[]; active?: string }
    try {
      const res = await fetch('/desktop/browse/state')
      if (!res.ok) return
      body = (await res.json()) as { open?: BrowseTab[]; active?: string }
    } catch {
      return
    }
    // A tab opened or closed while this was in flight: that answer predates the
    // change, and applying it would resurrect or drop the wrong tab.
    if (gen !== this.#gen) return

    const open = body.open ?? []
    const live = new Set(open.map((t) => t.id))
    const closed: BrowseSession[] = []
    let closedIdx = -1
    this.tabs.forEach((t, i) => {
      if (live.has(t.id)) return
      if (closedIdx < 0) closedIdx = i
      const sess = this.#sessions.get(t.id)
      this.#sessions.delete(t.id)
      if (sess) closed.push(sess)
    })

    this.tabs = open.map((t) => ({ id: t.id, title: t.title ?? '', url: t.url ?? '' }))

    if (!live.has(this.activeId)) {
      this.activeId = this.tabs[closedIdx]?.id ?? this.tabs[this.tabs.length - 1]?.id ?? ''
    }
    if (this.tabs.length === 0) {
      this.paneOpen = false
      this.#stopPoll()
    }
    this.#syncNative()
    for (const sess of closed) void this.#resync(sess)
  }

  // ── resync ──

  #throttleKey(sess: BrowseSession): string | null {
    if (!sess.key || sess.kind === 'other') return null
    return `${sess.kind}:${sess.key}`
  }

  /**
   * Apply a write-shaped issue resync the same way write.svelte does after a
   * successful comment/transition: pool + IndexedDB, then detail cache miss +
   * detailNonce so an open DetailPanel reloads.
   */
  #applyIssue(issue: IssueLite | null | undefined): void {
    if (!issue || !issue.issue_key) return
    issues.pool.set(issue.issue_key, issue)
    void db.putIssues([issue]).catch(() => {})
    invalidate(issue.issue_key)
    write.bumpDetail()
  }

  async #resync(sess: BrowseSession, opts?: { throttle?: boolean }): Promise<void> {
    if (sess.kind === 'other' || !sess.key) return

    const tKey = this.#throttleKey(sess)
    if (opts?.throttle && tKey) {
      const last = this.#lastResyncAt.get(tKey) ?? 0
      if (Date.now() - last < FOCUS_THROTTLE_MS) return
    }

    try {
      if (sess.kind === 'issue') {
        const res = await api.resyncIssue(sess.key)
        this.#applyIssue(res.issue)
      } else if (sess.kind === 'page') {
        await api.resyncPage(sess.key)
        pages.invalidateDetail(sess.key)
      }
      if (tKey) this.#lastResyncAt.set(tKey, Date.now())
    } catch (e) {
      console.warn('[browse] resync failed', sess.kind, sess.key, e)
    }
  }

  /** Coming back to the window is the other moment Jira may have moved on.
   *  One resync per item, however many tabs point at it. */
  onWindowFocus = (): void => {
    if (this.#sessions.size === 0) return
    const seen = new Set<string>()
    for (const sess of this.#sessions.values()) {
      const tKey = this.#throttleKey(sess)
      if (tKey) {
        if (seen.has(tKey)) continue
        seen.add(tKey)
      }
      void this.#resync(sess, { throttle: true })
    }
  }

  /** Teardown for the shell's onMount cleanup. */
  reset(): void {
    this.#stopPoll()
    if (this.#frameTimer !== null) clearTimeout(this.#frameTimer)
    this.#frameTimer = null
    this.#pendingFrame = null
    this.#lastSentFrame = null
    this.#lastSentActive = null
    this.#sessions.clear()
    this.#lastResyncAt.clear()
    this.tabs = []
    this.activeId = ''
    this.paneOpen = false
    this.#dialogOpen = false
    this.#toastVisible = false
  }
}

export const browse = new BrowseStore()

/**
 * What a tab is called before its page has answered with a title: the issue key
 * or page id if the URL carries one, the host otherwise. A blank tab is worse
 * than an approximate one — the strip has to be readable the instant it appears.
 */
export function tabLabel(tab: BrowseTab): string {
  if (tab.title.trim()) return tab.title.trim()
  try {
    const u = new URL(tab.url)
    const browsePath = u.pathname.match(/\/browse\/([A-Z][A-Z0-9]*-\d+)/)
    if (browsePath) return browsePath[1]
    const wiki = u.pathname.match(/\/wiki\/spaces\/([^/]+)\/pages\/(\d+)/)
    if (wiki) return `${wiki[1]} / ${wiki[2]}`
    return u.host
  } catch {
    return tab.url
  }
}

/** Install the focus listener. Noop off desktop; returns the uninstall. */
export function installBrowseSessions(): () => void {
  if (!isDesktop()) return () => {}
  window.addEventListener('focus', browse.onWindowFocus)
  return () => {
    window.removeEventListener('focus', browse.onWindowFocus)
    browse.reset()
  }
}
