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
    // GDK-1497: the body branch is hasBody + AdfBody (was `paragraphs`).
    const body = page.indexOf('hasBody')
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
    // GDK-1497: the body is AdfBody now (was `paragraphs`).
    const body = page.indexOf('AdfBody')
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

describe('GDK-1497 A2 — the header is a control surface', () => {
  const detail = read('screens/Detail.svelte')

  it("wires the assignee sheet's Unassigned row through setAssignee(issueKey, null)", () => {
    // The wire body {"account_id":null} is pinned in writes.test.ts against
    // a fake transport; this pins the other half of the chain — that the
    // row the person taps hands pickAssignee a literal null, and that the
    // picker's one exit is the typed wrapper (a row that looked clearing
    // but PUTs the current id would pass every transport test).
    const at = detail.indexOf('async function pickAssignee')
    expect(at).toBeGreaterThan(-1)
    const pickFn = detail.slice(at, detail.indexOf('\n  }', at))
    expect(pickFn).toMatch(/setAssignee\(issueKey, accountId\)/)
    expect(detail).toMatch(/void pickAssignee\(null\)/)
    expect(detail).toMatch(/t\('common\.unassigned'\)/)
  })

  it('clears priority through the None row and marks the current one', () => {
    const at = detail.indexOf('async function pickPriority')
    expect(at).toBeGreaterThan(-1)
    expect(detail.slice(at, detail.indexOf('\n  }', at))).toMatch(/setPriority\(issueKey, priorityId\)/)
    expect(detail).toMatch(/void pickPriority\(null\)/)
    expect(detail).toMatch(/t\('common\.none'\)/)
    expect(detail).toMatch(/lite\.priority_id === p\.id/)
  })

  it('keeps the writes-off degradation on every new control, like the composer', () => {
    // One owner per screen (GDK-933): the sticky flag disables the header
    // controls too, and no control bypasses it.
    for (const fn of ['openAssignee', 'openPriority', 'editSummary', 'editDescription']) {
      const at = detail.indexOf(`function ${fn}`)
      expect(detail.slice(at, detail.indexOf('\n  }', at))).toMatch(/writesOff/)
    }
  })

  // GDK-1497 A2 vision round (2026-09-07): the judge could not tell which
  // row was current. The priority sheet's mark was a leading glyph the
  // fixture never triggers, and the assignee sheet had none at all — only
  // an accent tint, which is Cancel's and every link's colour on this
  // screen, so it read as "actionable", not "current".
  const detailMarkupOnly = markup('screens/Detail.svelte')
  const sheetOf = (openFlag: string): string => {
    const at = detailMarkupOnly.indexOf(`{#if ${openFlag}}`)
    expect(at).toBeGreaterThan(-1)
    return detailMarkupOnly.slice(at, detailMarkupOnly.indexOf('{/if}\n\n', at))
  }

  it('marks the current row with a shape, not with the link colour', () => {
    const styles = detail.slice(detail.indexOf('<style>'))
    // The tint was the only marker; a tick that a colour rule can outvote
    // is not a marker.
    expect(styles).not.toMatch(/\.t-row\.current\s+\.t-name\s*\{/)
    expect(detailMarkupOnly).toMatch(/\{#snippet currentTick\(\)\}/)
    for (const flag of ['assigneeOpen', 'priorityOpen']) {
      const sheet = sheetOf(flag)
      // Both sheets render the one snippet — no second, divergent glyph.
      expect(sheet.match(/@render currentTick\(\)/g)?.length).toBe(2)
      // …and every render sits after its row's text, i.e. on the trailing
      // edge. A leading tick shifts the label and reads as a bullet.
      for (const row of sheet.split('<button').slice(1)) {
        const tick = row.indexOf('@render currentTick()')
        if (tick === -1) continue
        expect(tick).toBeGreaterThan(row.indexOf('class="t-text"'))
      }
    }
  })

  it('marks the clearing row when the issue carries no value', () => {
    // Unassigned / None are rows like any other: if that is the current
    // value, the sheet says so.
    expect(detail).toMatch(/const unassignedNow = \$derived\(!lite\?\.assignee_id\)/)
    expect(detail).toMatch(/const priorityNone = \$derived\(/)
    expect(sheetOf('assigneeOpen')).toMatch(/class:current=\{unassignedNow\}/)
    expect(sheetOf('priorityOpen')).toMatch(/class:current=\{priorityNone\}/)
  })

  it('puts the assignee search above the rows it filters', () => {
    const sheet = sheetOf('assigneeOpen')
    expect(sheet).toContain('class="search"')
    expect(sheet.indexOf('class="search"')).toBeLessThan(sheet.indexOf('class="t-row"'))
  })

  it('leaves one Cancel on the description sheet and arms its Save', () => {
    // The Sheet header owns Cancel; the action row owns the primary action
    // only, wearing the composer's armed fill (GDK-934 tokens, no new
    // colours). The format_loss branch keeps its own Cancel — that one
    // backs out of the replace prompt, not out of the sheet.
    const sheet = sheetOf('descOpen')
    const rest = sheet.slice(sheet.indexOf('{:else}'))
    expect(rest).toMatch(/class="save" class:armed=/)
    expect(rest).not.toContain('class="ghost"')
    const styles = detail.slice(detail.indexOf('<style>'))
    expect(styles.match(/\.save\.armed\s*\{[^}]+\}/)?.[0]).toMatch(
      /background:\s*var\(--color-accent\)/,
    )
  })

  it('does not give a loading placeholder the same class as a button', () => {
    // .ghost was both the text-button rule and the skeleton wrapper; the
    // later wrapper rule silently overrode display/padding on every Cancel.
    const styles = detail.slice(detail.indexOf('<style>'))
    expect(styles.match(/^\s*\.ghost\s*\{/gm)?.length).toBe(1)
    expect(detailMarkupOnly).not.toMatch(/<div class="ghost"/)
  })

  it('routes header writes through the typed wrappers, not request bodies', () => {
    // Screens hold no request bodies (the wrappers own them); Detail only
    // picks values and paints the returned issue.
    expect(detail).not.toMatch(/method:\s*['"]PUT/)
  })
})

describe('GDK-1497 A2 — the create sheet on the Issues screen', () => {
  const issues = read('screens/Issues.svelte')

  it('posts through createIssue and lands on the new issue', () => {
    const at = issues.indexOf('async function createTheIssue')
    expect(at).toBeGreaterThan(-1)
    const fn = issues.slice(at, issues.indexOf('\n  }', at))
    expect(fn).toMatch(/createIssue\(/)
    expect(fn).toMatch(/openIssue\(res\.issue\.issue_key\)/)
  })

  it('asks for a project only when the serve offers more than one', () => {
    expect(issues).toMatch(/creatableProjects\.length > 1/)
    expect(issues).toMatch(/t\('common\.project'\)/)
  })
})

describe('GDK-1495 A4 — the 0.21 concepts are the desktop’s rules, imported', () => {
  const domain = read('lib/domain.ts')
  const row = markup('ui/Row.svelte')
  const issues = markup('screens/Issues.svelte')
  const detail = markup('screens/Detail.svelte')

  it('imports every awareness rule from its desktop owner, and re-spells none', () => {
    // The seam, as a gate: a later round that finds the import awkward and
    // types the rule out by hand is the failure this catches. Each pair is
    // (symbol, owning module) — the phone may adapt the call, never the rule.
    for (const [symbol, owner] of [
      ['workAge', 'lib/view-config'],
      ['isStale', 'lib/view-config'],
      ['staleThresholdHoursEffective', 'lib/view-config'],
      ['setStaleFlowSource', 'lib/view-config'],
      ['relatchBoundary', 'lib/session-strip'],
      ['changedSince', 'lib/session-strip'],
      ['stripLabel', 'lib/session-strip'],
      ['pickSince', 'lib/resume-card'],
      ['resumeDelta', 'lib/resume-card'],
      ['resumeLabel', 'lib/resume-card'],
      ['builtinViews', 'lib/builtin-views'],
    ] as const) {
      const at = domain.indexOf(symbol)
      expect(at, `domain.ts does not mention ${symbol}`).toBeGreaterThan(-1)
      expect(domain, `${symbol} is not imported from web/src/${owner}`).toMatch(
        new RegExp(`import[^]{0,400}\\b${symbol}\\b[^]{0,400}web/src/${owner.replace('/', '\\/')}'`),
      )
    }
    // The two numbers a copy would have to spell out: the session gap and
    // the p85 sample bar. Neither may appear as a literal on this side.
    expect(domain).not.toMatch(/30\s*\*\s*60\s*\*\s*1000/)
    expect(domain).not.toMatch(/CycleP85MinSamples|FLOW_MIN_SAMPLES/)
  })

  it('② the row wears the work-item age as data, never as a control', () => {
    expect(row).toMatch(/rowIsStale\(issue\)/)
    expect(row).toMatch(/rowAgeDays\(issue\)/)
    expect(row).toMatch(/t\('list\.staleDaysShort'/)
    // GDK-906: the header chip is data. So is this one — a span with a
    // title, not a button that filters.
    const at = row.indexOf('class="age"')
    expect(at).toBeGreaterThan(-1)
    expect(row.slice(row.lastIndexOf('<', at), at)).toBe('<span ')
  })

  it('① the session strip is the list’s first line, above the glance strip', () => {
    const strip = issues.indexOf('data-testid="session-strip"')
    const glance = issues.indexOf('<GlanceStrip')
    expect(strip).toBeGreaterThan(-1)
    expect(glance).toBeGreaterThan(-1)
    expect(strip).toBeLessThan(glance)
    expect(issues).toMatch(/sessionLine\(/)
  })

  it('① the latch has one owner, and one visibility listener feeds it', () => {
    const store = read('lib/store.svelte.ts')
    // The away-clock and the re-latch live where the app already hears it
    // leave and return; a second visibilitychange listener for the strip is
    // the drift this catches.
    expect(store.match(/addEventListener\('visibilitychange'/g)?.length).toBe(1)
    expect(store).toMatch(/visibilityState === 'hidden'/)
    expect(store).toMatch(/relatchBoundary\(hiddenAtMs, Date\.now\(\)\)/)
    // The count is a snapshot: one guard, set before it is filled, so a
    // later sync cannot grow it.
    const at = store.indexOf('function latchSession')
    expect(at).toBeGreaterThan(-1)
    const fn = store.slice(at, store.indexOf('\n}', at))
    expect(fn).toMatch(/if \(app\.session\.computed\) return/)
    expect(fn.indexOf('app.session.computed = true')).toBeLessThan(fn.indexOf('app.session.delta ='))
    // Leaving a host takes its boundary and its threshold with it.
    const reset = store.slice(store.indexOf('function resetSessionState'))
    expect(reset.slice(0, reset.indexOf('\n}'))).toMatch(/app\.session = \{/)
  })

  it('③ the resume card sits above the comments, and dismisses locally', () => {
    const card = detail.indexOf('data-testid="resume-card"')
    const comments = detail.indexOf("t('detail.comments')")
    expect(card).toBeGreaterThan(-1)
    expect(card).toBeLessThan(comments)
    expect(detail).toMatch(/resumeDismissed/)
    // No fetch of its own: the card rides the detail response already loaded.
    const script = read('screens/Detail.svelte')
    const at = script.indexOf('const resumeSinceAt')
    expect(at).toBeGreaterThan(-1)
    expect(script.slice(at, at + 600)).not.toMatch(/request\(|fetch\(/)
  })
})

describe('GDK-1495 A4 vision FIX — the five points the blind judge sent back', () => {
  const row = markup('ui/Row.svelte')
  const rowSrc = read('ui/Row.svelte')
  const issuesSrc = read('screens/Issues.svelte')
  const detail = markup('screens/Detail.svelte')
  const detailSrc = read('screens/Detail.svelte')
  const sheet = read('ui/ScopeSheet.svelte')
  const domain = read('lib/domain.ts')

  it('① the age rides the meta line beside the key, not the title baseline', () => {
    // The judge counted 8 of 11 summaries truncated with the age and the
    // date both on the title baseline. The age is meta — it belongs on the
    // meta line, which already truncates by design; the date stays. Measured
    // by a4-captures on the demo fixture: the title gains 23px (306 → 329),
    // which is the chip and its gap, so this is a placement contract, not a
    // truncation one. What still costs the title width is the date.
    //
    // GDK-1543, 2026-09-07: and so the date followed it. The 23px the chip
    // gave back left 11 of the 12 first-screen summaries still cut; moving
    // the date off line 1 gave the summary the row's whole 370px content
    // box. This assertion is not loosened — it is re-pointed: the date is
    // still pinned to exactly one line, the meta line, and line 1 is now
    // asserted to hold nothing but the summary.
    const line1 = row.indexOf('class="line1"')
    const line2 = row.indexOf('class="line2"')
    const age = row.indexOf('class="age"')
    expect(line1).toBeGreaterThan(-1)
    expect(line2).toBeGreaterThan(line1)
    expect(age).toBeGreaterThan(line2)
    const when = row.indexOf('class="when"')
    expect(when).toBeGreaterThan(line2)
    // Line 1 is the sentence and nothing else: between the two lines sits
    // the summary span and its close, no third element.
    const line1Markup = row.slice(line1, line2)
    expect(line1Markup).toMatch(/class="summary"/)
    expect(line1Markup.match(/<span /g)?.length).toBe(2)
    // Still data, never a control (GDK-906), and still naming its own rule.
    expect(row.slice(row.lastIndexOf('<', age), age)).toBe('<span ')
    expect(row).toMatch(/class="age"[\s\S]{0,80}title=\{rowAgeTitle\(issue\)\}/)
  })

  it('② only the loud band carries colour; mid falls back to the meta greys', () => {
    // Three bands photographed as two: mid was the stale amber at 0.8, which
    // reads as the same dark brown as loud. Colour ladder only — the ratios
    // stay where the desk's workAge put them.
    const style = rowSrc.slice(rowSrc.indexOf('<style>'))
    const midAt = style.indexOf(".age[data-age-band='mid']")
    expect(midAt).toBeGreaterThan(-1)
    const midRule = style.slice(midAt, style.indexOf('}', midAt))
    expect(midRule).not.toMatch(/--color-status-stale/)
    expect(midRule).toMatch(/--color-text-(muted|secondary)/)
    expect(midRule).not.toMatch(/opacity/)
    const loudAt = style.indexOf(".age[data-age-band='loud']")
    expect(loudAt).toBeGreaterThan(-1)
    expect(style.slice(loudAt, style.indexOf('}', loudAt))).toMatch(/--color-status-stale/)
    expect(domain).toMatch(/ratio <= 2\) return 'quiet'/)
    expect(domain).toMatch(/ratio <= 4\) return 'mid'/)
  })

  it('③ the session strip may run to two lines rather than lose its fact', () => {
    // "1 of them assigned …" truncated away the most specific half of the
    // sentence. Two lines when it needs them, one quiet line when it fits.
    const style = issuesSrc.slice(issuesSrc.indexOf('<style>'))
    const at = style.indexOf('.session {')
    expect(at).toBeGreaterThan(-1)
    const rule = style.slice(at, style.indexOf('}', at))
    expect(rule).toMatch(/line-clamp:\s*2/)
    expect(rule).not.toMatch(/white-space:\s*nowrap/)
  })

  it('④ the resume card is a card: bounded, tinted, two lines, with a dismiss', () => {
    // It rendered as a bare grey line indistinguishable from the strip, and
    // offered no way out but tapping the sentence itself.
    const style = detailSrc.slice(detailSrc.indexOf('<style>'))
    const at = style.indexOf('.resume {')
    expect(at).toBeGreaterThan(-1)
    const rule = style.slice(at, style.indexOf('}', at))
    expect(rule).toMatch(/border-radius:\s*8px/)
    expect(rule).toMatch(/background:\s*var\(--color-bg-(elevated|panel)\)/)
    expect(rule).toMatch(/padding:/)
    const textAt = style.indexOf('.resume-text {')
    expect(textAt).toBeGreaterThan(-1)
    expect(style.slice(textAt, style.indexOf('}', textAt))).toMatch(/line-clamp:\s*2/)
    // An explicit dismiss at the iOS touch floor, labelled from the catalog.
    expect(detail).toMatch(/class="resume-x"/)
    expect(detail).toMatch(/aria-label=\{t\('detail\.resume\.dismiss'\)\}/)
    const xAt = style.indexOf('.resume-x {')
    expect(xAt).toBeGreaterThan(-1)
    const xRule = style.slice(xAt, style.indexOf('}', xAt))
    expect(xRule).toMatch(/width:\s*var\(--spacing-control\)/)
    expect(xRule).toMatch(/height:\s*var\(--spacing-control\)/)
    expect(detail).toMatch(/resumeDismissed = true/)
  })

  it('⑤ the five built-ins read as one section, and there is no sixth', () => {
    // A phone-authored "Assigned to me" sat alone under MY ISSUES while the
    // other four wore stance sub-labels under VIEWS, so the desk's one
    // built-in set read as two groups; folding it in left two rows with the
    // same count. It is gone (GDK-1542) — the section is the shared
    // catalog's five, and nothing else may push into it.
    const at = domain.indexOf("section: 'builtin'")
    expect(at).toBeGreaterThan(-1)
    // The only `section: 'builtin'` in the file is inside the catalog loop.
    expect(domain.split("section: 'builtin'")).toHaveLength(2)
    expect(domain.slice(0, at)).toMatch(/for \(const view of builtinViews\(\)\) \{/)
    expect(domain).not.toMatch(/SCOPE_ME/)
    expect(domain).not.toMatch(/ScopeSection = 'me'/)
    // The picker no longer draws a heading of its own for it.
    expect(sheet).not.toMatch(/personal\.myIssues/)
    expect(sheet).toMatch(/ORDER: ScopeSection\[\] = \['builtin', 'views', 'filters', 'docs'\]/)
  })
})
