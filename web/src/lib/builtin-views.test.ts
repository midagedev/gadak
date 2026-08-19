import { describe, expect, test } from 'vitest'
import { startOfWeekMonday } from './calendar'
import { builtinViews } from './builtin-views'
import { configToParams } from './view-config'

describe('builtinViews (moved from e2e/dates.spec.ts)', () => {
  test('resolved-week serialises to rf= (resolved_from), not uf=', () => {
    const view = builtinViews().find((v) => v.id === 'resolved-week')
    expect(view, 'resolved-week builtin must exist').toBeTruthy()
    const monday = startOfWeekMonday()
    expect(view!.config.filters.status_category).toEqual(['done'])
    expect(view!.config.filters.resolved_from).toBe(monday)
    expect(view!.config.filters.updated_from).toBeNull()
    const params = configToParams(view!.config)
    expect(params.rf).toBe(monday)
    expect(params.rf).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(params.uf).toBeNull()
    expect(params.sc).toBe('done')
  })
})
