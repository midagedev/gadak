/*
 * GDK-316: the six same-class modal dialogs must take their visual chrome
 * from DialogShell.svelte. A seventh that copy-pastes the backdrop or the
 * anim-pop/enter panel + header X will fail here before e2e/dialog-shell
 * has a row for it.
 *
 * vitest unit cannot import .svelte (Onboarding.test.ts). This reads source.
 *
 * CommandPalette and MediaViewer are a different class (e2e/dialog-shell
 * spec comment): the palette has the backdrop tint but no closeEsc header X;
 * MediaViewer is bg-black/90. Both must stay off this gate.
 */
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const COMPONENTS = join(HERE, '..')
const SHELL = join(HERE, 'DialogShell.svelte')

/** Unique fill/opacity of this dialog class — MediaViewer is bg-black/90. */
const BACKDROP_TINT = 'bg-[#1c1812]/28'
/** Header X accessible name this class shares. */
const CLOSE_ESC = 'common.closeEsc'
/** Panel chrome this class shares — dropdowns use bg-bg-elevated. */
const PANEL = 'rounded-lg border border-border-strong bg-bg-panel shadow-overlay'

function svelteFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...svelteFiles(p))
    else if (name.endsWith('.svelte')) out.push(p)
  }
  return out
}

function rel(p: string): string {
  return relative(COMPONENTS, p)
}

describe('GDK-316 DialogShell is the only dialog-class visual shell', () => {
  const files = svelteFiles(COMPONENTS).filter((p) => p !== SHELL)

  test('backdrop tint + closeEsc does not appear outside DialogShell', () => {
    const hits = files.filter((p) => {
      const src = readFileSync(p, 'utf8')
      return src.includes(BACKDROP_TINT) && src.includes(CLOSE_ESC)
    })
    expect(
      hits.map(rel),
      `dialog-class backdrop copied in: ${hits.map(rel).join(', ') || '(none)'}`,
    ).toEqual([])
  })

  test('anim-pop/enter panel + closeEsc does not appear outside DialogShell', () => {
    const hits = files.filter((p) => {
      const src = readFileSync(p, 'utf8')
      if (!src.includes(CLOSE_ESC) || !src.includes(PANEL)) return false
      return src.includes('anim-pop') || src.includes('anim-enter')
    })
    expect(
      hits.map(rel),
      `dialog-class panel copied in: ${hits.map(rel).join(', ') || '(none)'}`,
    ).toEqual([])
  })

  test('DialogShell owns the backdrop, panel, and header-X tokens', () => {
    expect(existsSync(SHELL), 'web/src/components/ui/DialogShell.svelte must exist').toBe(true)
    const src = readFileSync(SHELL, 'utf8')
    expect(src).toContain(BACKDROP_TINT)
    expect(src).toContain(CLOSE_ESC)
    expect(src).toContain(PANEL)
    expect(src).toContain('data-dialog-footer')
    expect(src).toContain("name=\"x\"")
    expect(src).toContain('h-control-sm')
  })
})
