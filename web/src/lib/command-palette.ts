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
  openHistory: () => void
  openDocs: () => void
  toggleTerminal: () => void
  openFeed: () => void
  clearUserFilters: () => void
  toggleFlag: (flag: 'reopened' | 'unassigned' | 'stale') => void
  syncStatus: () => void
  syncNow: () => void
  createNow: (summary: string) => void
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
    case 'a:history':
      return host.openHistory
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
