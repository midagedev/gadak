/*
 * Single owner of command facts: key chords, i18n labels, palette rows,
 * and the shortcuts-sheet groups. keymap.svelte.ts matches chords and
 * dispatches; CommandPalette and ShortcutsDialog render from here.
 *
 * Adding a command is an entry in COMMANDS. Do not re-list chords in the
 * three consumers — GDK-618 / GDK-652 were that split.
 *
 * What a key is bound to:
 *   npx vitest run web/src/lib/commands.test.ts -t dumpKey
 */

import type { MessageKey } from './i18n/catalog'

export type { MessageKey }

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
  /** Carried for code chords (GDK-1250); plain chords never read it. */
  shiftKey: boolean
  /** ev.code — the physical key. Null when the caller had no event (unit
   *  tests), which is also why a code chord can never match there. */
  code: string | null
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
  /** A dashboard holds the main column (GDK-827). */
  dashboardOpen: boolean
  /**
   * The terminal pane is open as an overlay over the content track
   * (GDK-945). A docked split is not this — there Esc stays the column's.
   */
  terminalOverlayOpen: boolean
  /**
   * This keydown originated inside the terminal host, so the VT owns it —
   * Esc included (vim, less, fzf live on it). Chrome must not answer it.
   */
  keyFromTerminalHost: boolean
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
  | { type: 'toggle-terminal' }
  | { type: 'terminal-prev-session' }
  | { type: 'terminal-next-session' }
  | { type: 'terminal-focus-strip' }
  | { type: 'terminal-open-issue' }
  | { type: 'close-shortcuts' }
  | { type: 'open-shortcuts' }
  | { type: 'focus-narrow'; testid: string | null }
  | { type: 'move-list'; dir: 1 | -1 }
  | { type: 'open-cursor' }
  | { type: 'open-origin'; target: 'issue' | 'page' }
  | { type: 'hide-browse' }
  | { type: 'clear-bulk' }
  | { type: 'clear-selection' }
  | { type: 'close-docs' }
  | { type: 'close-history' }
  | { type: 'close-dashboard' }
  | { type: 'close-feed' }
  | { type: 'close-terminal-overlay' }
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
    shiftKey: false,
    code: null,
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
    dashboardOpen: false,
    terminalOverlayOpen: false,
    keyFromTerminalHost: false,
    historyView: false,
    docsOpen: false,
    listActive: false,
    cursorKey: null,
    keysReady: true,
    detailOpen: false,
    browsePaneOpen: false,
    triageMenuOpen: false,
    bulkActive: false,
    pageSelected: false,
    personSelected: false,
    ...over,
  }
}

/** List cursor the resolver actually sees — null unless the list is active. */
export function listCursor(ctx: KeyContext): string | null {
  return ctx.listActive ? ctx.cursorKey : null
}

/** Which main-column field `/` should focus, or null when a full-column
 *  surface without one (feed, dashboard) owns the column. */
export function narrowFieldTestId(ctx: {
  feedBlocksNarrow: boolean
  dashboardOpen: boolean
  historyView: boolean
  docsOpen: boolean
}): string | null {
  if (ctx.feedBlocksNarrow || ctx.dashboardOpen) return null
  if (ctx.historyView) return NARROW_FIELD_TESTID.history
  if (ctx.docsOpen) return NARROW_FIELD_TESTID.docs
  return NARROW_FIELD_TESTID.issues
}

export type KeyPhase = 'always' | 'shortcuts-open' | 'default'

/**
 * Same-chord commands may coexist when their scopes differ (c in detail vs
 * list vs global). Duplicate (phase, scope, chord) is the integrity failure.
 */
export type KeyScope =
  | 'always'
  | 'shortcuts-open'
  | 'global'
  | 'boot'
  | 'list'
  | 'detail'
  | 'bulk'
  | 'page'
  | 'person'
  | 'browse'
  | 'overlay-terminal'
  | 'overlay-feed'
  | 'overlay-dashboard'
  | 'overlay-history'
  | 'overlay-docs'

export interface Chord {
  /** ev.key — the printed character. Omitted on a code chord. */
  key?: string
  /** meta OR ctrl, matching ⌘K / Ctrl+K. */
  mod?: boolean
  /** ev.code — the physical key, for the Shift-sensitive chords (GDK-1250):
   *  with Shift held, '[' prints '{' on a US layout and something else on
   *  every other one, and the binding is to the key, not the glyph. A chord
   *  carries exactly one of key/code. */
  code?: string
  /** Shift held (code chords only — a plain chord's shifted form is already
   *  a different ev.key). */
  shift?: boolean
}

export type HelpGroupId =
  | 'global'
  | 'list'
  | 'columnViews'
  | 'detail'
  | 'search'
  | 'palette'
  | 'compose'

export interface HelpRow {
  group: HelpGroupId
  /** Sheet glyphs. `{mod}` is replaced with ⌘ or Ctrl. */
  kbd: string
  labelKey: MessageKey
  sort: number
}

export type PaletteKind =
  | 'triage-menu'
  | 'triage-comment'
  | 'triage-select'
  | 'favorite'
  | 'watch'
  | 'triage-clear'
  | 'origin'
  | 'new-issue'
  | 'always'
  | 'feed'
  | 'toggle-flag'
  | 'locales'
  | 'themes'
  | 'create-now'

export interface PaletteSpec {
  id: string
  kind: PaletteKind
  sort: number
  kbd?: string
  testid?: string
  labelKey: MessageKey
  altLabelKey?: MessageKey
  menu?: TriageMenuKey
  flag?: 'reopened' | 'unassigned' | 'stale'
}

export interface CommandDef {
  id: string
  chords: readonly Chord[]
  phase?: KeyPhase
  scope?: KeyScope
  when?: (ctx: KeyContext) => boolean
  dispatch?: (ctx: KeyContext) => KeyCommand
  palette?: PaletteSpec
  help?: HelpRow | readonly HelpRow[]
}

export const HELP_GROUPS: readonly { id: HelpGroupId; titleKey: MessageKey }[] = [
  { id: 'global', titleKey: 'shortcuts.sectionGlobal' },
  { id: 'list', titleKey: 'shortcuts.sectionList' },
  { id: 'columnViews', titleKey: 'shortcuts.sectionColumnViews' },
  { id: 'detail', titleKey: 'shortcuts.sectionDetail' },
  { id: 'search', titleKey: 'shortcuts.sectionSearch' },
  { id: 'palette', titleKey: 'shortcuts.sectionPalette' },
  { id: 'compose', titleKey: 'shortcuts.sectionCompose' },
]

function hasListTarget(ctx: KeyContext): boolean {
  return ctx.bulkActive || (!ctx.detailOpen && Boolean(listCursor(ctx)))
}

function escFreeOfBrowseMenu(ctx: KeyContext): boolean {
  return !ctx.browsePaneOpen && !ctx.triageMenuOpen
}

/**
 * Registry order is keymap match order (first chord+when wins). Palette
 * sort and help sort are independent and match the previous handwritten
 * tables.
 */
export const COMMANDS: readonly CommandDef[] = [
  {
    id: 'toggle-palette',
    phase: 'always',
    scope: 'always',
    chords: [{ key: 'k', mod: true }],
    dispatch: () => ({ type: 'toggle-palette' }),
    help: { group: 'global', kbd: '{mod} K', labelKey: 'shortcuts.palette', sort: 10 },
  },
  /*
   * GDK-1250/GDK-1251: the four Ctrl+Shift chords the app owns while the VT
   * holds focus — renderer.ts isAppChord is the door that lets them out of
   * xterm, so they never reach the PTY. Chords are ev.code, not ev.key:
   * Shift turns '[' into '{' on a US layout and into something else on every
   * other one, and the binding is to the physical key. Registered ahead of
   * toggle-terminal on purpose — the matcher is first-chord-wins and that
   * entry is Shift-agnostic, so a layout whose Shift+Backquote prints '`'
   * would otherwise hand Ctrl+Shift+` to it. `when` matches its Ctrl-not-Cmd
   * shape too: Cmd+Shift+… stays macOS's. Sheet rows only, no palette rows —
   * the pane's own strip is the surface these act on.
   */
  {
    id: 'terminal-prev-session',
    phase: 'always',
    scope: 'always',
    chords: [{ code: 'BracketLeft', mod: true, shift: true }],
    when: (ctx) => ctx.ctrlKey && !ctx.metaKey && !ctx.altKey,
    dispatch: () => ({ type: 'terminal-prev-session' }),
    help: {
      group: 'global',
      kbd: 'Ctrl+Shift+[',
      labelKey: 'shortcuts.terminalPrevSession',
      sort: 16,
    },
  },
  {
    id: 'terminal-next-session',
    phase: 'always',
    scope: 'always',
    chords: [{ code: 'BracketRight', mod: true, shift: true }],
    when: (ctx) => ctx.ctrlKey && !ctx.metaKey && !ctx.altKey,
    dispatch: () => ({ type: 'terminal-next-session' }),
    help: {
      group: 'global',
      kbd: 'Ctrl+Shift+]',
      labelKey: 'shortcuts.terminalNextSession',
      sort: 17,
    },
  },
  {
    id: 'terminal-focus-strip',
    phase: 'always',
    scope: 'always',
    chords: [{ code: 'Backquote', mod: true, shift: true }],
    when: (ctx) => ctx.ctrlKey && !ctx.metaKey && !ctx.altKey,
    dispatch: () => ({ type: 'terminal-focus-strip' }),
    // Ctrl+Shift+` was toggle-terminal's conflict fallback, left unused on
    // purpose when Ctrl+` landed — this is the job it was being kept for:
    // leave the VT without closing the pane, focus on the active tab.
    help: {
      group: 'global',
      kbd: 'Ctrl+Shift+`',
      labelKey: 'shortcuts.terminalFocusTabs',
      sort: 18,
    },
  },
  {
    id: 'terminal-open-issue',
    phase: 'always',
    scope: 'always',
    chords: [{ code: 'KeyO', mod: true, shift: true }],
    when: (ctx) => ctx.ctrlKey && !ctx.metaKey && !ctx.altKey,
    dispatch: () => ({ type: 'terminal-open-issue' }),
    help: {
      group: 'global',
      kbd: 'Ctrl+Shift+O',
      labelKey: 'shortcuts.terminalOpenIssue',
      sort: 19,
    },
  },
  /*
   * Ctrl+` (VS Code). Always-phase so it fires while the terminal textarea
   * is focused. `when` requires ctrl and not meta: Cmd+` is macOS's
   * same-app window cycle and is not ours. No existing backquote binding
   * was found; the Ctrl+Shift+` conflict fallback stayed unused until
   * terminal-focus-strip took it over (GDK-1250).
   */
  {
    id: 'toggle-terminal',
    phase: 'always',
    scope: 'always',
    chords: [{ key: '`', mod: true }],
    when: (ctx) => ctx.ctrlKey && !ctx.metaKey && !ctx.altKey,
    dispatch: () => ({ type: 'toggle-terminal' }),
    // In the palette as well as on a chord: ⌘K is this app's answer to "how
    // do I do anything", and a surface that is only reachable by a shortcut
    // is reachable only by someone who already knows it. The row carries the
    // chord so the palette is where you learn it.
    palette: {
      id: 'a:terminal',
      kind: 'always',
      sort: 125,
      kbd: 'Ctrl+`',
      testid: 'palette-action-terminal',
      labelKey: 'palette.actionTerminal',
    },
    help: { group: 'global', kbd: 'Ctrl+`', labelKey: 'shortcuts.terminal', sort: 15 },
  },
  {
    id: 'close-shortcuts',
    phase: 'shortcuts-open',
    scope: 'shortcuts-open',
    chords: [{ key: '?' }],
    dispatch: () => ({ type: 'close-shortcuts' }),
  },
  {
    id: 'open-shortcuts',
    scope: 'global',
    chords: [{ key: '?' }],
    dispatch: () => ({ type: 'open-shortcuts' }),
    help: { group: 'global', kbd: '?', labelKey: 'shortcuts.help', sort: 40 },
  },
  {
    id: 'open-settings',
    scope: 'global',
    chords: [{ key: ',' }],
    dispatch: () => ({ type: 'open-settings' }),
    palette: {
      id: 'a:settings',
      kind: 'always',
      sort: 110,
      kbd: ',',
      labelKey: 'palette.actionSettings',
    },
    help: { group: 'global', kbd: ',', labelKey: 'shortcuts.settings', sort: 20 },
  },
  {
    id: 'focus-narrow',
    scope: 'global',
    chords: [{ key: '/' }],
    dispatch: (ctx) => ({ type: 'focus-narrow', testid: narrowFieldTestId(ctx) }),
    help: { group: 'search', kbd: '/', labelKey: 'shortcuts.focusSearch', sort: 10 },
  },
  {
    id: 'hold-boot-j',
    scope: 'boot',
    chords: [{ key: 'j' }],
    when: (ctx) => ctx.listActive && !ctx.keysReady,
    dispatch: (ctx) => ({ type: 'hold-boot-key', key: ctx.key }),
  },
  {
    id: 'hold-boot-k',
    scope: 'boot',
    chords: [{ key: 'k' }],
    when: (ctx) => ctx.listActive && !ctx.keysReady,
    dispatch: (ctx) => ({ type: 'hold-boot-key', key: ctx.key }),
  },
  {
    id: 'hold-boot-x',
    scope: 'boot',
    chords: [{ key: 'x' }],
    when: (ctx) => ctx.listActive && !ctx.keysReady,
    dispatch: (ctx) => ({ type: 'hold-boot-key', key: ctx.key }),
  },
  {
    id: 'move-down',
    scope: 'list',
    chords: [{ key: 'j' }],
    when: (ctx) => ctx.listActive,
    dispatch: () => ({ type: 'move-list', dir: 1 }),
    help: { group: 'list', kbd: 'j', labelKey: 'shortcuts.moveDown', sort: 10 },
  },
  {
    id: 'move-up',
    scope: 'list',
    chords: [{ key: 'k' }],
    when: (ctx) => ctx.listActive,
    dispatch: () => ({ type: 'move-list', dir: -1 }),
    help: { group: 'list', kbd: 'k', labelKey: 'shortcuts.moveUp', sort: 20 },
  },
  {
    id: 'open-cursor',
    scope: 'list',
    chords: [{ key: 'Enter' }],
    when: (ctx) => !ctx.enterActivating && Boolean(listCursor(ctx)),
    dispatch: () => ({ type: 'open-cursor' }),
    help: { group: 'list', kbd: '↵', labelKey: 'shortcuts.openIssue', sort: 30 },
  },
  {
    id: 'open-origin',
    scope: 'detail',
    chords: [{ key: 'o' }],
    when: (ctx) => Boolean(ctx.pageSelected || listCursor(ctx) || ctx.detailOpen),
    dispatch: (ctx) => ({ type: 'open-origin', target: ctx.pageSelected ? 'page' : 'issue' }),
    palette: {
      id: 'a:open-origin',
      kind: 'origin',
      sort: 90,
      kbd: 'o',
      labelKey: 'detail.openJira',
    },
    help: [
      { group: 'list', kbd: 'o', labelKey: 'detail.openJira', sort: 40 },
      { group: 'detail', kbd: 'o', labelKey: 'shortcuts.detailOpenJira', sort: 10 },
      { group: 'detail', kbd: 'o', labelKey: 'doc.openSource', sort: 20 },
    ],
  },
  {
    id: 'hide-browse',
    scope: 'browse',
    chords: [{ key: 'Escape' }],
    when: (ctx) => ctx.browsePaneOpen,
    dispatch: () => ({ type: 'hide-browse' }),
    help: { group: 'global', kbd: 'Esc', labelKey: 'browse.back', sort: 50 },
  },
  {
    id: 'clear-bulk',
    scope: 'bulk',
    chords: [{ key: 'Escape' }],
    when: (ctx) => escFreeOfBrowseMenu(ctx) && ctx.bulkActive,
    dispatch: () => ({ type: 'clear-bulk' }),
    palette: {
      id: 'a:triage-clear',
      kind: 'triage-clear',
      sort: 80,
      kbd: 'Esc',
      labelKey: 'palette.actionTriageClear',
    },
  },
  {
    id: 'clear-selection-esc',
    scope: 'detail',
    chords: [{ key: 'Escape' }],
    when: (ctx) => escFreeOfBrowseMenu(ctx) && !ctx.bulkActive && ctx.detailOpen,
    dispatch: () => ({ type: 'clear-selection' }),
    help: { group: 'list', kbd: 'Esc', labelKey: 'shortcuts.clearSelection', sort: 110 },
  },
  /*
   * GDK-945, axis B: the overlay terminal joins the Esc ladder — but only
   * when the VT does not hold the keystroke. A focused terminal's Esc is
   * the PTY's (vim, less, fzf), so keyFromTerminalHost refuses the key even
   * though keymap's defaultPrevented exception is what lets terminal keys
   * reach this resolver at all (that is how Ctrl+` closes from inside).
   * The pane covers the content track, so it takes its turn before the
   * column views; a docked split never enters — terminalOverlayOpen is the
   * overlay owner's verdict (terminalChrome open && narrow). No help row on
   * purpose: closing the top surface is what Esc already means everywhere
   * else in the ladder (close-feed/close-dashboard precedent).
   */
  {
    id: 'close-terminal-overlay',
    scope: 'overlay-terminal',
    chords: [{ key: 'Escape' }],
    when: (ctx) =>
      escFreeOfBrowseMenu(ctx) &&
      !ctx.bulkActive &&
      !ctx.detailOpen &&
      ctx.terminalOverlayOpen &&
      !ctx.keyFromTerminalHost,
    dispatch: () => ({ type: 'close-terminal-overlay' }),
  },
  {
    id: 'close-feed',
    scope: 'overlay-feed',
    chords: [{ key: 'Escape' }],
    when: (ctx) =>
      escFreeOfBrowseMenu(ctx) &&
      !ctx.bulkActive &&
      !ctx.detailOpen &&
      !ctx.terminalOverlayOpen &&
      ctx.feedBlocksNarrow,
    dispatch: () => ({ type: 'close-feed' }),
  },
  /*
   * The dashboard's slot is the feed's: a full-column surface, so Esc gives
   * the column back to the list — but only after bulk/detail had their turn
   * (an open detail panel closes first; that order predates this entry and
   * stays). The column union keeps the flags one-of-many, and the guards
   * below still spell the chain out because the registry's order is the
   * chain's documentation (GDK-827).
   */
  {
    id: 'close-dashboard',
    scope: 'overlay-dashboard',
    chords: [{ key: 'Escape' }],
    when: (ctx) =>
      escFreeOfBrowseMenu(ctx) &&
      !ctx.bulkActive &&
      !ctx.detailOpen &&
      !ctx.terminalOverlayOpen &&
      !ctx.feedBlocksNarrow &&
      ctx.dashboardOpen,
    dispatch: () => ({ type: 'close-dashboard' }),
  },
  {
    id: 'close-history',
    scope: 'overlay-history',
    chords: [{ key: 'Escape' }],
    when: (ctx) =>
      escFreeOfBrowseMenu(ctx) &&
      !ctx.bulkActive &&
      !ctx.detailOpen &&
      !ctx.terminalOverlayOpen &&
      !ctx.feedBlocksNarrow &&
      !ctx.dashboardOpen &&
      ctx.historyView,
    dispatch: () => ({ type: 'close-history' }),
  },
  {
    id: 'close-docs',
    scope: 'overlay-docs',
    chords: [{ key: 'Escape' }],
    when: (ctx) =>
      escFreeOfBrowseMenu(ctx) &&
      !ctx.bulkActive &&
      !ctx.detailOpen &&
      !ctx.terminalOverlayOpen &&
      !ctx.feedBlocksNarrow &&
      !ctx.dashboardOpen &&
      !ctx.historyView &&
      ctx.docsOpen,
    dispatch: () => ({ type: 'close-docs' }),
    help: { group: 'columnViews', kbd: 'Esc', labelKey: 'shortcuts.closeColumnView', sort: 20 },
  },
  {
    id: 'clear-page',
    scope: 'page',
    chords: [{ key: 'x' }],
    when: (ctx) => ctx.pageSelected,
    dispatch: () => ({ type: 'clear-page' }),
  },
  {
    id: 'clear-person',
    scope: 'person',
    chords: [{ key: 'x' }],
    when: (ctx) => !ctx.pageSelected && ctx.personSelected,
    dispatch: () => ({ type: 'clear-person' }),
  },
  {
    id: 'toggle-bulk-cursor',
    scope: 'list',
    chords: [{ key: 'x' }],
    when: (ctx) => !ctx.pageSelected && !ctx.personSelected && Boolean(listCursor(ctx)),
    dispatch: () => ({ type: 'toggle-bulk-cursor' }),
    palette: {
      id: 'a:triage-select',
      kind: 'triage-select',
      sort: 50,
      kbd: 'x',
      labelKey: 'palette.actionTriageSelect',
      altLabelKey: 'palette.actionTriageDeselect',
    },
    help: { group: 'list', kbd: 'x', labelKey: 'shortcuts.selectRow', sort: 50 },
  },
  {
    id: 'clear-selection-x',
    scope: 'detail',
    chords: [{ key: 'x' }],
    when: (ctx) =>
      !ctx.pageSelected && !ctx.personSelected && !listCursor(ctx) && ctx.detailOpen,
    dispatch: () => ({ type: 'clear-selection' }),
  },
  {
    id: 'list-status',
    scope: 'list',
    chords: [{ key: 's' }],
    when: hasListTarget,
    dispatch: () => ({ type: 'request-menu', menu: 'status' }),
    palette: {
      id: 'a:triage-status',
      kind: 'triage-menu',
      sort: 10,
      kbd: 's',
      labelKey: 'palette.actionTriageStatus',
      menu: 'status',
    },
    help: { group: 'list', kbd: 's', labelKey: 'shortcuts.listStatus', sort: 60 },
  },
  {
    id: 'list-priority',
    scope: 'list',
    chords: [{ key: 'p' }],
    when: hasListTarget,
    dispatch: () => ({ type: 'request-menu', menu: 'priority' }),
    help: { group: 'list', kbd: 'p', labelKey: 'shortcuts.listPriority', sort: 70 },
  },
  {
    id: 'list-assignee',
    scope: 'list',
    chords: [{ key: 'a' }],
    when: hasListTarget,
    dispatch: () => ({ type: 'request-menu', menu: 'assignee' }),
    palette: {
      id: 'a:triage-assignee',
      kind: 'triage-menu',
      sort: 20,
      kbd: 'a',
      labelKey: 'palette.actionTriageAssignee',
      menu: 'assignee',
    },
    help: { group: 'list', kbd: 'a', labelKey: 'shortcuts.listAssignee', sort: 80 },
  },
  {
    id: 'list-labels',
    scope: 'list',
    chords: [{ key: 'l' }],
    when: hasListTarget,
    dispatch: () => ({ type: 'request-menu', menu: 'labels' }),
    palette: {
      id: 'a:triage-labels',
      kind: 'triage-menu',
      sort: 30,
      kbd: 'l',
      labelKey: 'palette.actionTriageLabels',
      menu: 'labels',
    },
    help: { group: 'list', kbd: 'l', labelKey: 'shortcuts.listLabels', sort: 90 },
  },
  {
    id: 'detail-labels',
    scope: 'detail',
    chords: [{ key: 'l' }],
    when: (ctx) => ctx.detailOpen,
    dispatch: () => ({ type: 'activate-labels' }),
    help: { group: 'detail', kbd: 'l', labelKey: 'shortcuts.focusLabels', sort: 60 },
  },
  {
    id: 'detail-status',
    scope: 'detail',
    chords: [{ key: 's' }],
    when: (ctx) => ctx.detailOpen,
    dispatch: () => ({ type: 'click-detail', testid: DETAIL_TESTID.status }),
    help: { group: 'detail', kbd: 's', labelKey: 'shortcuts.focusStatus', sort: 30 },
  },
  {
    id: 'detail-assignee',
    scope: 'detail',
    chords: [{ key: 'a' }],
    when: (ctx) => ctx.detailOpen,
    dispatch: () => ({ type: 'click-detail', testid: DETAIL_TESTID.assignee }),
    help: { group: 'detail', kbd: 'a', labelKey: 'shortcuts.focusAssignee', sort: 50 },
  },
  {
    id: 'detail-priority',
    scope: 'detail',
    chords: [{ key: 'p' }],
    when: (ctx) => ctx.detailOpen,
    dispatch: () => ({ type: 'click-detail', testid: DETAIL_TESTID.priority }),
    help: { group: 'detail', kbd: 'p', labelKey: 'shortcuts.focusPriority', sort: 40 },
  },
  {
    id: 'focus-comment',
    scope: 'detail',
    chords: [{ key: 'c' }],
    when: (ctx) => ctx.detailOpen,
    dispatch: () => ({ type: 'focus-comment' }),
    help: { group: 'detail', kbd: 'c', labelKey: 'shortcuts.focusComment', sort: 70 },
  },
  {
    id: 'open-comment-cursor',
    scope: 'list',
    chords: [{ key: 'c' }],
    when: (ctx) => !ctx.detailOpen && Boolean(listCursor(ctx)),
    dispatch: () => ({ type: 'open-comment-cursor' }),
    palette: {
      id: 'a:triage-comment',
      kind: 'triage-comment',
      sort: 40,
      kbd: 'c',
      labelKey: 'palette.actionTriageComment',
    },
    help: { group: 'list', kbd: 'c', labelKey: 'shortcuts.listComment', sort: 100 },
  },
  {
    id: 'new-issue',
    scope: 'global',
    chords: [{ key: 'c' }],
    when: (ctx) => !ctx.detailOpen && !listCursor(ctx),
    dispatch: () => ({ type: 'new-issue' }),
    palette: {
      id: 'a:new',
      kind: 'new-issue',
      sort: 100,
      kbd: 'c',
      testid: 'palette-new-issue',
      labelKey: 'write.newIssue',
    },
    help: { group: 'global', kbd: 'c', labelKey: 'shortcuts.newIssueContext', sort: 30 },
  },

  /* Palette-only — no global keymap chord. */
  {
    id: 'a:favorite',
    chords: [],
    palette: {
      id: 'a:favorite',
      kind: 'favorite',
      sort: 60,
      testid: 'palette-action-favorite',
      labelKey: 'palette.actionFavorite',
      altLabelKey: 'palette.actionUnfavorite',
    },
  },
  {
    id: 'a:watch',
    chords: [],
    palette: {
      id: 'a:watch',
      kind: 'watch',
      sort: 70,
      testid: 'palette-action-watch',
      labelKey: 'palette.actionWatch',
      altLabelKey: 'palette.actionUnwatch',
    },
  },
  {
    id: 'a:history',
    chords: [],
    palette: {
      id: 'a:history',
      kind: 'always',
      sort: 120,
      labelKey: 'palette.actionHistory',
    },
  },
  {
    id: 'a:docs',
    chords: [],
    palette: {
      id: 'a:docs',
      kind: 'always',
      sort: 130,
      testid: 'palette-action-docs',
      labelKey: 'palette.actionDocs',
    },
  },
  {
    id: 'a:feed',
    chords: [],
    palette: {
      id: 'a:feed',
      kind: 'feed',
      sort: 140,
      testid: 'palette-action-feed',
      labelKey: 'palette.actionFeed',
    },
  },
  {
    id: 'a:reset',
    chords: [],
    palette: {
      id: 'a:reset',
      kind: 'always',
      sort: 150,
      labelKey: 'filter.clear',
    },
  },
  {
    id: 'a:reopened',
    chords: [],
    palette: {
      id: 'a:reopened',
      kind: 'toggle-flag',
      sort: 160,
      labelKey: 'palette.actionToggleReopened',
      flag: 'reopened',
    },
  },
  {
    id: 'a:unassigned',
    chords: [],
    palette: {
      id: 'a:unassigned',
      kind: 'toggle-flag',
      sort: 170,
      labelKey: 'palette.actionToggleUnassigned',
      flag: 'unassigned',
    },
  },
  {
    id: 'a:stale',
    chords: [],
    palette: {
      id: 'a:stale',
      kind: 'toggle-flag',
      sort: 180,
      labelKey: 'palette.actionToggleStale',
      flag: 'stale',
    },
  },
  {
    id: 'a:locale',
    chords: [],
    palette: {
      id: 'a:locale',
      kind: 'locales',
      sort: 190,
      labelKey: 'palette.actionLocale',
    },
  },
  {
    id: 'a:theme',
    chords: [],
    palette: {
      id: 'a:theme',
      kind: 'themes',
      sort: 200,
      labelKey: 'palette.actionTheme',
    },
  },
  {
    id: 'a:sync',
    chords: [],
    palette: {
      id: 'a:sync',
      kind: 'always',
      sort: 210,
      labelKey: 'palette.actionSyncStatus',
    },
  },
  {
    id: 'a:copy-view-link',
    chords: [],
    palette: {
      id: 'a:copy-view-link',
      kind: 'always',
      sort: 115,
      labelKey: 'view.copyLink',
    },
  },
  {
    id: 'a:sync-now',
    chords: [],
    palette: {
      id: 'a:sync-now',
      kind: 'always',
      sort: 220,
      labelKey: 'palette.actionSyncNow',
    },
  },
  {
    id: 'a:create-now',
    chords: [],
    palette: {
      id: 'a:create-now',
      kind: 'create-now',
      sort: 230,
      testid: 'palette-create-now',
      labelKey: 'palette.actionCreateIssue',
    },
  },

  /* Help-only — local handlers (SearchBox, palette input, composer, Tab). */
  {
    id: 'help-tab-column-views',
    chords: [],
    help: { group: 'columnViews', kbd: 'Tab', labelKey: 'shortcuts.tabMoveRows', sort: 10 },
  },
  {
    id: 'help-search-suggestions',
    chords: [],
    help: { group: 'search', kbd: '↑ ↓', labelKey: 'shortcuts.suggestions', sort: 20 },
  },
  {
    id: 'help-search-apply',
    chords: [],
    help: { group: 'search', kbd: '↵', labelKey: 'shortcuts.applySearch', sort: 30 },
  },
  {
    id: 'help-search-clear',
    chords: [],
    help: { group: 'search', kbd: 'Esc', labelKey: 'shortcuts.clearSearch', sort: 40 },
  },
  {
    id: 'help-palette-move',
    chords: [],
    help: { group: 'palette', kbd: '↑ ↓', labelKey: 'shortcuts.paletteMove', sort: 10 },
  },
  {
    id: 'help-palette-run',
    chords: [],
    help: { group: 'palette', kbd: '↵', labelKey: 'shortcuts.paletteRun', sort: 20 },
  },
  {
    id: 'help-palette-close',
    chords: [],
    help: { group: 'palette', kbd: 'Esc', labelKey: 'shortcuts.paletteClose', sort: 30 },
  },
  {
    id: 'help-submit-comment',
    chords: [],
    help: { group: 'compose', kbd: '{mod} ↵', labelKey: 'shortcuts.submitComment', sort: 10 },
  },
]

/** List keys that must not land on the unfiltered boot pool. */
export function isBootHoldKey(key: string): boolean {
  return COMMANDS.some(
    (c) => c.scope === 'boot' && c.chords.some((ch) => !ch.mod && ch.key === key),
  )
}

export function chordMatches(ctx: KeyContext, chords: readonly Chord[]): boolean {
  return chords.some((ch) => {
    // Code chords (GDK-1250): the physical key decides, and Shift is part
    // of the chord or explicitly absent — the shiftless form of these keys
    // belongs to the PTY, not to a fuzzy match here.
    if (ch.code) {
      if (ctx.code !== ch.code) return false
      if (ch.shift ? !ctx.shiftKey : ctx.shiftKey) return false
      return ch.mod ? ctx.metaKey || ctx.ctrlKey : true
    }
    if (!ch.key) return false
    if (ch.mod) {
      return (ctx.metaKey || ctx.ctrlKey) && ctx.key.toLowerCase() === ch.key.toLowerCase()
    }
    return ctx.key === ch.key
  })
}

function phaseOf(cmd: CommandDef): KeyPhase {
  return cmd.phase ?? 'default'
}

function firstMatch(ctx: KeyContext, phase: KeyPhase): CommandDef | undefined {
  for (const cmd of COMMANDS) {
    if (phaseOf(cmd) !== phase) continue
    if (!cmd.dispatch || cmd.chords.length === 0) continue
    if (!chordMatches(ctx, cmd.chords)) continue
    if (cmd.when && !cmd.when(ctx)) continue
    return cmd
  }
  return undefined
}

function blocksGlobalKeys(ctx: KeyContext): boolean {
  return (
    ctx.settingsOpen ||
    ctx.newIssueOpen ||
    ctx.serverSettingsOpen ||
    ctx.paletteOpen ||
    ctx.commentOpen ||
    ctx.mediaViewerOpen
  )
}

/**
 * Decide what a key means. DOM (does the field exist?) is the handler's job —
 * `/` still returns `focus-narrow` when the testid is known, and the handler
 * only preventDefault-s if the node is on the page.
 */
export function resolveGlobalKey(ctx: KeyContext): KeyCommand {
  const always = firstMatch(ctx, 'always')
  if (always?.dispatch) return always.dispatch(ctx)

  if (ctx.metaKey || ctx.ctrlKey || ctx.altKey) return { type: 'ignore' }
  if (ctx.inEditable) return { type: 'ignore' }
  if (blocksGlobalKeys(ctx)) return { type: 'ignore' }

  if (ctx.shortcutsOpen) {
    const sheet = firstMatch(ctx, 'shortcuts-open')
    if (sheet?.dispatch) return sheet.dispatch(ctx)
    return { type: 'ignore' }
  }

  const hit = firstMatch(ctx, 'default')
  return hit?.dispatch ? hit.dispatch(ctx) : { type: 'ignore' }
}

export function helpRowsOf(cmd: CommandDef): readonly HelpRow[] {
  if (!cmd.help) return []
  return Array.isArray(cmd.help) ? cmd.help : ([cmd.help] as HelpRow[])
}

export function formatHelpKbd(template: string, mod: string): string {
  return template.replaceAll('{mod}', mod)
}

export interface HelpSectionView {
  titleKey: MessageKey
  rows: { kbd: string; labelKey: MessageKey }[]
}

export function helpSections(mod: string): HelpSectionView[] {
  return HELP_GROUPS.map((group) => {
    const rows = COMMANDS.flatMap((cmd) =>
      helpRowsOf(cmd)
        .filter((row) => row.group === group.id)
        .map((row) => ({
          kbd: formatHelpKbd(row.kbd, mod),
          labelKey: row.labelKey,
          sort: row.sort,
        })),
    ).sort((a, b) => a.sort - b.sort || a.labelKey.localeCompare(b.labelKey))
    return {
      titleKey: group.titleKey,
      rows: rows.map(({ kbd, labelKey }) => ({ kbd, labelKey })),
    }
  }).filter((section) => section.rows.length > 0)
}

export function collectLabelKeys(commands: readonly CommandDef[] = COMMANDS): MessageKey[] {
  const keys = new Set<MessageKey>()
  for (const group of HELP_GROUPS) keys.add(group.titleKey)
  for (const cmd of commands) {
    for (const row of helpRowsOf(cmd)) keys.add(row.labelKey)
    if (cmd.palette) {
      keys.add(cmd.palette.labelKey)
      if (cmd.palette.altLabelKey) keys.add(cmd.palette.altLabelKey)
    }
  }
  return [...keys]
}

export function duplicateBindingKeys(commands: readonly CommandDef[] = COMMANDS): string[] {
  const seen = new Map<string, string>()
  const dupes: string[] = []
  for (const cmd of commands) {
    if (!cmd.dispatch || cmd.chords.length === 0) continue
    const phase = phaseOf(cmd)
    const scope = cmd.scope ?? 'global'
    for (const ch of cmd.chords) {
      const token = `${phase}\0${scope}\0${ch.mod ? 'mod' : ''}${ch.shift ? '+shift' : ''}\0${ch.code ?? ch.key}`
      const prev = seen.get(token)
      if (prev) {
        dupes.push(`${ch.mod ? 'mod+' : ''}${ch.code ?? ch.key} scope=${scope} (${prev} ∩ ${cmd.id})`)
      } else seen.set(token, cmd.id)
    }
  }
  return dupes
}

function chordDump(ch: Chord): string {
  return `${ch.mod ? 'mod+' : ''}${ch.shift ? 'shift+' : ''}${ch.code ?? ch.key}`
}

function dumpQueryHits(text: string, q: string): boolean {
  const a = text.toLowerCase()
  const b = q.toLowerCase()
  if (a === b) return true
  if ((b === 'esc' || b === 'escape') && (a === 'escape' || a === 'esc')) return true
  if ((b === 'enter' || b === '↵') && (a === 'enter' || a === '↵')) return true
  if (b === 'mod+k' && (a === 'mod+k' || a === 'k')) return true
  return false
}

/**
 * One-line answer to "what is this key bound to?". Pass a key (`s`, `Escape`,
 * `mod+k`) or omit for the full table.
 */
export function dumpKeyBindings(key?: string): string {
  const lines: string[] = []
  for (const cmd of COMMANDS) {
    const rows = helpRowsOf(cmd)
    const help = rows.map((h) => `${h.group}:${h.labelKey}`).join(',')
    const pal = cmd.palette ? `palette:${cmd.palette.id}` : '-'
    const dispatch = cmd.dispatch ? 'keymap' : '-'
    if (cmd.chords.length === 0) {
      const kbd = rows.map((h) => h.kbd).join('|') || cmd.palette?.kbd || '-'
      if (!key) {
        lines.push(`${kbd}\t${cmd.id}\t${dispatch}\t${pal}\t${help || '-'}`)
      } else if (
        dumpQueryHits(cmd.palette?.kbd ?? '', key) ||
        rows.some((h) => dumpQueryHits(h.kbd.replace('{mod} ', 'mod+').replace('{mod}', 'mod+'), key) || dumpQueryHits(h.kbd, key))
      ) {
        lines.push(`${kbd}\t${cmd.id}\t${dispatch}\t${pal}\t${help || '-'}`)
      }
      continue
    }
    for (const ch of cmd.chords) {
      const token = chordDump(ch)
      if (key && !dumpQueryHits(token, key) && !(ch.mod && dumpQueryHits(`mod+${ch.key}`, key))) continue
      lines.push(
        `${token}\t${cmd.id}\tscope=${cmd.scope ?? '-'}\t${dispatch}\t${pal}\t${help || '-'}`,
      )
    }
  }
  return lines.join('\n')
}
