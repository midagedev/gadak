/*
 * GDK-1560 (#17) recurrence gate. The reported symptom was one label
 * ('ソースプロジェクト') wrapping to two lines in the ja breakdown menu; the
 * class is a label outgrowing the cell in a locale nobody is looking at, so
 * this re-derives the fit for every option in every locale from the shipped
 * catalog. A translation that does not fit fails here rather than in a
 * screenshot months later.
 *
 * FAIL-first at authoring time (menu still w-64, CELL_TEXT_PX 100):
 *   ja field.source_project 117px, ko field.source_project 117px,
 *   ko field.development_test_result 104px — three over, all > 100px.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import { messages } from '../../lib/i18n/catalog'
import { LOCALES } from '../../lib/i18n/types'
import { CELL_TEXT_PX, MENU_PX, fitsCell, labelWidthPx } from './breakdown-menu-width'

const HERE = dirname(fileURLToPath(import.meta.url))
const BAR = readFileSync(join(HERE, 'BreakdownBar.svelte'), 'utf8')

/** ALL_OPTIONS in BreakdownBar.svelte, as catalog keys. Kept in step by the
 *  last test in this file rather than by memory. */
const OPTION_KEYS = [
  'field.status_category',
  'field.status',
  'group.byProduct',
  'field.team_group',
  'field.assignee',
  'field.actor',
  'field.priority',
  'field.severity',
  'field.issue_type',
  'field.development_test_result',
  'field.qa_impact',
  'field.source_project',
  'group.byEpic',
  'group.sectionNone',
] as const

describe('GDK-1560 breakdown menu fits its labels', () => {
  test.each(LOCALES)('%s: every option label is one line', (locale) => {
    const over = OPTION_KEYS.map((key) => {
      const label = (messages as Record<string, Record<string, string>>)[key][locale]
      return { key, label, px: labelWidthPx(label) }
    }).filter((o) => !fitsCell(o.label))
    expect(
      over.map((o) => `${o.key} "${o.label}" ${o.px}px > ${CELL_TEXT_PX}px`).join('\n') || '(none)',
      `widen MENU_PX (now ${MENU_PX}) or shorten the ${locale} label — a menu cell is one line`,
    ).toBe('(none)')
  })

  // The arithmetic is only true of the real menu if the markup still uses the
  // width it models, and the nowrap keeps a future overgrowth from silently
  // reflowing the grid instead of failing this file.
  test('BreakdownBar uses the modeled width and cannot wrap', () => {
    expect(BAR).toContain(`w-${MENU_PX / 4}`)
    expect(BAR).toMatch(/grid-cols-2/)
    expect(BAR, 'option buttons are nowrap + truncate').toMatch(/whitespace-nowrap truncate/)
  })

  // A new axis in ALL_OPTIONS that is not listed above would be measured by
  // nobody, so the count is asserted rather than assumed.
  test('OPTION_KEYS covers every ALL_OPTIONS entry', () => {
    const entries = BAR.slice(BAR.indexOf('ALL_OPTIONS'), BAR.indexOf('const OPTIONS'))
    const count = (entries.match(/\{ key: '/g) ?? []).length
    expect(count, 'add the new axis to OPTION_KEYS').toBe(OPTION_KEYS.length)
  })
})
