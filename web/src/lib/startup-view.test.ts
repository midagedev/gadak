import { describe, expect, test } from 'vitest'
import { emptyConfig, parseConfig, type ViewConfig } from './view-config'
import {
  applyStartupView,
  decideStartupView,
  demoStartupConfig,
  startupViewTick,
  type StartupViewInput,
} from './startup-view'
import { builtinViews } from './builtin-views'

function input(over: Partial<StartupViewInput> = {}): StartupViewInput {
  return {
    urlHasViewParam: false,
    hostedDemo: false,
    demoView: undefined,
    lastViewKey: null,
    teamGroupEnabled: false,
    group: null,
    identified: false,
    myWork: undefined,
    myWorkCount: 0,
    ...over,
  }
}

/** The hosted demo's landing shape: the open pool grouped by epic. */
function epicConfig(): ViewConfig {
  const c = emptyConfig()
  c.filters.status_category = ['new', 'inprogress']
  c.display.group_by = 'epic'
  return c
}

function allOpen(): ViewConfig {
  const c = emptyConfig()
  c.filters.status_category = ['new', 'inprogress']
  return c
}

describe('decideStartupView', () => {
  test('URL view params win over every other source', () => {
    expect(
      decideStartupView(
        input({
          urlHasViewParam: true,
          hostedDemo: true,
          demoView: epicConfig(),
          lastViewKey: 'q=foo',
          teamGroupEnabled: true,
          group: 'platform',
        }),
      ),
    ).toEqual({ kind: 'keep-url' })
  })

  test('hosted demo with a landing config applies it', () => {
    const epic = epicConfig()
    expect(decideStartupView(input({ hostedDemo: true, demoView: epic }))).toEqual({
      kind: 'apply',
      config: epic,
    })
  })

  test('hosted demo without a landing config falls through to last-used / group / all-open', () => {
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

  test('first run on a self site lands on all open', () => {
    // GDK-100 made this the Epic breakdown; GDK-1493 (2026-09-07) removed
    // that built-in — a layout of the open pool that needs a hierarchy the
    // site may not use — so the anonymous first run is the pool itself.
    expect(decideStartupView(input({}))).toEqual({ kind: 'apply', config: allOpen() })
  })

  test('the last-used view beats the first-run pool', () => {
    expect(decideStartupView(input({ lastViewKey: 'q=foo' }))).toEqual({
      kind: 'apply',
      config: parseConfig(new URLSearchParams('q=foo')),
    })
  })

  test('the group preset beats the first-run pool — personalization over the generic default', () => {
    const group = emptyConfig()
    group.filters.team_group = ['platform']
    group.filters.status_category = ['new', 'inprogress']
    expect(decideStartupView(input({ teamGroupEnabled: true, group: 'platform' }))).toEqual({
      kind: 'apply',
      config: group,
    })
  })

  test('demoStartupConfig is the open pool grouped by epic, and not a built-in', () => {
    expect(demoStartupConfig()).toEqual(epicConfig())
    expect(builtinViews().some((v) => v.config.display.group_by === 'epic')).toBe(false)
  })
})

/*
 * my-work pack: the contributor's first screen (G4 — "a first screen that is
 * 'my work'"; THEORY.md "Two stances").
 *
 * Contract ↔ assertion table (clause → assertion names):
 *  C5 first run: my-work when identified && myWork && myWorkCount > 0
 *     first run, identified, assigned work → my-work
 *     first run, identified, zero assigned work → epic breakdown
 *     first run, not identified → epic breakdown (identity gate lives here)
 *     first run without the my-work preset → epic breakdown
 *  C5 last-used still beats the first-run my-work
 *     the last-used view beats the first-run my-work
 *     the group preset beats the first-run my-work
 *
 * FAIL-first 2026-09-06: against the pre-change decideStartupView every
 * test in this block failed — the my-work branch did not exist, so the
 * identified+count case returned the epic breakdown (or all-open).
 */
describe('decideStartupView: first-run my-work (my-work pack)', () => {
  function myWorkConfig(): ViewConfig {
    const c = emptyConfig()
    c.filters.mine = true
    c.filters.status_category = ['inprogress', 'new']
    c.display.sort = 'priority'
    c.display.dir = 'asc'
    return c
  }

  test('first run, identified, assigned work → my-work', () => {
    const myWork = myWorkConfig()
    expect(
      decideStartupView(
        input({ identified: true, myWork, myWorkCount: 46 }),
      ),
    ).toEqual({ kind: 'apply', config: myWork })
  })

  test('first run, identified, zero assigned work → all open', () => {
    expect(
      decideStartupView(input({ identified: true, myWork: myWorkConfig(), myWorkCount: 0 })),
    ).toEqual({ kind: 'apply', config: allOpen() })
  })

  test('first run, not identified → all open (identity gate lives here)', () => {
    // App guards the count behind identified, but the decision itself must
    // not open an identity view for an anonymous reader even if a caller
    // passed a stale count.
    expect(
      decideStartupView(
        input({ identified: false, myWork: myWorkConfig(), myWorkCount: 5 }),
      ),
    ).toEqual({ kind: 'apply', config: allOpen() })
  })

  test('first run without the my-work preset → all open', () => {
    expect(
      decideStartupView(input({ identified: true, myWork: undefined, myWorkCount: 46 })),
    ).toEqual({ kind: 'apply', config: allOpen() })
  })

  test('the last-used view beats the first-run my-work', () => {
    expect(
      decideStartupView(
        input({ identified: true, myWork: myWorkConfig(), myWorkCount: 46, lastViewKey: 'q=foo' }),
      ),
    ).toEqual({ kind: 'apply', config: parseConfig(new URLSearchParams('q=foo')) })
  })

  test('the group preset beats the first-run my-work', () => {
    const group = emptyConfig()
    group.filters.team_group = ['platform']
    group.filters.status_category = ['new', 'inprogress']
    expect(
      decideStartupView(
        input({
          identified: true,
          myWork: myWorkConfig(),
          myWorkCount: 46,
          teamGroupEnabled: true,
          group: 'platform',
        }),
      ),
    ).toEqual({ kind: 'apply', config: group })
  })

  test('hosted demo still lands on its own config before my-work', () => {
    const epic = epicConfig()
    expect(
      decideStartupView(
        input({ hostedDemo: true, demoView: epic, identified: true, myWork: myWorkConfig(), myWorkCount: 46 }),
      ),
    ).toEqual({ kind: 'apply', config: epic })
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

/*
 * GDK-46 unit — startupViewTick
 *   keys before commit do not vanish:  first tick after apply is mark-ready
 *   user view change still resets:     later tick with a new viewKey
 *   same-view after ready is not a reset (boundary of the reset clause)
 */
describe('startupViewTick (GDK-46 readiness rule)', () => {
  test('before the startup view is applied the list waits', () => {
    expect(startupViewTick(false, false)).toBe('wait')
    expect(startupViewTick(false, true, 'sc=done', '')).toBe('wait')
  })

  test('the first tick after apply marks ready instead of resetting', () => {
    expect(startupViewTick(true, false, 'sc=new,inprogress', '')).toBe('mark-ready')
    expect(startupViewTick(true, false, 'sc=new,inprogress', 'sc=new,inprogress')).toBe(
      'mark-ready',
    )
  })

  test('a later tick with a new viewKey resets the cursor', () => {
    expect(startupViewTick(true, true, 'sc=done', 'sc=new,inprogress')).toBe('reset-cursor')
  })

  test('a later tick with the same viewKey does not reset', () => {
    expect(startupViewTick(true, true, 'sc=new,inprogress', 'sc=new,inprogress')).toBe(
      'same-view',
    )
    expect(startupViewTick(true, true, '', '')).toBe('same-view')
  })
})
