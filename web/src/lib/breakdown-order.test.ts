/*
 * GDK-1850: the breakdown strip's chips follow the order of the sections or
 * columns drawn below them. Count order put `Done 6` before `In progress 4`
 * above a board whose columns read new / inprogress / done.
 */
import { describe, expect, test } from 'vitest'
import { orderBreakdownGroups } from './issue-group'

const g = (key: string, total: number) => ({ key, counts: { total } })
const groups = [g('new', 10), g('done', 6), g('inprogress', 4)]

describe('orderBreakdownGroups (GDK-1850)', () => {
  test('board: status_category reads in column order', () => {
    expect(orderBreakdownGroups(groups, 'status_category', 'board').map((x) => x.key)).toEqual([
      'new',
      'inprogress',
      'done',
    ])
  })

  test("list: status_category reads in the list's section order", () => {
    expect(orderBreakdownGroups(groups, 'status_category', 'list').map((x) => x.key)).toEqual([
      'inprogress',
      'new',
      'done',
    ])
  })

  test('other axes stay count-descending', () => {
    const a = [g('x', 1), g('y', 9), g('z', 4)]
    expect(orderBreakdownGroups(a, 'assignee', 'board').map((x) => x.key)).toEqual(['y', 'z', 'x'])
    expect(orderBreakdownGroups(a, 'assignee', 'list').map((x) => x.key)).toEqual(['y', 'z', 'x'])
  })

  test('does not mutate its input', () => {
    const before = groups.map((x) => x.key)
    orderBreakdownGroups(groups, 'status_category', 'board')
    expect(groups.map((x) => x.key)).toEqual(before)
  })
})
