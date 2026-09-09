/*
 * Settings copy contracts, moved from e2e/settings.spec.ts by the GDK-1702
 * cost ladder: three cases whose every assertion reads source or catalog
 * text that a browser run can only mirror back — each one cost a full app
 * boot in CI to assert a string. The FeaturesTab.test.ts idiom: scan the
 * source the compiler emits and the catalog the strings live in. What
 * stays in e2e is what needs the dialog open (Tabs rendering, runtime
 * mirror, API round-trips).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en } from '../../lib/i18n/en'

const HERE = dirname(fileURLToPath(import.meta.url))
const DIALOG = join(HERE, 'SettingsDialog.svelte')
const SOURCES = join(HERE, 'SourcesTab.svelte')
const SCOPE = join(HERE, 'ScopePicker.svelte')
const ABOUT = join(HERE, 'AboutTab.svelte')

describe('GDK-476: settings lead names the job, not a file path', () => {
  const dialog = readFileSync(DIALOG, 'utf8')

  test('the lead paragraph is the intro copy, not implementation vocabulary', () => {
    // Blocks: the lead drifting back to "edit your config.json" — the
    // audit's finding. The e2e asserted toHaveText(en['settings.intro'])
    // plus no 'config.json' / '~/.gadak'; the binding plus the catalog
    // sentence carry both halves here.
    expect(dialog).toMatch(/data-testid="settings-intro"[\s\S]{0,200}\{t\('settings\.intro'\)\}/)
    expect(en['settings.intro']).not.toMatch(/config\.json/)
    expect(en['settings.intro']).not.toMatch(/~\/\.gadak/)
  })
})

describe('empty project picker label includes every project', () => {
  const sources = readFileSync(SOURCES, 'utf8')
  const scope = readFileSync(SCOPE, 'utf8')

  test('an empty selection renders the every-project sentence, which says so', () => {
    // Blocks: the pre-fix catalog that said "no issue is mirrored" for an
    // empty selection — which mirrors EVERYTHING. The e2e route-stubbed
    // projects:[] and asserted the rendered label; the sentence and its
    // binding are the whole contract, and "every project" is the exact
    // phrase that case checked for.
    expect(sources).toMatch(/emptyLabel=\{t\('settings\.sourcesNoProjects'\)\}/)
    expect(scope).toMatch(/\{:else if emptyLabel\}[\s\S]{0,200}data-testid="scope-empty"[^>]*>\{emptyLabel\}/)
    expect(en['settings.sourcesNoProjects']).toContain('every project')
  })
})

describe('about tab lists the four feedback channels', () => {
  const about = readFileSync(ABOUT, 'utf8')

  test('github / issues / email / x hrefs and their testids stay paired', () => {
    // Blocks: a channel silently pointing at the wrong URL (or losing its
    // testid, which is how anything downstream finds it). The e2e asserted
    // toHaveAttribute('href', …) on each of the four — the constants are
    // where those hrefs live.
    const pairs: [string, string][] = [
      ['github', 'https://github.com/midagedev/gadak'],
      ['issues', 'https://github.com/midagedev/gadak/issues'],
      ['email', 'mailto:midagedev@gmail.com'],
      ['x', 'https://x.com/midagedev'],
    ]
    for (const [id, href] of pairs) {
      expect(about, `about-link-${id} href`).toMatch(new RegExp(`href=\\{(${id.toUpperCase()})\\}`))
      expect(about, `the ${id.toUpperCase()} constant`).toMatch(
        new RegExp(`const ${id.toUpperCase()} = '${href.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}'`),
      )
      expect(about).toMatch(new RegExp(`data-testid="about-link-${id}"`))
    }
  })
})
