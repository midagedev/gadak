import { describe, expect, it } from 'vitest'
import { createDeepLinkRouter, type DeepLinkRefusal } from './deeplink-entry'

/*
 * GDK-873, the wiring half. deeplink.test.ts proves the decision; this
 * proves the two things the decision cannot know about — that a link
 * arriving before the app is ready is not dropped, and that a link which is
 * not ours produces no user-visible noise.
 */

function harness(ready = true) {
  const opened: string[] = []
  const refused: { refusal: DeepLinkRefusal; why: string }[] = []
  let isReady = ready
  const router = createDeepLinkRouter({
    openIssue: (key) => opened.push(key),
    ready: () => isReady,
    onRefused: (refusal, why) => refused.push({ refusal, why }),
  })
  return {
    router,
    opened,
    refused,
    become(readyNow: boolean) {
      isReady = readyNow
    },
  }
}

describe('GDK-873 — the deep-link router', () => {
  it('opens an issue when the app is ready', () => {
    const h = harness()
    h.router.deliver('gadak://view?issue=GDK-119')
    expect(h.opened).toEqual(['GDK-119'])
    expect(h.refused).toEqual([])
    expect(h.router.pending()).toBeNull()
  })

  // The cold-launch case: iOS hands over the URL that started the app while
  // the webview is still booting. Opening a detail screen then would push
  // over nothing, and dropping it would make the tap that launched the app
  // do nothing at all.
  it('holds a link that arrives before the app can show it', () => {
    const h = harness(false)
    h.router.deliver('gadak://view?issue=GDK-119')
    expect(h.opened).toEqual([])
    expect(h.router.pending()).toBe('GDK-119')

    h.become(true)
    h.router.flush()
    expect(h.opened).toEqual(['GDK-119'])
    expect(h.router.pending()).toBeNull()
  })

  it('flushing before readiness keeps holding rather than dropping', () => {
    const h = harness(false)
    h.router.deliver('gadak://view?issue=GDK-119')
    h.router.flush()
    expect(h.opened).toEqual([])
    expect(h.router.pending()).toBe('GDK-119')
  })

  it('flushing twice opens once', () => {
    const h = harness(false)
    h.router.deliver('gadak://view?issue=GDK-119')
    h.become(true)
    h.router.flush()
    h.router.flush()
    expect(h.opened).toEqual(['GDK-119'])
  })

  it('a second link supersedes the one still waiting', () => {
    // The user's last tap is the one they are waiting on; opening both
    // would push two detail screens they have to dismiss in turn.
    const h = harness(false)
    h.router.deliver('gadak://view?issue=GDK-119')
    h.router.deliver('gadak://view?issue=NMB-1')
    expect(h.router.pending()).toBe('NMB-1')
    h.become(true)
    h.router.flush()
    expect(h.opened).toEqual(['NMB-1'])
  })

  it('flushing with nothing held does nothing', () => {
    const h = harness()
    h.router.flush()
    expect(h.opened).toEqual([])
  })

  it('says nothing at all about another app’s scheme', () => {
    // A shared handler sees other schemes. Reporting them would put a toast
    // on the user's screen for an event that has nothing to do with gadak.
    const h = harness()
    h.router.deliver('myapp://view?issue=GDK-119')
    h.router.deliver('https://example.com/browse/GDK-119')
    h.router.deliver('')
    expect(h.opened).toEqual([])
    expect(h.refused).toEqual([])
  })

  it('reports a link that is ours and refused', () => {
    const h = harness()
    h.router.deliver('gadak://view/w/oss?issue=GDK-119')
    h.router.deliver('gadak://view?issue=GDK-119#panel')
    h.router.deliver('gadak://timeline?issue=GDK-119')
    expect(h.refused.map((r) => r.refusal)).toEqual(['other_workspace', 'malformed', 'unsupported_action'])
    for (const r of h.refused) expect(r.why.length).toBeGreaterThan(0)
    expect(h.opened).toEqual([])
  })

  it('does not hold a refused link', () => {
    // Holding a refusal would make it fire later, when the user has moved
    // on and no longer has any idea what caused it.
    const h = harness(false)
    h.router.deliver('gadak://view/w/oss?issue=GDK-119')
    expect(h.router.pending()).toBeNull()
    h.become(true)
    h.router.flush()
    expect(h.opened).toEqual([])
  })

  it('survives anything the OS can hand it', () => {
    const h = harness()
    for (const raw of ['', 'gadak:', 'gadak://', 'gadak://view?q=%', 'gadak://' + 'x'.repeat(9000)]) {
      expect(() => h.router.deliver(raw)).not.toThrow()
    }
    expect(h.opened).toEqual([])
  })

  it('works with no onRefused wired', () => {
    // App.svelte ships without one until the refusal copy is authored in
    // all three locales; a missing callback must not become a crash.
    const opened: string[] = []
    const router = createDeepLinkRouter({ openIssue: (k) => opened.push(k), ready: () => true })
    expect(() => router.deliver('gadak://view/w/oss?issue=GDK-119')).not.toThrow()
    expect(opened).toEqual([])
    router.deliver('gadak://view?issue=GDK-119')
    expect(opened).toEqual(['GDK-119'])
  })
})
