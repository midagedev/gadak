import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * GDK-1925 — the five write sheets Detail.svelte shed, as contracts.
 *
 * Same style as this tree's other component tests (AttachmentGrid,
 * GlanceStrip): source contracts over the .svelte, no DOM mount harness.
 * The behaviour half (what a pick does to the wire) stays in
 * lib/writes.test.ts against a fake transport, and the rendered half
 * (geometry, sheet chrome) stays in the viewport gate — this file pins the
 * boundary the extraction created:
 *
 *   ③ request invariance — a sheet fetches nothing. Catalogs and searches
 *      cross the wire through the screen or the typed wrappers, never
 *      through a sheet's own `request(`.
 *   · ownership — every write answers through onwritten/onrefused, the
 *      screen's two verdicts, and each sheet carries its own per-row
 *      flight state instead of the trio the sheets used to share.
 */

const here = dirname(fileURLToPath(import.meta.url))
const read = (name: string): string => readFileSync(join(here, name), 'utf8')

const sheets = [
  'TransitionSheet.svelte',
  'AssigneeSheet.svelte',
  'PrioritySheet.svelte',
  'LabelsSheet.svelte',
  'DueSheet.svelte',
] as const

/** The four that write: each answers through the wrappers. TransitionSheet
 *  is presentational only — its props in, one onpick out. */
const pickers = [
  'AssigneeSheet.svelte',
  'PrioritySheet.svelte',
  'LabelsSheet.svelte',
  'DueSheet.svelte',
] as const

const detail = readFileSync(join(here, '..', '..', 'screens', 'Detail.svelte'), 'utf8')

describe('GDK-1925 request invariance — a sheet fetches nothing', () => {
  it('no sheet owns a request body; every write is the typed wrapper', () => {
    // errorMessage may cross from lib/api — a sentence to paint is not a
    // request to make. The wire verbs (request/requestBlob) are what a
    // sheet may never hold.
    for (const name of sheets) {
      const src = read(name)
      expect(src, name).not.toMatch(/\brequest\s*[<(]/)
      expect(src, name).not.toMatch(/\brequestBlob\s*[<(]/)
    }
  })

  it('the screen keeps the two catalog null checks a reopen rides on', () => {
    // A second open of the priority sheet asks nothing: the catalog is the
    // screen's, kept for its life. The transitions sheet is the same
    // bargain with its own catalog.
    const at = detail.indexOf('function openPriority')
    const fn = detail.slice(at, detail.indexOf('\n  }', detail.indexOf('prioritiesLoading = true', at)))
    expect(fn).toMatch(/if \(priorities !== null \|\| prioritiesLoading\) return/)
    const tr = detail.slice(
      detail.indexOf('async function openTransitions'),
      detail.indexOf('async function applyTransition'),
    )
    expect(tr).toMatch(/if \(transitions !== null\) \{/)
  })
})

describe('GDK-1925 ownership — the two verdicts stay on the screen', () => {
  it('every sheet hands a landed write to onwritten, naming its field', () => {
    // Since GDK-1964 the landing also announces the save, and the sentence
    // names the field — so the call carries it: `onwritten(res.issue, '…')`.
    const field = {
      'AssigneeSheet.svelte': 'assignee',
      'PrioritySheet.svelte': 'priority',
      'LabelsSheet.svelte': 'labels',
      'DueSheet.svelte': 'due',
    } as const
    for (const name of pickers) {
      expect(read(name), name).toMatch(new RegExp(`onwritten\\(res\\.issue, '${field[name]}'\\)`))
      expect(read(name), name).not.toMatch(/void sync\(\)/)
    }
  })

  it('a refusal crosses as onrefused; only labels and due keep their sheet standing', () => {
    expect(read('AssigneeSheet.svelte')).toMatch(/if \(onrefused\(err\)\) return/)
    expect(read('PrioritySheet.svelte')).toMatch(/if \(onrefused\(err\)\) return/)
    // The composed-value bargain (GDK-1863): a set the person toggled or a
    // date they picked survives the refusal that took the writes away.
    expect(read('LabelsSheet.svelte')).toMatch(/onrefused\(err, true\)/)
    expect(read('DueSheet.svelte')).toMatch(/onrefused\(err, true\)/)
  })

  it('the screen latches the write, drops whichever sheet is standing, syncs', () => {
    const at = detail.indexOf('function onWritten')
    const fn = detail.slice(at, detail.indexOf('\n  }', at))
    expect(fn).toContain('written = next')
    for (const flag of ['sheetOpen', 'assigneeOpen', 'priorityOpen', 'labelsOpen', 'dueOpen']) {
      expect(fn).toContain(`${flag} = false`)
    }
    expect(fn).toMatch(/void sync\(\)/)
  })

  it('each pick sheet carries its own per-row flight state, not a shared trio', () => {
    // The sheets cannot be open at once, so per-sheet copies lose nothing —
    // and the screen no longer has a trio for a handler in an unrelated
    // sheet to reach. The screen's own copy is gone with the sheets.
    for (const name of ['AssigneeSheet.svelte', 'PrioritySheet.svelte'] as const) {
      const src = read(name)
      expect(src).toMatch(/let applyingId = \$state/)
      expect(src).toMatch(/let rowError = \$state/)
      expect(src).toMatch(/let failedRow = \$state/)
    }
    expect(detail).not.toMatch(/let (applyingId|rowError|failedRow) = \$state/)
  })

  it('the assignee search teardown rides the sheet, not the screen', () => {
    const src = read('AssigneeSheet.svelte')
    const teardown = src.slice(src.indexOf('onDestroy('))
    expect(teardown.indexOf('clearTimeout')).toBeGreaterThan(-1)
    expect(teardown.indexOf('abort()')).toBeGreaterThan(teardown.indexOf('clearTimeout'))
    // The screen that used to carry this teardown no longer does: a cleanup
    // sentence there would be dead code about a sheet it cannot see.
    const effect = detail.slice(detail.indexOf('$effect('), detail.indexOf('openTransitions'))
    expect(effect).not.toMatch(/searchTimer|searchAbort|searchSeq/)
  })

  it('the labels sheet reads the snapshot the phone already holds', () => {
    const src = read('LabelsSheet.svelte')
    expect(src).toMatch(/knownLabels\(app\.issues\)/)
    // Read-only: the store import never writes back through its own hands.
    expect(src).not.toMatch(/\bapp\.\w+ =/)
  })
})

describe('GDK-1925 the tick has one owner', () => {
  it('CurrentTick is the only svg with the tick class in the family', () => {
    expect(read('CurrentTick.svelte')).toMatch(/<svg class="tick"/)
    for (const name of sheets) {
      expect(read(name), name).not.toMatch(/class="tick"/)
    }
    expect(detail).not.toMatch(/class="tick"/)
  })
})

describe('GDK-1965 every pick sheet shows the current value', () => {
  // What the work carries right now, above the rows that would change it.
  // The marker is the attribute each sheet's rows already own: aria-current
  // on the three single-pick sheets, aria-pressed on labels' multi-pick,
  // and the due sheet's own {#if current} clearing row.
  it('transition, priority and assignee carry an aria-current marker', () => {
    expect(read('TransitionSheet.svelte')).toMatch(/aria-current/)
    expect(read('PrioritySheet.svelte')).toMatch(/aria-current/)
    expect(read('AssigneeSheet.svelte')).toMatch(/aria-current/)
  })

  it('labels marks the on set with aria-pressed; due shows the current line', () => {
    expect(read('LabelsSheet.svelte')).toMatch(/aria-pressed/)
    expect(read('DueSheet.svelte')).toMatch(/\{#if current\}/)
  })

  it('the screen hands TransitionSheet the status it is about to move', () => {
    // The sheet cannot know the status — the screen passes it, and the
    // marker assertions above are meaningless without this hand-off. The
    // category rides spineToken, the screen's own single owner for the
    // token every dot on it already paints by.
    const at = detail.indexOf('<TransitionSheet')
    const call = detail.slice(at, detail.indexOf('/>', at))
    expect(call).toContain('current={lite ? { status: lite.status, category: spineToken(lite) } : null}')
  })
})
