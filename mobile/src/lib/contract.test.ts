import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// Recurrence layer for GDK-870 / GDK-879: the viewport gate cannot grow
// (its spec file is only allowed to unskip the 44pt test). These read the
// source so a later round cannot silently put the status action back in
// the header, reorder Detail to description-first, or paint Unpair with
// a status token.

const src = join(dirname(fileURLToPath(import.meta.url)), '..')

function read(rel: string): string {
  return readFileSync(join(src, rel), 'utf8')
}

/** Markup only — comments that name a ban are not the ban. */
function markup(rel: string): string {
  return read(rel)
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

describe('GDK-870 Detail contracts', () => {
  const detail = read('screens/Detail.svelte')

  it('renders comments before description', () => {
    // Order, not language: DESIGN.md §3.6. Was pinned to English markup
    // (`<h3>Comments` / `<h3>Description`); a Korean catalog then made the
    // contract unenforceable. Keys survive translation.
    const comments = detail.indexOf("t('detail.comments')")
    const desc = detail.indexOf("t('detail.description')")
    expect(comments).toBeGreaterThan(-1)
    expect(desc).toBeGreaterThan(-1)
    expect(comments).toBeLessThan(desc)
  })

  it('opens the transition sheet from the composer, not the header chips', () => {
    const chips = detail.indexOf('class="chips"')
    const composer = detail.indexOf('composer-slab')
    const statusBtn = detail.indexOf('class="status"')
    expect(chips).toBeGreaterThan(-1)
    expect(composer).toBeGreaterThan(chips)
    expect(statusBtn).toBeGreaterThan(composer)
  })
})

describe('GDK-906 Detail F2 — one control, catalog copy, honest empty', () => {
  const detail = read('screens/Detail.svelte')
  const detailMarkup = markup('screens/Detail.svelte')
  const page = markup('screens/PageDetail.svelte')

  it('keeps the header status chip as data and demotes priority/assignee to meta', () => {
    const chipsAt = detailMarkup.indexOf('class="chips"')
    expect(chipsAt).toBeGreaterThan(-1)
    const chipsEnd = detailMarkup.indexOf('</div>', chipsAt)
    const chips = detailMarkup.slice(chipsAt, chipsEnd)
    expect(chips).toMatch(/lite\.status/)
    expect(chips).not.toMatch(/<button/)
    expect(chips).not.toMatch(/openTransitions/)
    expect(chips).not.toMatch(/lite\.priority/)
    expect(chips).not.toMatch(/lite\.assignee/)
    const metaAt = detailMarkup.indexOf('class="meta"')
    expect(metaAt).toBeGreaterThan(chipsAt)
    const meta = detailMarkup.slice(metaAt, detailMarkup.indexOf('</p>', metaAt))
    expect(meta).toMatch(/lite\.priority/)
    expect(meta).toMatch(/lite\.assignee/)
  })

  it('makes the composer status button the only transition control', () => {
    const composer = detailMarkup.indexOf('composer-slab')
    const click = detailMarkup.indexOf('onclick={openTransitions}')
    expect(composer).toBeGreaterThan(-1)
    expect(click).toBeGreaterThan(composer)
    expect(detailMarkup.indexOf('onclick={openTransitions}', click + 1)).toBe(-1)
  })

  it('does not open the transition sheet when the serve has no origin credential', () => {
    expect(detail).toMatch(/credential_required/)
    expect(detail).toMatch(/writesOff/)
    expect(detail).toMatch(/disabled=\{writesOff\}/)
    const openFn = detail.slice(
      detail.indexOf('async function openTransitions'),
      detail.indexOf('async function applyTransition'),
    )
    expect(openFn).toMatch(/if \(writesOff\) return/)
  })

  it('uses catalog keys on the issue detail sections, not English literals', () => {
    expect(detailMarkup).toContain("t('detail.comments')")
    expect(detailMarkup).toContain("t('detail.description')")
    expect(detailMarkup).toContain("t('detail.noDescription')")
    expect(detailMarkup).toContain("t('detail.linked')")
    expect(detailMarkup).toContain("t('detail.unknownAuthor')")
    expect(detailMarkup).not.toMatch(/<h3>Comments/)
    expect(detailMarkup).not.toMatch(/<h3>Description/)
    expect(detailMarkup).not.toMatch(/<h3>Linked/)
    expect(detailMarkup).not.toMatch(/>No description\.</)
    expect(detailMarkup).not.toMatch(/c\.author \?\? 'Unknown'/)
  })

  it('maps a missing issue to detail.notFound on this side of api.ts', () => {
    expect(detail).toContain("t('detail.notFound')")
    expect(detail).toMatch(/code === 'not_found'/)
  })

  it('reports a refused comment inside the composer control, not above the slab', () => {
    const slab = detail.indexOf('composer-slab')
    const composer = detail.indexOf('class="composer', slab)
    const sendErr = detail.indexOf('{sendError}', slab)
    const statusBtn = detail.indexOf('class="status"', slab)
    expect(composer).toBeGreaterThan(slab)
    expect(sendErr).toBeGreaterThan(composer)
    expect(statusBtn).toBeGreaterThan(slab)
    expect(statusBtn).toBeLessThan(composer)
    expect(sendErr).toBeGreaterThan(statusBtn)
  })

  it('reports a refused transition on the row that acted', () => {
    const row = detail.indexOf('class="t-row"')
    expect(row).toBeGreaterThan(-1)
    const after = detail.slice(row)
    const rowEnd = after.indexOf('</button>')
    expect(after.slice(0, rowEnd)).toMatch(/transitionError/)
  })

  it('paints an empty page body with doc.noContent instead of a hole', () => {
    expect(page).toContain("t('doc.noContent')")
    const body = page.indexOf('paragraphs')
    const empty = page.indexOf("t('doc.noContent')")
    const comments = page.indexOf("t('doc.comments')")
    expect(body).toBeGreaterThan(-1)
    expect(empty).toBeGreaterThan(body)
    expect(comments).toBeGreaterThan(empty)
  })
})

describe('GDK-879 pairing / spine contracts', () => {
  it('does not borrow a status token for Unpair', () => {
    const pairing = read('screens/PairingTab.svelte')
    const styles = pairing.slice(pairing.indexOf('<style>'))
    const unpair = styles.match(/\.unpair\s*\{[^}]+\}/)
    const armed = styles.match(/\.unpair\.armed\s*\{[^}]+\}/)
    expect(unpair?.[0]).toBeTruthy()
    expect(armed?.[0]).toBeTruthy()
    expect(unpair?.[0]).not.toMatch(/--color-status-/)
    expect(armed?.[0]).not.toMatch(/--color-status-/)
  })

  it('maps the new spine through --color-spine-new, not the raw status token', () => {
    const css = read('app.css')
    expect(css).toMatch(/--color-spine-new:\s*var\(--color-accent\)/)
    expect(read('ui/Row.svelte')).toMatch(/var\(--color-spine-new\)/)
    expect(read('screens/Detail.svelte')).toMatch(/var\(--color-spine-new\)/)
  })
})

describe('GDK-887 document rows and page detail', () => {
  it('paints document rows without an ink spine', () => {
    const row = markup('ui/DocRow.svelte')
    expect(row).not.toMatch(/class="spine/)
    expect(row).not.toMatch(/\.spine/)
    expect(row).not.toMatch(/status_category/)
  })

  it('puts comments after the page body and has no composer', () => {
    const page = markup('screens/PageDetail.svelte')
    const body = page.indexOf('paragraphs')
    const comments = page.indexOf("t('doc.comments')")
    expect(body).toBeGreaterThan(-1)
    expect(comments).toBeGreaterThan(body)
    expect(page).not.toMatch(/composer/)
    expect(page).not.toMatch(/<input/)
    expect(page).not.toMatch(/method:\s*['"]POST/)
  })

  it('search paints pages in a Documents section below issues', () => {
    const search = read('screens/Search.svelte')
    expect(search).toContain('serverPages')
    expect(search).toContain("t('sidebar.docs')")
    expect(search).toContain('DocRow')
  })
})

describe('GDK-867 tap floor owner', () => {
  it('sets the 44pt floor on button in app.css', () => {
    expect(read('app.css')).toMatch(/button\s*\{[^}]*min-height:\s*var\(--spacing-control\)/s)
  })

  it('does not use --spacing-control-sm as a button tap size', () => {
    const files = [
      'screens/Issues.svelte',
      'screens/Search.svelte',
      'screens/Detail.svelte',
      'screens/PageDetail.svelte',
      'screens/PairingTab.svelte',
      'ui/Sheet.svelte',
      'ui/ScopeSheet.svelte',
      'ui/DocRow.svelte',
    ]
    for (const rel of files) {
      const text = read(rel)
      for (const m of text.matchAll(/\.([a-z0-9-]+)[^{]*\{[^}]*min-height:\s*var\(--spacing-control-sm\)/gi)) {
        const cls = m[1]
        expect(text, `${rel} .${cls} is a button using the visual-chip token`).not.toMatch(
          new RegExp(`<button[^>]*class="${cls}"`),
        )
      }
    }
  })
})

describe('GDK-933 writes-off is one surface', () => {
  const detail = read('screens/Detail.svelte')
  const styles = detail.slice(detail.indexOf('<style>'))

  it('disables the comment field when writes are off', () => {
    const input = detail.slice(detail.indexOf('<input'), detail.indexOf('placeholder="Comment…"'))
    expect(input).toMatch(/disabled=\{writesOff\}/)
  })

  it('disables Send when writes are off, not only when the field is empty', () => {
    const send = detail.slice(detail.indexOf('class="send"'), detail.indexOf('class="send"') + 280)
    expect(send).toMatch(/disabled=\{[^}]*writesOff/)
  })

  it('fades the composer with the same opacity the status row already uses', () => {
    expect(detail).toMatch(/class:off=\{writesOff\}/)
    expect(styles).toMatch(/\.status:disabled\s*\{[^}]*opacity:\s*0\.45/)
    expect(styles).toMatch(/\.composer\.off\s*\{[^}]*opacity:\s*0\.45/)
  })

  it('keeps the one writes-off sentence on the status row — Send does not repeat it', () => {
    const sendHead = detail.slice(detail.indexOf('async function send()'), detail.indexOf('sending = true'))
    expect(sendHead).toMatch(/writesOff/)
    const slab = detail.indexOf('composer-slab')
    const sendErrAt = detail.indexOf('{#if sendError', slab)
    expect(sendErrAt).toBeGreaterThan(slab)
    expect(detail.slice(sendErrAt, sendErrAt + 80)).toMatch(/sendError && !writesOff/)
  })
})

describe('GDK-934 resting Send is not the accent thread', () => {
  const detail = read('screens/Detail.svelte')
  const styles = detail.slice(detail.indexOf('<style>'))

  it('wears the accent fill only when Send is armed, not while disabled', () => {
    expect(detail).toMatch(/class:armed=\{sendArmed\}/)
    const sendBlock = styles.match(/\.send\s*\{[^}]+\}/)?.[0]
    expect(sendBlock).toBeTruthy()
    expect(sendBlock).not.toMatch(/--color-accent/)
    const armed = styles.match(/\.send\.armed\s*\{[^}]+\}/)?.[0]
    expect(armed).toBeTruthy()
    expect(armed).toMatch(/background:\s*var\(--color-accent\)/)
  })

  it('does not recede a disabled Send by fading the accent fill', () => {
    const disabled = styles.match(/\.send:disabled\s*\{[^}]+\}/)?.[0] ?? ''
    expect(disabled).not.toMatch(/opacity/)
  })
})

describe('GDK-935 row folio is one grammar', () => {
  it('dates the right-hand folio with folioDate, not relTime', () => {
    const row = read('ui/Row.svelte')
    expect(row).toMatch(/folioDate\(issue\.updated_at/)
    expect(row).not.toMatch(/relTime\(issue\.updated_at/)
  })
})
