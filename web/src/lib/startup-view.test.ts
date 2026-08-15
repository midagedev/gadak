import { describe, expect, test } from 'vitest'
import { emptyConfig, parseConfig, type ViewConfig } from './view-config'
import { applyStartupView, decideStartupView, type StartupViewInput } from './startup-view'

function input(over: Partial<StartupViewInput> = {}): StartupViewInput {
  return {
    urlHasViewParam: false,
    hostedDemo: false,
    epicBreakdown: undefined,
    lastViewKey: null,
    teamGroupEnabled: false,
    group: null,
    ...over,
  }
}

function epicConfig(): ViewConfig {
  const c = emptyConfig()
  c.filters.status_category = ['new', 'inprogress']
  c.display.group_by = 'epic'
  return c
}

describe('decideStartupView', () => {
  test('URL view params win over every other source', () => {
    expect(
      decideStartupView(
        input({
          urlHasViewParam: true,
          hostedDemo: true,
          epicBreakdown: epicConfig(),
          lastViewKey: 'q=foo',
          teamGroupEnabled: true,
          group: 'platform',
        }),
      ),
    ).toEqual({ kind: 'keep-url' })
  })

  test('hosted demo with an epic-breakdown preset applies that config', () => {
    const epic = epicConfig()
    expect(decideStartupView(input({ hostedDemo: true, epicBreakdown: epic }))).toEqual({
      kind: 'apply',
      config: epic,
    })
  })

  test('hosted demo without the preset falls through to last-used / group / all-open', () => {
    const allOpen = emptyConfig()
    allOpen.filters.status_category = ['new', 'inprogress']
    expect(decideStartupView(input({ hostedDemo: true }))).toEqual({
      kind: 'apply',
      config: allOpen,
    })
    const last = decideStartupView(input({ hostedDemo: true, lastViewKey: 'q=foo' }))
    expect(last).toEqual({ kind: 'apply', config: parseConfig(new URLSearchParams('q=foo')) })
  })

  test('last-used viewKey is parsed the way applyStartupView used to parse it', () => {
    expect(decideStartupView(input({ lastViewKey: 'sc=new,inprogress&g=epic' }))).toEqual({
      kind: 'apply',
      config: parseConfig(new URLSearchParams('sc=new,inprogress&g=epic')),
    })
  })

  test('empty last-used string is missing, not a view', () => {
    const group = emptyConfig()
    group.filters.team_group = ['platform']
    group.filters.status_category = ['new', 'inprogress']
    expect(
      decideStartupView(input({ lastViewKey: '', teamGroupEnabled: true, group: 'platform' })),
    ).toEqual({ kind: 'apply', config: group })
  })

  test('group preset needs both the flag and a group', () => {
    const expected = emptyConfig()
    expected.filters.team_group = ['platform']
    expected.filters.status_category = ['new', 'inprogress']
    expect(decideStartupView(input({ teamGroupEnabled: true, group: 'platform' }))).toEqual({
      kind: 'apply',
      config: expected,
    })
    expect(decideStartupView(input({ teamGroupEnabled: true, group: null }))).toEqual({
      kind: 'apply',
      config: (() => {
        const c = emptyConfig()
        c.filters.status_category = ['new', 'inprogress']
        return c
      })(),
    })
    expect(decideStartupView(input({ teamGroupEnabled: false, group: 'platform' }))).toEqual({
      kind: 'apply',
      config: (() => {
        const c = emptyConfig()
        c.filters.status_category = ['new', 'inprogress']
        return c
      })(),
    })
  })

  test('default is all open (new + inprogress)', () => {
    const c = emptyConfig()
    c.filters.status_category = ['new', 'inprogress']
    expect(decideStartupView(input())).toEqual({ kind: 'apply', config: c })
  })

  test('last-used wins over the group preset', () => {
    expect(
      decideStartupView(
        input({ lastViewKey: 'q=foo', teamGroupEnabled: true, group: 'platform' }),
      ),
    ).toEqual({ kind: 'apply', config: parseConfig(new URLSearchParams('q=foo')) })
  })
})

describe('applyStartupView', () => {
  test('keep-url does not call applyConfig', () => {
    const applied: ViewConfig[] = []
    applyStartupView(input({ urlHasViewParam: true }), (c) => applied.push(c))
    expect(applied).toEqual([])
  })

  test('apply hands the decided config to applyConfig once', () => {
    const applied: ViewConfig[] = []
    applyStartupView(input(), (c) => applied.push(c))
    const expected = emptyConfig()
    expected.filters.status_category = ['new', 'inprogress']
    expect(applied).toEqual([expected])
  })
})
