import { describe, expect, test } from 'vitest'
import {
  DETAIL_TESTID,
  NARROW_FIELD_TESTID,
  isBootHoldKey,
  isEditableTarget,
  isEnterActivatingTarget,
  keyContext,
  narrowFieldTestId,
  replayHeldListKeys,
  resolveGlobalKey,
} from './keymap.svelte'

type EnterWant = 'ignore' | 'open-cursor'

/** Duck-typed event target so the table runs in the node vitest project. */
function fakeTarget(
  tag: string,
  attrs: Record<string, string> = {},
  extra: { isContentEditable?: boolean; parent?: EventTarget | null } = {},
): EventTarget {
  return {
    tagName: tag.toUpperCase(),
    isContentEditable: Boolean(extra.isContentEditable),
    parentElement: extra.parent ?? null,
    getAttribute: (name: string) => (name in attrs ? attrs[name] : null),
    hasAttribute: (name: string) => name in attrs,
  } as unknown as EventTarget
}

function resolveEnterOn(row: { target: EventTarget }) {
  return resolveGlobalKey(
    keyContext({
      key: 'Enter',
      listActive: true,
      cursorKey: 'NMB-1',
      inEditable: isEditableTarget(row.target),
      enterActivating: isEnterActivatingTarget(row.target),
    }),
  )
}

const ENTER_TARGET_CASES: {
  shape: string
  target: EventTarget
  inEditable: boolean
  activates: boolean
  want: EnterWant
}[] = [
  {
    shape: 'button',
    target: fakeTarget('button'),
    inEditable: false,
    activates: true,
    want: 'ignore',
  },
  {
    shape: 'span inside button',
    target: fakeTarget('span', {}, { parent: fakeTarget('button') }),
    inEditable: false,
    activates: true,
    want: 'ignore',
  },
  {
    shape: 'a[href]',
    target: fakeTarget('a', { href: '#' }),
    inEditable: false,
    activates: true,
    want: 'ignore',
  },
  {
    shape: 'input[type=text]',
    target: fakeTarget('input', { type: 'text' }),
    inEditable: true,
    activates: false,
    want: 'ignore',
  },
  {
    shape: 'input[type=checkbox]',
    target: fakeTarget('input', { type: 'checkbox' }),
    inEditable: true,
    activates: false,
    want: 'ignore',
  },
  {
    shape: 'div',
    target: fakeTarget('div'),
    inEditable: false,
    activates: false,
    want: 'open-cursor',
  },
  {
    shape: 'body',
    target: fakeTarget('body'),
    inEditable: false,
    activates: false,
    want: 'open-cursor',
  },
  {
    shape: 'list row',
    target: fakeTarget('div', { 'data-issue-key': 'NMB-1', role: 'button' }),
    inEditable: false,
    activates: false,
    want: 'open-cursor',
  },
  {
    shape: 'role="button"',
    target: fakeTarget('div', { role: 'button' }),
    inEditable: false,
    activates: false,
    want: 'open-cursor',
  },
  {
    shape: 'contenteditable',
    target: fakeTarget('div', { contenteditable: 'true' }, { isContentEditable: true }),
    inEditable: true,
    activates: true,
    want: 'ignore',
  },
]

describe('narrow field / detail testid map', () => {
  test('testids match the nodes the shell already mounts', () => {
    expect(NARROW_FIELD_TESTID.history).toBe('history-filter-input')
    expect(NARROW_FIELD_TESTID.docs).toBe('docs-filter-input')
    expect(NARROW_FIELD_TESTID.issues).toBe('search-input')
    expect(DETAIL_TESTID.status).toBe('status-transition')
    expect(DETAIL_TESTID.assignee).toBe('assignee-picker')
    expect(DETAIL_TESTID.priority).toBe('priority-picker')
    expect(DETAIL_TESTID.labelInput).toBe('label-editor-input')
    expect(DETAIL_TESTID.labelAdd).toBe('label-editor-add')
    expect(DETAIL_TESTID.comment).toBe('comment-composer')
  })

  test('narrowFieldTestId: feed blocks; else history, docs, issues', () => {
    expect(
      narrowFieldTestId({ feedBlocksNarrow: true, historyView: true, docsOpen: true }),
    ).toBeNull()
    expect(
      narrowFieldTestId({ feedBlocksNarrow: false, historyView: true, docsOpen: false }),
    ).toBe('history-filter-input')
    expect(
      narrowFieldTestId({ feedBlocksNarrow: false, historyView: false, docsOpen: true }),
    ).toBe('docs-filter-input')
    expect(
      narrowFieldTestId({ feedBlocksNarrow: false, historyView: false, docsOpen: false }),
    ).toBe('search-input')
  })
})

describe('resolveGlobalKey', () => {
  test('⌘K / Ctrl+K toggles the palette even in a field or under a modal', () => {
    expect(resolveGlobalKey(keyContext({ key: 'k', metaKey: true, inEditable: true }))).toEqual({
      type: 'toggle-palette',
    })
    expect(
      resolveGlobalKey(keyContext({ key: 'K', ctrlKey: true, settingsOpen: true })),
    ).toEqual({ type: 'toggle-palette' })
  })

  test('other modifiers and typing in a field are ignored', () => {
    expect(resolveGlobalKey(keyContext({ key: 'k', altKey: true }))).toEqual({ type: 'ignore' })
    expect(resolveGlobalKey(keyContext({ key: 'j', metaKey: true, listActive: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: 'j', inEditable: true, listActive: true }))).toEqual({
      type: 'ignore',
    })
  })

  test('open settings / new issue / palette / comment / media viewer swallow keys other than ⌘K', () => {
    expect(resolveGlobalKey(keyContext({ key: '?', settingsOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: '?', newIssueOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: '?', paletteOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: '?', commentOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: '?', serverSettingsOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: '?', mediaViewerOpen: true }))).toEqual({
      type: 'ignore',
    })
  })

  test('shortcuts sheet: ? closes; every other key is ignored', () => {
    expect(resolveGlobalKey(keyContext({ key: '?', shortcutsOpen: true }))).toEqual({
      type: 'close-shortcuts',
    })
    expect(resolveGlobalKey(keyContext({ key: 'j', shortcutsOpen: true, listActive: true }))).toEqual(
      { type: 'ignore' },
    )
  })

  test('? opens the shortcuts sheet', () => {
    expect(resolveGlobalKey(keyContext({ key: '?' }))).toEqual({ type: 'open-shortcuts' })
  })

  test(', opens settings; ignored in a field or under a modal', () => {
    expect(resolveGlobalKey(keyContext({ key: ',' }))).toEqual({ type: 'open-settings' })
    expect(resolveGlobalKey(keyContext({ key: ',', inEditable: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', settingsOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', serverSettingsOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', paletteOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', shortcutsOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', mediaViewerOpen: true }))).toEqual({
      type: 'ignore',
    })
  })

  test('/ focuses the column narrow field; feed has none', () => {
    expect(resolveGlobalKey(keyContext({ key: '/' }))).toEqual({
      type: 'focus-narrow',
      testid: 'search-input',
    })
    expect(resolveGlobalKey(keyContext({ key: '/', historyView: true }))).toEqual({
      type: 'focus-narrow',
      testid: 'history-filter-input',
    })
    expect(resolveGlobalKey(keyContext({ key: '/', docsOpen: true }))).toEqual({
      type: 'focus-narrow',
      testid: 'docs-filter-input',
    })
    expect(resolveGlobalKey(keyContext({ key: '/', feedBlocksNarrow: true }))).toEqual({
      type: 'focus-narrow',
      testid: null,
    })
  })

  test('j/k move only while the list is active', () => {
    expect(resolveGlobalKey(keyContext({ key: 'j', listActive: true }))).toEqual({
      type: 'move-list',
      dir: 1,
    })
    expect(resolveGlobalKey(keyContext({ key: 'k', listActive: true }))).toEqual({
      type: 'move-list',
      dir: -1,
    })
    expect(resolveGlobalKey(keyContext({ key: 'j', listActive: false }))).toEqual({ type: 'ignore' })
  })

  test('Enter opens the cursor row only when the list is active', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 'Enter', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'open-cursor' })
    expect(resolveGlobalKey(keyContext({ key: 'Enter', cursorKey: 'NMB-1' }))).toEqual({
      type: 'ignore',
    })
  })

  /*
   * GDK-276: Enter on a control the browser already activates must not become
   * open-cursor. The table is the decision (target shape → ignore | open-cursor).
   */
  test.each(ENTER_TARGET_CASES)('Enter on $shape is $want', (row) => {
    expect(isEditableTarget(row.target), `${row.shape} isEditableTarget`).toBe(row.inEditable)
    expect(isEnterActivatingTarget(row.target), `${row.shape} isEnterActivatingTarget`).toBe(
      row.activates,
    )
    expect(resolveEnterOn(row).type).toBe(row.want)
  })

  test('j still moves the list when a button would own Enter', () => {
    expect(
      resolveGlobalKey(
        keyContext({
          key: 'j',
          listActive: true,
          cursorKey: 'NMB-1',
          enterActivating: true,
        }),
      ),
    ).toEqual({ type: 'move-list', dir: 1 })
  })

  test('paletteOpen and inEditable still swallow Enter (search / palette)', () => {
    expect(
      resolveGlobalKey(
        keyContext({ key: 'Enter', listActive: true, cursorKey: 'NMB-1', paletteOpen: true }),
      ),
    ).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'Enter', listActive: true, cursorKey: 'NMB-1', inEditable: true }),
      ),
    ).toEqual({ type: 'ignore' })
  })

  test('Escape: browse, then menu (pass), then bulk, then detail', () => {
    expect(resolveGlobalKey(keyContext({ key: 'Escape', browsePaneOpen: true }))).toEqual({
      type: 'hide-browse',
    })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'Escape', browsePaneOpen: true, bulkActive: true, detailOpen: true }),
      ),
    ).toEqual({ type: 'hide-browse' })
    expect(resolveGlobalKey(keyContext({ key: 'Escape', triageMenuOpen: true }))).toEqual({
      type: 'ignore',
    })
    expect(resolveGlobalKey(keyContext({ key: 'Escape', bulkActive: true }))).toEqual({
      type: 'clear-bulk',
    })
    expect(resolveGlobalKey(keyContext({ key: 'Escape', detailOpen: true }))).toEqual({
      type: 'clear-selection',
    })
    expect(resolveGlobalKey(keyContext({ key: 'Escape' }))).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(
        keyContext({
          key: 'Escape',
          mediaViewerOpen: true,
          detailOpen: true,
          bulkActive: true,
        }),
      ),
    ).toEqual({ type: 'ignore' })
  })

  test('x: page, then person, then cursor, then detail', () => {
    expect(resolveGlobalKey(keyContext({ key: 'x', pageSelected: true, detailOpen: true }))).toEqual({
      type: 'clear-page',
    })
    expect(
      resolveGlobalKey(keyContext({ key: 'x', personSelected: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'clear-person' })
    expect(
      resolveGlobalKey(keyContext({ key: 'x', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'toggle-bulk-cursor' })
    expect(resolveGlobalKey(keyContext({ key: 'x', detailOpen: true }))).toEqual({
      type: 'clear-selection',
    })
    expect(resolveGlobalKey(keyContext({ key: 'x' }))).toEqual({ type: 'ignore' })
  })

  test('s/a/l: bulk or cursor-without-detail opens the menu; detail clicks the picker', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 's', bulkActive: true, detailOpen: true })),
    ).toEqual({ type: 'request-menu', menu: 'status' })
    expect(
      resolveGlobalKey(keyContext({ key: 'a', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'request-menu', menu: 'assignee' })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'l', listActive: true, cursorKey: 'NMB-1', detailOpen: true }),
      ),
    ).toEqual({ type: 'activate-labels' })
    expect(resolveGlobalKey(keyContext({ key: 's', detailOpen: true }))).toEqual({
      type: 'click-detail',
      testid: 'status-transition',
    })
    expect(resolveGlobalKey(keyContext({ key: 'a', detailOpen: true }))).toEqual({
      type: 'click-detail',
      testid: 'assignee-picker',
    })
    expect(resolveGlobalKey(keyContext({ key: 's' }))).toEqual({ type: 'ignore' })
  })

  test('p: bulk or cursor-without-detail opens the priority menu; detail clicks the picker', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 'p', bulkActive: true, detailOpen: true })),
    ).toEqual({ type: 'request-menu', menu: 'priority' })
    expect(
      resolveGlobalKey(keyContext({ key: 'p', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'request-menu', menu: 'priority' })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'p', listActive: true, cursorKey: 'NMB-1', detailOpen: true }),
      ),
    ).toEqual({ type: 'click-detail', testid: 'priority-picker' })
    expect(resolveGlobalKey(keyContext({ key: 'p', detailOpen: true }))).toEqual({
      type: 'click-detail',
      testid: 'priority-picker',
    })
    expect(resolveGlobalKey(keyContext({ key: 'p' }))).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(keyContext({ key: 'p', inEditable: true, bulkActive: true })),
    ).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(keyContext({ key: 'p', paletteOpen: true, bulkActive: true })),
    ).toEqual({ type: 'ignore' })
  })

  test('c: detail composer, else cursor comment, else new issue', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 'c', detailOpen: true, listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'focus-comment' })
    expect(
      resolveGlobalKey(keyContext({ key: 'c', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'open-comment-cursor' })
    expect(resolveGlobalKey(keyContext({ key: 'c' }))).toEqual({ type: 'new-issue' })
  })

  /*
   * GDK-81: o is the header escape-hatch alias (detail.openJira / doc.openSource).
   *   happy:    list cursor, open detail, or selected page → open-origin
   *   boundary: no target, and typing in a field, stay ignore
   */
  test('o opens origin with a list cursor, an open detail, or a selected page', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 'o', listActive: true, cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'open-origin', target: 'issue' })
    expect(resolveGlobalKey(keyContext({ key: 'o', detailOpen: true }))).toEqual({
      type: 'open-origin',
      target: 'issue',
    })
    expect(resolveGlobalKey(keyContext({ key: 'o', pageSelected: true }))).toEqual({
      type: 'open-origin',
      target: 'page',
    })
    expect(
      resolveGlobalKey(
        keyContext({
          key: 'o',
          pageSelected: true,
          listActive: true,
          cursorKey: 'NMB-1',
          detailOpen: true,
        }),
      ),
    ).toEqual({ type: 'open-origin', target: 'page' })
  })

  test('o is ignored without a cursor, detail, or page, and in a field', () => {
    expect(resolveGlobalKey(keyContext({ key: 'o' }))).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(keyContext({ key: 'o', cursorKey: 'NMB-1' })),
    ).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'o', listActive: true, cursorKey: 'NMB-1', inEditable: true }),
      ),
    ).toEqual({ type: 'ignore' })
    expect(
      resolveGlobalKey(
        keyContext({ key: 'o', listActive: true, cursorKey: 'NMB-1', paletteOpen: true }),
      ),
    ).toEqual({ type: 'ignore' })
  })

  /*
   * GDK-46 unit: keys before the startup commit do not vanish
   *   happy:    j/k/x before keysReady are held when the list is mounted
   *   boundary: j before keysReady is ignored when the list is not mounted
   */
  test('j/k/x before keysReady are held when the list is mounted', () => {
    expect(
      resolveGlobalKey(keyContext({ key: 'j', listActive: true, keysReady: false })),
    ).toEqual({ type: 'hold-boot-key', key: 'j' })
    expect(
      resolveGlobalKey(keyContext({ key: 'k', listActive: true, keysReady: false })),
    ).toEqual({ type: 'hold-boot-key', key: 'k' })
    expect(
      resolveGlobalKey(keyContext({ key: 'x', listActive: true, keysReady: false })),
    ).toEqual({ type: 'hold-boot-key', key: 'x' })
  })

  test('j before keysReady is ignored when the list is not mounted', () => {
    expect(resolveGlobalKey(keyContext({ key: 'j', keysReady: false }))).toEqual({
      type: 'ignore',
    })
  })

  test('?/, comma and ⌘K still work before keysReady', () => {
    expect(resolveGlobalKey(keyContext({ key: '?', keysReady: false, listActive: true }))).toEqual({
      type: 'open-shortcuts',
    })
    expect(resolveGlobalKey(keyContext({ key: ',', keysReady: false, listActive: true }))).toEqual({
      type: 'open-settings',
    })
    expect(resolveGlobalKey(keyContext({ key: '/', keysReady: false, listActive: true }))).toEqual({
      type: 'focus-narrow',
      testid: 'search-input',
    })
    expect(
      resolveGlobalKey(keyContext({ key: 'k', metaKey: true, keysReady: false, listActive: true })),
    ).toEqual({ type: 'toggle-palette' })
  })

  test('j after keysReady still moves', () => {
    expect(resolveGlobalKey(keyContext({ key: 'j', listActive: true, keysReady: true }))).toEqual({
      type: 'move-list',
      dir: 1,
    })
  })
})

describe('boot-held list keys', () => {
  test('isBootHoldKey is only j/k/x', () => {
    expect(isBootHoldKey('j')).toBe(true)
    expect(isBootHoldKey('k')).toBe(true)
    expect(isBootHoldKey('x')).toBe(true)
    expect(isBootHoldKey('s')).toBe(false)
    expect(isBootHoldKey('c')).toBe(false)
  })

  test('replay j then x is move then toggle', () => {
    const calls: string[] = []
    replayHeldListKeys(['j', 'x'], {
      move: (dir) => calls.push(`move:${dir}`),
      toggleCursor: () => calls.push('toggle'),
    })
    expect(calls).toEqual(['move:1', 'toggle'])
  })

  test('replay x alone is only toggle (no invented cursor move)', () => {
    const calls: string[] = []
    replayHeldListKeys(['x'], {
      move: (dir) => calls.push(`move:${dir}`),
      toggleCursor: () => calls.push('toggle'),
    })
    expect(calls).toEqual(['toggle'])
  })

  test('replay k then unknown keys only moves up', () => {
    const calls: string[] = []
    replayHeldListKeys(['k', 's', ''], {
      move: (dir) => calls.push(`move:${dir}`),
      toggleCursor: () => calls.push('toggle'),
    })
    expect(calls).toEqual(['move:-1'])
  })

  test('empty hold list is a no-op', () => {
    const calls: string[] = []
    replayHeldListKeys([], {
      move: (dir) => calls.push(`move:${dir}`),
      toggleCursor: () => calls.push('toggle'),
    })
    expect(calls).toEqual([])
  })
})
