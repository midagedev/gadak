/*
 * GDK-737: a read-path surface that paints a skeleton must go through the
 * shared grace owner. Boot already waited 120ms; every other surface drew
 * immediately. vitest is environment:'node' with no svelte plugin on the
 * unit project, so a .svelte file cannot be mounted (FeaturesTab.test.ts /
 * HistoryView.test.ts). What this file can prove is the class in the source
 * the compiler emits.
 *
 * The list is explicit — a new surface has to be added here on purpose,
 * not matched by a clever regex.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '..')
const OWNER = join(HERE, 'skeleton-grace.svelte.ts')

/** Read-path surfaces that must consume the shared grace. */
const READ_PATH: { name: string; file: string; comment: string }[] = [
  {
    name: 'boot',
    file: join(WEB_SRC, 'App.svelte'),
    comment: 'cold start; IndexedDB cache hit should paint nothing',
  },
  {
    name: 'history',
    file: join(WEB_SRC, 'components/history/HistoryView.svelte'),
    comment: 'GET history/ is a local.db read',
  },
  {
    name: 'feed',
    file: join(WEB_SRC, 'components/personal/PersonalFeed.svelte'),
    comment: 'GET feed/ is a local mirror read; sibling column of history',
  },
  {
    // GDK-827: the dashboard row fetch joined the column's read-path family
    // (loopback serve read; the frame paints the instant the row lands).
    name: 'dashboard',
    file: join(WEB_SRC, 'components/dashboard/DashboardView.svelte'),
    comment: 'GET dashboards/{id} is a loopback read; sibling column of history/feed',
  },
  {
    name: 'issue detail body',
    file: join(WEB_SRC, 'components/detail/DetailPanel.svelte'),
    comment: 'GET detail is loopback; header already paints from the pool',
  },
  {
    name: 'document body',
    file: join(WEB_SRC, 'components/detail/DocumentPanel.svelte'),
    comment: 'page detail is loopback; header paints from the index',
  },
  {
    name: 'person comments',
    file: join(WEB_SRC, 'components/detail/PersonPanel.svelte'),
    comment: 'author comments are a loopback mirror read; header is already on screen',
  },
  {
    name: 'terminal first attach',
    file: join(WEB_SRC, 'components/terminal/TerminalPane.svelte'),
    comment: 'POST session + socket open can take seconds; sibling of dashboard grace',
  },
]

/**
 * Surfaces that paint a loading state without the grace, with a one-line
 * reason. A write in flight is not a read; a network origin call is not
 * the mirror.
 */
const ALLOWLIST: { file: string; reason: string }[] = [
  {
    file: join(WEB_SRC, 'components/ui/LoadingState.svelte'),
    reason: 'shared visual; callers own the grace',
  },
  {
    file: join(WEB_SRC, 'components/shell/LoadingShell.svelte'),
    reason: 'boot visual only; App.svelte owns the grace',
  },
  {
    file: join(WEB_SRC, 'components/settings/SettingsDialog.svelte'),
    reason: 'settings GET is a dialog, not a mirror issue/page/feed/history read',
  },
  {
    file: join(WEB_SRC, 'components/settings/IntegrationsTab.svelte'),
    reason: 'integration status is a live origin check, not the mirror',
  },
  {
    file: join(WEB_SRC, 'components/settings/DevicesTab.svelte'),
    reason: 'pairing device list is an in-process store read, not the mirror',
  },
  {
    file: join(WEB_SRC, 'components/write/NewIssueDialog.svelte'),
    reason: 'write-path create-meta; a write in flight is not a read',
  },
]

const GRACE_DEF = /export const SKELETON_GRACE_MS\s*=\s*120\b/
const INLINE_BOOT_DELAY = /setTimeout\(\s*\(\)\s*=>\s*\(\s*showSkeleton\s*=\s*true\s*\)\s*,\s*120\s*\)/
const OWNER_IMPORT = /from\s+['"][^'"]*skeleton-grace\.svelte['"]/
const OWNER_CALL = /createSkeletonGrace\s*\(/

function read(path: string): string {
  return readFileSync(path, 'utf8')
}

function walkFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walkFiles(p, out)
    else if (name.endsWith('.ts') || name.endsWith('.svelte')) {
      if (name.endsWith('.test.ts')) continue
      out.push(p)
    }
  }
  return out
}

function rel(p: string): string {
  return relative(WEB_SRC, p).replaceAll('\\', '/')
}

/** A file paints a skeleton / LoadingState that the sweep must classify. */
function paintsLoading(src: string): boolean {
  if (src.includes('<LoadingState')) return true
  if (src.includes('<LoadingShell')) return true
  if (/<!--\s*Skeleton/.test(src)) return true
  if (/<!--\s*Same skeleton/.test(src)) return true
  return false
}

describe('GDK-737 read-path skeleton grace', () => {
  test('the 120 ms threshold has exactly one definition site', () => {
    const defs: string[] = []
    for (const abs of walkFiles(WEB_SRC)) {
      const src = read(abs)
      if (GRACE_DEF.test(src)) defs.push(rel(abs))
    }
    expect(defs, `SKELETON_GRACE_MS = 120 sites:\n${defs.join('\n')}`).toEqual([
      'lib/skeleton-grace.svelte.ts',
    ])
    expect(read(OWNER)).toMatch(GRACE_DEF)
  })

  test('boot no longer inlines the delay; every read-path surface reaches the owner', () => {
    const app = read(join(WEB_SRC, 'App.svelte'))
    expect(app, 'App.svelte still inlines setTimeout(..., 120) for the boot skeleton').not.toMatch(
      INLINE_BOOT_DELAY,
    )

    for (const surface of READ_PATH) {
      const src = read(surface.file)
      expect(src, `${surface.name}: does not import skeleton-grace.svelte`).toMatch(OWNER_IMPORT)
      expect(src, `${surface.name}: does not call createSkeletonGrace`).toMatch(OWNER_CALL)
      expect(src, `${surface.name}: never reads .visible, so a fast load still paints`).toMatch(
        /\.visible\b/,
      )
    }
  })

  test('a painted skeleton carries data-skeleton (locator / screenshot)', () => {
    // Boot has no persistent host during the grace (LoadingShell is unlisted
    // and unmounted while waiting). App.svelte publishes the same attribute
    // onto <html> via dataset.skeleton, the house idiom next to uiFocusPoll.
    const app = read(join(WEB_SRC, 'App.svelte'))
    expect(app, 'boot missing dataset.skeleton debug').toMatch(/dataset\.skeleton/)

    for (const surface of READ_PATH) {
      if (surface.name === 'boot') continue
      const src = read(surface.file)
      expect(src, `${surface.name}: missing data-skeleton=`).toMatch(/data-skeleton=/)
    }
  })

  test('every loading-state file is either a read-path consumer or allowlisted', () => {
    const readSet = new Set(READ_PATH.map((s) => s.file))
    const allowSet = new Set(ALLOWLIST.map((s) => s.file))
    const unmarked: string[] = []
    for (const abs of walkFiles(WEB_SRC)) {
      if (readSet.has(abs) || allowSet.has(abs)) continue
      if (paintsLoading(read(abs))) unmarked.push(rel(abs))
    }
    expect(
      unmarked.sort(),
      `loading-state files with no grace and no allowlist reason:\n${unmarked.join('\n')}`,
    ).toEqual([])
  })

  test('allowlist entries still exist (reasons stay attached to real files)', () => {
    for (const entry of ALLOWLIST) {
      expect(read(entry.file).length, `${rel(entry.file)}: empty or missing`).toBeGreaterThan(0)
    }
  })
})
