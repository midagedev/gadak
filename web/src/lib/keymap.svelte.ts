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
 * Chords, labels, palette rows, and the shortcuts sheet are owned by
 * lib/commands.ts — this file matches them and runs the host.
 * `o` is the header escape hatch (issue Jira URL / page source URL).
 */

import {
  DETAIL_TESTID,
  resolveGlobalKey,
  type KeyCommand,
  type KeyContext,
  type TriageMenuKey,
} from './commands'

export {
  DETAIL_TESTID,
  NARROW_FIELD_TESTID,
  isBootHoldKey,
  keyContext,
  narrowFieldTestId,
  resolveGlobalKey,
} from './commands'
export type { KeyCommand, KeyContext, TriageMenuKey } from './commands'

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
  pages: {
    historyView: boolean
    open: boolean
    selectedKey: string | null
    clear: () => void
    closeDocs: () => void
    closeHistory: () => void
  }
  person: { selectedEmail: string | null; clear: () => void }
  bulk: { active: boolean; clear: () => void; toggle: (key: string) => void }
  browse: { paneOpen: boolean; hidePane: () => void }
  me: { feedOpen: boolean; closeFeed: () => void }
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
    case 'close-docs':
      e.preventDefault()
      host.pages.closeDocs()
      return
    case 'close-history':
      e.preventDefault()
      host.pages.closeHistory()
      return
    case 'close-feed':
      e.preventDefault()
      host.me.closeFeed()
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
