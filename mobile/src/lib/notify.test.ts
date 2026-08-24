/*
 * Notification transport tests. The contracts under pin:
 *
 *  - The visibility policy is the quiet-queue differential: while the app
 *    is visible the queue refreshes in place and showBanner raises nothing;
 *    a banner only goes out when the document is hidden.
 *  - The payload carries issue_key (group = the threadIdentifier analog,
 *    extra = the tap router's input) and nothing else — no badge options.
 *  - ensurePermission asks the OS at most through requestPermission after
 *    isPermissionGranted said no; A-nav owns when it is first called.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { isPermissionGranted, onAction, requestPermission, sendNotification } = vi.hoisted(() => ({
  isPermissionGranted: vi.fn(),
  onAction: vi.fn(),
  requestPermission: vi.fn(),
  sendNotification: vi.fn(),
}))
vi.mock('@tauri-apps/plugin-notification', () => ({
  isPermissionGranted,
  onAction,
  requestPermission,
  sendNotification,
}))

import type * as notifyModule from './notify'

// bindNotificationTap keeps module-level state (one plugin listener, a
// swappable handler) — each test needs a fresh module instance.
let notify: typeof notifyModule

/** The node test environment has no document — a minimal stand-in. */
function stubDocument(visibilityState: 'visible' | 'hidden'): void {
  vi.stubGlobal('document', { visibilityState })
}

beforeEach(async () => {
  vi.resetModules()
  notify = await import('./notify')
  sendNotification.mockReset()
  requestPermission.mockReset()
  isPermissionGranted.mockReset()
  onAction.mockReset()
  vi.unstubAllGlobals()
})

describe('visibility policy (quiet queue)', () => {
  it('raises nothing while the app is visible', () => {
    stubDocument('visible')
    const sent = notify.showBanner({ title: 'T', body: 'B', issueKey: 'GDK-1' })
    expect(sent).toBe(false)
    expect(sendNotification).not.toHaveBeenCalled()
  })

  it('sends the banner when the document is hidden', () => {
    stubDocument('hidden')
    const sent = notify.showBanner({ title: 'T', body: 'B', issueKey: 'GDK-1' })
    expect(sent).toBe(true)
    expect(sendNotification).toHaveBeenCalledOnce()
  })
})

describe('payload shape', () => {
  it('groups by issue_key (threadIdentifier analog) and carries it in extra — and sets nothing else', () => {
    stubDocument('hidden')
    notify.showBanner({ title: 'Robin mentioned you on GDK-9', body: 'look at this', issueKey: 'GDK-9' })
    expect(sendNotification).toHaveBeenCalledWith({
      title: 'Robin mentioned you on GDK-9',
      body: 'look at this',
      group: 'GDK-9',
      extra: { issue_key: 'GDK-9' },
    })
  })

  it('omits the group and nulls the payload key when there is no issue_key', () => {
    stubDocument('hidden')
    notify.showBanner({ title: 'T', body: 'B', issueKey: null })
    expect(sendNotification).toHaveBeenCalledWith({
      title: 'T',
      body: 'B',
      group: undefined,
      extra: { issue_key: null },
    })
  })
})

describe('tap routing', () => {
  it('forwards extra.issue_key to onopen', () => {
    let captured: ((n: unknown) => void) | undefined
    onAction.mockImplementation((cb: (n: unknown) => void) => {
      captured = cb
    })
    const onopen = vi.fn()
    notify.bindNotificationTap(onopen)
    expect(onAction).toHaveBeenCalledOnce()
    captured?.({ extra: { issue_key: 'GDK-2' } })
    expect(onopen).toHaveBeenCalledWith('GDK-2')
  })

  it('routes a keyless tap to the queue (null)', () => {
    let captured: ((n: unknown) => void) | undefined
    onAction.mockImplementation((cb: (n: unknown) => void) => {
      captured = cb
    })
    const onopen = vi.fn()
    notify.bindNotificationTap(onopen)
    captured?.({})
    expect(onopen).toHaveBeenCalledWith(null)
  })

  it('ignores a non-string issue_key instead of trusting it', () => {
    let captured: ((n: unknown) => void) | undefined
    onAction.mockImplementation((cb: (n: unknown) => void) => {
      captured = cb
    })
    const onopen = vi.fn()
    notify.bindNotificationTap(onopen)
    captured?.({ extra: { issue_key: 7 } })
    expect(onopen).toHaveBeenCalledWith(null)
  })

  it('rebinding replaces the handler without stacking plugin listeners', () => {
    let captured: ((n: unknown) => void) | undefined
    onAction.mockImplementation((cb: (n: unknown) => void) => {
      captured = cb
    })
    const first = vi.fn()
    const second = vi.fn()
    notify.bindNotificationTap(first)
    notify.bindNotificationTap(second)
    expect(onAction).toHaveBeenCalledOnce()
    captured?.({ extra: { issue_key: 'GDK-3' } })
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledWith('GDK-3')
  })
})

describe('ensurePermission', () => {
  it('does not ask again when already granted', async () => {
    isPermissionGranted.mockResolvedValue(true)
    await expect(notify.ensurePermission()).resolves.toBe(true)
    expect(requestPermission).not.toHaveBeenCalled()
  })

  it('asks once when not granted and reports the verdict', async () => {
    isPermissionGranted.mockResolvedValue(false)
    requestPermission.mockResolvedValue('granted')
    await expect(notify.ensurePermission()).resolves.toBe(true)
    expect(requestPermission).toHaveBeenCalledOnce()
  })

  it('reports false when the user denies', async () => {
    isPermissionGranted.mockResolvedValue(false)
    requestPermission.mockResolvedValue('denied')
    await expect(notify.ensurePermission()).resolves.toBe(false)
  })
})
