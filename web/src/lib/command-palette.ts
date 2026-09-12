/*
 * Palette action rows derived from COMMANDS. Store calls stay in the
 * host the palette passes in — this file must not import stores (keymap
 * tests load commands.ts; they must not pull the store graph).
 */

import { locale, setLocale, t } from './i18n'
import type { Locale, MessageKey } from './i18n/catalog'
import { LOCALES } from './i18n/types'
import { persistThemePreference, THEME_MODES } from './theme'
import { originTrackerName } from './config'
import { issueOriginUrl } from './issue-origin'
import { COMMANDS, type PaletteSpec, type TriageMenuKey } from './commands'
import type { DraggableLayoutAxis } from './viewport-regime'

export interface PaletteActionHost {
  requestMenu: (menu: TriageMenuKey) => void
  openComment: (key: string) => void
  toggleBulk: (key: string) => void
  clearBulk: () => void
  toggleFavorite: (key: string) => void
  toggleWatch: (key: string) => void
  openPageOrigin: () => void
  openIssueOrigin: (key: string) => void
  openNewIssue: () => void
  openSettings: () => void
  copyViewLink: () => void
  openHistory: () => void
  openRetro: () => void
  openDocs: () => void
  toggleTerminal: () => void
  openFeed: () => void
  clearUserFilters: () => void
  toggleFlag: (flag: 'reopened' | 'unassigned' | 'stale') => void
  syncStatus: () => void
  syncNow: () => void
  createNow: (summary: string) => void
  showIssueList: () => void
  copyIssueLink: (key: string) => void
  markAllFeedRead: () => void
  saveView: () => void
  /**
   * Put the keyboard on `axis`'s layout grip (GDK-1796). Focus-shaped on
   * purpose: the grip's own key handler then does the resizing, which a
   * click on it would not.
   */
  focusResizeGrip: (axis: DraggableLayoutAxis) => void
}

export interface PaletteActionInput {
  bulkCount: number
  bulkHasCursor: boolean
  cursor: string | null
  pageKey: string | null
  issueKey: string | null
  identified: boolean
  hostedDemo: boolean
  feedEnabled: boolean
  /** Unread feed items — the "Mark all read" row has nothing to do at 0. */
  feedUnread: number
  /** The column is on the issue list, so "save this view" has a view. */
  onIssueList: boolean
  /**
   * The viewport is in the docked regime — the only one that paints the two
   * layout grips (App.svelte). Off it the resize rows have no target, and a
   * row that cannot act is a promise the palette should not show.
   */
  docked: boolean
  /**
   * The terminal is open AND is a dock rather than an overlay sheet — the
   * only state that paints the dock's grip (GDK-1815). A separate field from
   * `docked` because the two thresholds are genuinely different: between 900
   * and 1099px with no docked detail panel the terminal is a dock while the
   * layout is already overlay, so `docked` would hide a row that works and
   * `!docked` would show one that cannot.
   */
  terminalDocked: boolean
  query: string
  favoriteHas: (key: string) => boolean
  watchHas: (key: string) => boolean
  host: PaletteActionHost
}

export interface PaletteActionItem {
  id: string
  label: string
  kbd?: string
  testid?: string
  stayOpen?: boolean
  run: () => void
}

const LOCALE_LABEL: Record<Locale, MessageKey> = {
  en: 'settings.localeEn',
  ko: 'settings.localeKo',
  ja: 'settings.localeJa',
}

function triageTarget(input: PaletteActionInput): string {
  return input.bulkCount ? t('palette.triageSelected', { n: input.bulkCount }) : (input.cursor as string)
}

function runAlways(id: string, host: PaletteActionHost): () => void {
  switch (id) {
    case 'a:settings':
      return host.openSettings
    case 'a:copy-view-link':
      return host.copyViewLink
    case 'a:history':
      return host.openHistory
    case 'a:retro':
      return host.openRetro
    case 'a:docs':
      return host.openDocs
    case 'a:terminal':
      return host.toggleTerminal
    case 'a:reset':
      return host.clearUserFilters
    case 'a:sync':
      return host.syncStatus
    case 'a:sync-now':
      return host.syncNow
    default:
      return () => {}
  }
}

function itemsFor(spec: PaletteSpec, input: PaletteActionInput): PaletteActionItem[] {
  const { host } = input
  switch (spec.kind) {
    case 'triage-menu': {
      if (!input.bulkCount && !input.cursor) return []
      const menu = spec.menu
      if (!menu) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey, { target: triageTarget(input) }),
          kbd: spec.kbd,
          run: () => host.requestMenu(menu),
        },
      ]
    }
    case 'triage-comment': {
      const cursor = input.cursor
      if (!cursor) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey, { key: cursor }),
          kbd: spec.kbd,
          run: () => host.openComment(cursor),
        },
      ]
    }
    case 'triage-select': {
      const cursor = input.cursor
      if (!cursor) return []
      const on = input.bulkHasCursor
      return [
        {
          id: spec.id,
          label: t(on && spec.altLabelKey ? spec.altLabelKey : spec.labelKey, { key: cursor }),
          kbd: spec.kbd,
          run: () => host.toggleBulk(cursor),
        },
      ]
    }
    case 'favorite': {
      const cursor = input.cursor
      if (!cursor) return []
      const on = input.favoriteHas(cursor)
      return [
        {
          id: spec.id,
          label: t(on && spec.altLabelKey ? spec.altLabelKey : spec.labelKey, { key: cursor }),
          testid: spec.testid,
          run: () => host.toggleFavorite(cursor),
        },
      ]
    }
    case 'watch': {
      const cursor = input.cursor
      if (!cursor || !input.identified || input.hostedDemo) return []
      const on = input.watchHas(cursor)
      return [
        {
          id: spec.id,
          label: t(on && spec.altLabelKey ? spec.altLabelKey : spec.labelKey, { key: cursor }),
          testid: spec.testid,
          run: () => host.toggleWatch(cursor),
        },
      ]
    }
    case 'triage-clear': {
      if (!input.bulkCount) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey, { n: input.bulkCount }),
          kbd: spec.kbd,
          run: () => host.clearBulk(),
        },
      ]
    }
    case 'origin': {
      if (input.pageKey) {
        return [
          {
            id: spec.id,
            label: t('doc.openSource'),
            kbd: spec.kbd,
            run: () => host.openPageOrigin(),
          },
        ]
      }
      if (!input.issueKey) return []
      const key = input.issueKey
      // GDK-1313: the row promises an action, so it needs the page, not just
      // a key — on the built-in tracker (and a Linear row with no stored
      // url) `openIssueOrigin` is a no-op, and the 400-toast hatch already
      // gates on the same resolver (stores/write.svelte.ts).
      if (!issueOriginUrl(key)) return []
      return [
        {
          id: spec.id,
          label: t('detail.openJira', { tracker: originTrackerName() }),
          kbd: spec.kbd,
          run: () => host.openIssueOrigin(key),
        },
      ]
    }
    case 'new-issue': {
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          kbd: input.cursor ? undefined : spec.kbd,
          testid: spec.testid,
          run: () => host.openNewIssue(),
        },
      ]
    }
    case 'always':
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          kbd: spec.kbd,
          testid: spec.testid,
          run: runAlways(spec.id, host),
        },
      ]
    case 'feed': {
      if (!input.feedEnabled) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          testid: spec.testid,
          run: () => host.openFeed(),
        },
      ]
    }
    case 'toggle-flag': {
      const flag = spec.flag
      if (!flag) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          run: () => host.toggleFlag(flag),
        },
      ]
    }
    case 'locales': {
      const current = locale()
      return LOCALES.filter((code) => code !== current).map((code) => ({
        id: `a:locale-${code}`,
        label: t(spec.labelKey, { lang: t(LOCALE_LABEL[code]) }),
        run: () => setLocale(code),
      }))
    }
    case 'themes':
      return THEME_MODES.map((mode) => ({
        id: `a:theme-${mode.name}`,
        label: t(spec.labelKey, { mode: t(mode.labelKey) }),
        run: () => void persistThemePreference(mode.name),
      }))
    case 'issue-list': {
      // Nothing to go back to while the list already holds the column — the
      // row would be a no-op wearing an action's clothes.
      if (input.onIssueList) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          testid: spec.testid,
          run: () => host.showIssueList(),
        },
      ]
    }
    case 'issue-link': {
      const key = input.issueKey
      if (!key) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey, { key }),
          testid: spec.testid,
          run: () => host.copyIssueLink(key),
        },
      ]
    }
    case 'feed-read-all': {
      // Same three conditions the feed header's button paints under: the
      // build has a feed, this reader is identified, and something is unread.
      if (!input.feedEnabled || !input.identified || input.feedUnread === 0) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          testid: spec.testid,
          run: () => host.markAllFeedRead(),
        },
      ]
    }
    case 'save-view': {
      // A hand-off row, not a query-driven one: naming belongs to the save
      // popover (see components/list/save-view-request). Off the list there
      // is no current view to save.
      if (!input.onIssueList) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          testid: spec.testid,
          run: () => host.saveView(),
        },
      ]
    }
    case 'create-now': {
      const raw = input.query.trim()
      if (!raw) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey, { summary: raw }),
          testid: spec.testid,
          stayOpen: true,
          run: () => host.createNow(raw),
        },
      ]
    }
    case 'focus-resize': {
      const axis = spec.axis
      // Same rule as issue-list/save-view: a grip that is not mounted cannot
      // be focused, and a row that cannot reach its target is a no-op
      // wearing an action's clothes. The column grips are mounted by the
      // docked regime (App.svelte); the dock's is mounted with the terminal
      // pane, in its split rather than its overlay form (TerminalPane).
      if (!axis) return []
      if (!(axis === 'terminal' ? input.terminalDocked : input.docked)) return []
      return [
        {
          id: spec.id,
          label: t(spec.labelKey),
          testid: spec.testid,
          run: () => host.focusResizeGrip(axis),
        },
      ]
    }
  }
}

export function paletteActionItems(input: PaletteActionInput): PaletteActionItem[] {
  const specs = COMMANDS.map((c) => c.palette)
    .filter((p): p is PaletteSpec => p != null)
    .sort((a, b) => a.sort - b.sort)
  const out: PaletteActionItem[] = []
  for (const spec of specs) out.push(...itemsFor(spec, input))
  return out
}
