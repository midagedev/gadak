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
 * `o` is the header escape hatch (issue Jira URL / page source URL).
 */

export const NARROW_FIELD_TESTID = {
  history: 'history-filter-input',
  docs: 'docs-filter-input',
  issues: 'search-input',
} as const

export const DETAIL_TESTID = {
  status: 'status-transition',
  assignee: 'assignee-picker',
  priority: 'priority-picker',
  labelInput: 'label-editor-input',
  labelAdd: 'label-editor-add',
  comment: 'comment-composer',
} as const

export type TriageMenuKey = 'status' | 'assignee' | 'labels' | 'priority'

export interface KeyContext {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  inEditable: boolean
  /**
   * True when Enter already belongs to the event target (native activation).
   * Distinct from inEditable: a focused button is not a text field, but the
   * browser will click it on Enter and the keymap must not steal that key.
   */
  enterActivating: boolean
  settingsOpen: boolean
  newIssueOpen: boolean
  serverSettingsOpen: boolean
  paletteOpen: boolean
  commentOpen: boolean
  shortcutsOpen: boolean
  mediaViewerOpen: boolean
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
  /**
   * False until the list has absorbed the boot-time first view commit.
   * j/k/x in that window are held, not applied to the unfiltered pool.
   */
  keysReady: boolean
}

export type KeyCommand =
  | { type: 'ignore' }
  | { type: 'toggle-palette' }
  | { type: 'close-shortcuts' }
  | { type: 'open-shortcuts' }
  | { type: 'focus-narrow'; testid: string | null }
  | { type: 'move-list'; dir: 1 | -1 }
  | { type: 'open-cursor' }
  | { type: 'open-origin'; target: 'issue' | 'page' }
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
  | { type: 'open-settings' }
  | { type: 'hold-boot-key'; key: string }

export function keyContext(over: Partial<KeyContext> = {}): KeyContext {
  return {
    key: '',
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    inEditable: false,
    enterActivating: false,
    settingsOpen: false,
    newIssueOpen: false,
    serverSettingsOpen: false,
    paletteOpen: false,
    commentOpen: false,
    shortcutsOpen: false,
    mediaViewerOpen: false,
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
    keysReady: true,
    ...over,
  }
}

/** List keys that must not land on the unfiltered boot pool. */
export function isBootHoldKey(key: string): boolean {
  return key === 'j' || key === 'k' || key === 'x'
}

/**
 * Replay keys held during boot against the committed list.
 * Same verbs the live handler uses (move / toggle-bulk), so j then x
 * is "cursor on the first committed row, then select it".
 */
export function replayHeldListKeys(
  keys: readonly string[],
  act: { move: (dir: 1 | -1) => void; toggleCursor: () => void },
): void {
  for (const key of keys) {
    if (key === 'j') act.move(1)
    else if (key === 'k') act.move(-1)
    else if (key === 'x') act.toggleCursor()
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

function isHtmlish(el: EventTarget | null): el is HTMLElement {
  if (el == null || typeof el !== 'object') return false
  if (typeof HTMLElement !== 'undefined') return el instanceof HTMLElement
  return typeof (el as HTMLElement).tagName === 'string'
}

export function isEditableTarget(el: EventTarget | null): boolean {
  if (!isHtmlish(el)) return false
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
}

/**
 * True when Enter is already this element's activation key. One owner for
 * "does the browser (or contenteditable) consume Enter?" so chrome buttons
 * are not a growing tag-name list next to open-cursor.
 *
 * In: button, a[href], summary, contenteditable, input types the browser
 * activates on Enter (button/submit/reset/image/file).
 * Out: ARIA role=button (the browser does not activate it; IssueRow and
 * DocRow use it, and those rows must keep open-cursor), role=link/menuitem/
 * tab/switch, generic tabindex, checkbox/radio (Space, and already
 * isEditableTarget), select/textarea (already isEditableTarget).
 */
function activatesOnEnter(el: HTMLElement): boolean {
  if (el.isContentEditable) return true
  const tag = el.tagName
  if (tag === 'BUTTON' || tag === 'SUMMARY') return true
  if (tag === 'A') return el.hasAttribute('href')
  if (tag === 'INPUT') {
    const type = (el.getAttribute('type') ?? 'text').toLowerCase()
    return (
      type === 'button' ||
      type === 'submit' ||
      type === 'reset' ||
      type === 'image' ||
      type === 'file'
    )
  }
  return false
}

export function isEnterActivatingTarget(el: EventTarget | null): boolean {
  if (!isHtmlish(el)) return false
  let node: HTMLElement | null = el
  while (node) {
    if (activatesOnEnter(node)) return true
    node = node.parentElement
  }
  return false
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
    ctx.commentOpen ||
    ctx.mediaViewerOpen
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
  // Unused punctuation, same class as `?` and `/`. `s` is status; `g s`
  // would need a prefix latch this resolver does not have.
  if (key === ',') return { type: 'open-settings' }

  if (key === '/') {
    return {
      type: 'focus-narrow',
      testid: narrowFieldTestId(ctx),
    }
  }

  if (ctx.listActive && !ctx.keysReady && isBootHoldKey(key)) {
    return { type: 'hold-boot-key', key }
  }

  if (ctx.listActive && (key === 'j' || key === 'k')) {
    return { type: 'move-list', dir: key === 'j' ? 1 : -1 }
  }
  if (key === 'Enter' && ctx.enterActivating) return { type: 'ignore' }
  if (key === 'Enter' && cursorKey) return { type: 'open-cursor' }

  // Header escape hatch (detail.openJira / doc.openSource). No URL yet —
  // the dispatcher no-ops when the matching button would not open anything.
  if (key === 'o') {
    if (ctx.pageSelected) return { type: 'open-origin', target: 'page' }
    if (cursorKey || ctx.detailOpen) return { type: 'open-origin', target: 'issue' }
    return { type: 'ignore' }
  }

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

  if (key === 's' || key === 'a' || key === 'l' || key === 'p') {
    const menu: TriageMenuKey =
      key === 's' ? 'status' : key === 'a' ? 'assignee' : key === 'l' ? 'labels' : 'priority'
    if (ctx.bulkActive || (!ctx.detailOpen && cursorKey)) {
      return { type: 'request-menu', menu }
    }
    if (ctx.detailOpen) {
      if (key === 'l') return { type: 'activate-labels' }
      return {
        type: 'click-detail',
        testid:
          key === 's'
            ? DETAIL_TESTID.status
            : key === 'a'
              ? DETAIL_TESTID.assignee
              : DETAIL_TESTID.priority,
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
  get mediaViewerOpen(): boolean
  get serverSettingsOpen(): boolean
  set serverSettingsOpen(v: boolean)
  write: { settingsOpen: boolean; newIssueOpen: boolean; openNewIssue: () => void }
  triage: {
    commentKey: string | null
    listActive: boolean
    cursorKey: string | null
    keysReady: boolean
    menu: string | null
    move: (dir: 1 | -1) => void
    holdBootKey: (key: string) => void
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
  openOrigin: (target: 'issue' | 'page') => void
}

function contextFromEvent(e: KeyboardEvent, host: GlobalKeyHost): KeyContext {
  return {
    key: e.key,
    metaKey: e.metaKey,
    ctrlKey: e.ctrlKey,
    altKey: e.altKey,
    inEditable: isEditableTarget(e.target),
    enterActivating: isEnterActivatingTarget(e.target),
    settingsOpen: host.write.settingsOpen,
    newIssueOpen: host.write.newIssueOpen,
    serverSettingsOpen: host.serverSettingsOpen,
    paletteOpen: host.paletteOpen,
    commentOpen: Boolean(host.triage.commentKey),
    shortcutsOpen: host.shortcutsOpen,
    mediaViewerOpen: host.mediaViewerOpen,
    feedBlocksNarrow: host.me.feedOpen && host.feature('feed'),
    historyView: host.pages.historyView,
    docsOpen: host.pages.open,
    listActive: host.triage.listActive,
    cursorKey: host.triage.cursorKey,
    keysReady: host.triage.keysReady,
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
    case 'open-settings':
      e.preventDefault()
      host.serverSettingsOpen = true
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
    case 'open-origin':
      e.preventDefault()
      host.openOrigin(cmd.target)
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
      return
    case 'hold-boot-key':
      e.preventDefault()
      host.triage.holdBootKey(cmd.key)
      return
  }
}

function exposeLastKeyCmd(type: KeyCommand['type']): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.lastKeyCmd = type
}

export function createGlobalKeyHandler(host: GlobalKeyHost): (e: KeyboardEvent) => void {
  return function onGlobalKey(e: KeyboardEvent) {
    // A focused composer (GDK-462) or another surface already spent this key.
    // Still record `ignore` so lastKeyCmd is this keystroke, not the previous one.
    if (e.defaultPrevented) {
      exposeLastKeyCmd('ignore')
      return
    }
    const cmd = resolveGlobalKey(contextFromEvent(e, host))
    // One-step: "what did that keystroke do?" — hold-boot-key means it
    // arrived before keysReady. Same dataset surface as cacheScope / uiFocusPoll.
    exposeLastKeyCmd(cmd.type)
    dispatchKeyCommand(e, cmd, host)
  }
}
