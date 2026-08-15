/*
 * One global key handler for the shell.
 *
 * The list used to run its own window listener, and two listeners racing is
 * how Esc closed the detail *and* the selection. Resolution lives here so
 * App.svelte can keep exactly one `<svelte:window onkeydown>`. Shell-local
 * latches (palette, shortcuts, settings) are passed in; store objects are
 * passed in too, so this file stays free of store imports and the resolver
 * can be unit-tested in node.
 *
 * Keep ShortcutsDialog in sync — document only keys that have handlers.
 */

export const NARROW_FIELD_TESTID = {
  history: 'history-filter-input',
  docs: 'docs-filter-input',
  issues: 'search-input',
} as const

export const DETAIL_TESTID = {
  status: 'status-transition',
  assignee: 'assignee-picker',
  labelInput: 'label-editor-input',
  labelAdd: 'label-editor-add',
  comment: 'comment-composer',
} as const

export type TriageMenuKey = 'status' | 'assignee' | 'labels'

export interface KeyContext {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  inEditable: boolean
  settingsOpen: boolean
  newIssueOpen: boolean
  serverSettingsOpen: boolean
  paletteOpen: boolean
  commentOpen: boolean
  shortcutsOpen: boolean
  feedBlocksNarrow: boolean
  historyView: boolean
  docsOpen: boolean
  listActive: boolean
  cursorKey: string | null
  detailOpen: boolean
  browsePaneOpen: boolean
  triageMenuOpen: boolean
  bulkActive: boolean
  pageSelected: boolean
  personSelected: boolean
}

export type KeyCommand =
  | { type: 'ignore' }
  | { type: 'toggle-palette' }
  | { type: 'close-shortcuts' }
  | { type: 'open-shortcuts' }
  | { type: 'focus-narrow'; testid: string | null }
  | { type: 'move-list'; dir: 1 | -1 }
  | { type: 'open-cursor' }
  | { type: 'hide-browse' }
  | { type: 'clear-bulk' }
  | { type: 'clear-selection' }
  | { type: 'clear-page' }
  | { type: 'clear-person' }
  | { type: 'toggle-bulk-cursor' }
  | { type: 'request-menu'; menu: TriageMenuKey }
  | { type: 'activate-labels' }
  | { type: 'click-detail'; testid: string }
  | { type: 'focus-comment' }
  | { type: 'open-comment-cursor' }
  | { type: 'new-issue' }

export function keyContext(over: Partial<KeyContext> = {}): KeyContext {
  return {
    key: '',
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    inEditable: false,
    settingsOpen: false,
    newIssueOpen: false,
    serverSettingsOpen: false,
    paletteOpen: false,
    commentOpen: false,
    shortcutsOpen: false,
    feedBlocksNarrow: false,
    historyView: false,
    docsOpen: false,
    listActive: false,
    cursorKey: null,
    detailOpen: false,
    browsePaneOpen: false,
    triageMenuOpen: false,
    bulkActive: false,
    pageSelected: false,
    personSelected: false,
    ...over,
  }
}

/** Which main-column field `/` should focus, or null when the feed owns the column. */
export function narrowFieldTestId(ctx: {
  feedBlocksNarrow: boolean
  historyView: boolean
  docsOpen: boolean
}): string | null {
  if (ctx.feedBlocksNarrow) return null
  if (ctx.historyView) return NARROW_FIELD_TESTID.history
  if (ctx.docsOpen) return NARROW_FIELD_TESTID.docs
  return NARROW_FIELD_TESTID.issues
}

export function isEditableTarget(el: EventTarget | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
}

/**
 * Decide what a key means. DOM (does the field exist?) is the handler's job —
 * `/` still returns `focus-narrow` when the testid is known, and the handler
 * only preventDefault-s if the node is on the page.
 */
export function resolveGlobalKey(ctx: KeyContext): KeyCommand {
  if ((ctx.metaKey || ctx.ctrlKey) && ctx.key.toLowerCase() === 'k') {
    return { type: 'toggle-palette' }
  }
  if (ctx.metaKey || ctx.ctrlKey || ctx.altKey) return { type: 'ignore' }
  if (ctx.inEditable) return { type: 'ignore' }
  if (
    ctx.settingsOpen ||
    ctx.newIssueOpen ||
    ctx.serverSettingsOpen ||
    ctx.paletteOpen ||
    ctx.commentOpen
  ) {
    return { type: 'ignore' }
  }
  if (ctx.shortcutsOpen) {
    if (ctx.key === '?') return { type: 'close-shortcuts' }
    return { type: 'ignore' }
  }

  const key = ctx.key
  const cursorKey = ctx.listActive ? ctx.cursorKey : null

  if (key === '?') return { type: 'open-shortcuts' }

  if (key === '/') {
    return {
      type: 'focus-narrow',
      testid: narrowFieldTestId(ctx),
    }
  }

  if (ctx.listActive && (key === 'j' || key === 'k')) {
    return { type: 'move-list', dir: key === 'j' ? 1 : -1 }
  }
  if (key === 'Enter' && cursorKey) return { type: 'open-cursor' }

  if (key === 'Escape') {
    if (ctx.browsePaneOpen) return { type: 'hide-browse' }
    if (ctx.triageMenuOpen) return { type: 'ignore' }
    if (ctx.bulkActive) return { type: 'clear-bulk' }
    if (ctx.detailOpen) return { type: 'clear-selection' }
    return { type: 'ignore' }
  }

  if (key === 'x') {
    if (ctx.pageSelected) return { type: 'clear-page' }
    if (ctx.personSelected) return { type: 'clear-person' }
    if (cursorKey) return { type: 'toggle-bulk-cursor' }
    if (ctx.detailOpen) return { type: 'clear-selection' }
    return { type: 'ignore' }
  }

  if (key === 's' || key === 'a' || key === 'l') {
    const menu: TriageMenuKey = key === 's' ? 'status' : key === 'a' ? 'assignee' : 'labels'
    if (ctx.bulkActive || (!ctx.detailOpen && cursorKey)) {
      return { type: 'request-menu', menu }
    }
    if (ctx.detailOpen) {
      if (key === 'l') return { type: 'activate-labels' }
      return {
        type: 'click-detail',
        testid: key === 's' ? DETAIL_TESTID.status : DETAIL_TESTID.assignee,
      }
    }
    return { type: 'ignore' }
  }

  if (key === 'c') {
    if (ctx.detailOpen) return { type: 'focus-comment' }
    if (cursorKey) return { type: 'open-comment-cursor' }
    return { type: 'new-issue' }
  }

  return { type: 'ignore' }
}

export interface GlobalKeyHost {
  get paletteOpen(): boolean
  set paletteOpen(v: boolean)
  get shortcutsOpen(): boolean
  set shortcutsOpen(v: boolean)
  get serverSettingsOpen(): boolean
  write: { settingsOpen: boolean; newIssueOpen: boolean; openNewIssue: () => void }
  triage: {
    commentKey: string | null
    listActive: boolean
    cursorKey: string | null
    menu: string | null
    move: (dir: 1 | -1) => void
    requestMenu: (menu: TriageMenuKey) => unknown
    openComment: (key: string) => void
  }
  selection: { selectedKey: string | null; select: (key: string) => void; clear: () => void }
  pages: { historyView: boolean; open: boolean; selectedKey: string | null; clear: () => void }
  person: { selectedEmail: string | null; clear: () => void }
  bulk: { active: boolean; clear: () => void; toggle: (key: string) => void }
  browse: { paneOpen: boolean; hidePane: () => void }
  me: { feedOpen: boolean }
  feature: (name: 'feed') => boolean
}

function contextFromEvent(e: KeyboardEvent, host: GlobalKeyHost): KeyContext {
  return {
    key: e.key,
    metaKey: e.metaKey,
    ctrlKey: e.ctrlKey,
    altKey: e.altKey,
    inEditable: isEditableTarget(e.target),
    settingsOpen: host.write.settingsOpen,
    newIssueOpen: host.write.newIssueOpen,
    serverSettingsOpen: host.serverSettingsOpen,
    paletteOpen: host.paletteOpen,
    commentOpen: Boolean(host.triage.commentKey),
    shortcutsOpen: host.shortcutsOpen,
    feedBlocksNarrow: host.me.feedOpen && host.feature('feed'),
    historyView: host.pages.historyView,
    docsOpen: host.pages.open,
    listActive: host.triage.listActive,
    cursorKey: host.triage.cursorKey,
    detailOpen: host.selection.selectedKey !== null,
    browsePaneOpen: host.browse.paneOpen,
    triageMenuOpen: Boolean(host.triage.menu),
    bulkActive: host.bulk.active,
    pageSelected: Boolean(host.pages.selectedKey),
    personSelected: Boolean(host.person.selectedEmail),
  }
}

function dispatchKeyCommand(e: KeyboardEvent, cmd: KeyCommand, host: GlobalKeyHost): void {
  const cursorKey = host.triage.listActive ? host.triage.cursorKey : null

  switch (cmd.type) {
    case 'ignore':
      return
    case 'toggle-palette':
      e.preventDefault()
      host.paletteOpen = !host.paletteOpen
      return
    case 'close-shortcuts':
      e.preventDefault()
      host.shortcutsOpen = false
      return
    case 'open-shortcuts':
      e.preventDefault()
      host.shortcutsOpen = true
      return
    case 'focus-narrow': {
      if (!cmd.testid) return
      const field = document.querySelector<HTMLInputElement>(`[data-testid="${cmd.testid}"]`)
      if (field) {
        e.preventDefault()
        field.focus()
      }
      return
    }
    case 'move-list':
      e.preventDefault()
      host.triage.move(cmd.dir)
      return
    case 'open-cursor':
      if (!cursorKey) return
      e.preventDefault()
      host.selection.select(cursorKey)
      return
    case 'hide-browse':
      e.preventDefault()
      host.browse.hidePane()
      return
    case 'clear-bulk':
      e.preventDefault()
      host.bulk.clear()
      return
    case 'clear-selection':
      e.preventDefault()
      host.selection.clear()
      return
    case 'clear-page':
      e.preventDefault()
      host.pages.clear()
      return
    case 'clear-person':
      e.preventDefault()
      host.person.clear()
      return
    case 'toggle-bulk-cursor':
      if (!cursorKey) return
      e.preventDefault()
      host.bulk.toggle(cursorKey)
      return
    case 'request-menu':
      e.preventDefault()
      host.triage.requestMenu(cmd.menu)
      return
    case 'activate-labels': {
      e.preventDefault()
      const field = document.querySelector<HTMLInputElement>(
        `[data-testid="${DETAIL_TESTID.labelInput}"]`,
      )
      if (field) field.focus()
      else
        document
          .querySelector<HTMLButtonElement>(`[data-testid="${DETAIL_TESTID.labelAdd}"]`)
          ?.click()
      return
    }
    case 'click-detail':
      e.preventDefault()
      document.querySelector<HTMLButtonElement>(`[data-testid="${cmd.testid}"]`)?.click()
      return
    case 'focus-comment':
      e.preventDefault()
      document
        .querySelector<HTMLTextAreaElement>(`[data-testid="${DETAIL_TESTID.comment}"]`)
        ?.focus()
      return
    case 'open-comment-cursor':
      if (!cursorKey) return
      e.preventDefault()
      host.triage.openComment(cursorKey)
      return
    case 'new-issue':
      e.preventDefault()
      host.write.openNewIssue()
  }
}

export function createGlobalKeyHandler(host: GlobalKeyHost): (e: KeyboardEvent) => void {
  return function onGlobalKey(e: KeyboardEvent) {
    dispatchKeyCommand(e, resolveGlobalKey(contextFromEvent(e, host)), host)
  }
}
