import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { dismissToast, showToast, toastHost } from './toast.svelte'

/*
 * Recurrence layer for GDK-1504: the app has exactly one toast surface, and
 * a clipboard refusal is never silent again. Measured incident: AdfBody's
 * copyHref catch was empty (git show HEAD:mobile/src/ui/AdfBody.svelte,
 * scratch/w3-phone/gdk1504-fail-first.txt) while success had its own private
 * `.copied` field — so every future announcer would have copied the field,
 * and the one failure worth announcing had nothing to announce through.
 *
 * FAIL-first evidence: at HEAD~ the assertions below are red the moment
 * they are stated — the store did not exist, App.svelte had no ToastHost
 * (grep -c ToastHost → 0), and AdfBody's catch showed no toast.
 */

const HERE = dirname(fileURLToPath(import.meta.url))

describe('GDK-1504 the toast store: one host, timers owned beside the state', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    for (const toast of [...toastHost.toasts]) dismissToast(toast.id)
  })
  afterEach(() => vi.useRealTimers())

  it('showToast adds one toast and auto-dismisses after the kind’s cadence', () => {
    showToast('Copied', 'success')
    expect(toastHost.toasts).toHaveLength(1)
    expect(toastHost.toasts[0]).toMatchObject({ kind: 'success', message: 'Copied' })
    vi.advanceTimersByTime(2500)
    expect(toastHost.toasts).toEqual([])
  })

  it('an error toast outlives a success toast — 6s against 2.5s', () => {
    showToast('ok', 'success')
    showToast('no', 'error')
    vi.advanceTimersByTime(2500)
    expect(toastHost.toasts.map((t) => t.kind)).toEqual(['error'])
    vi.advanceTimersByTime(3500)
    expect(toastHost.toasts).toEqual([])
  })

  it('dismissToast is the early way out and is idempotent', () => {
    showToast('no', 'error')
    const id = toastHost.toasts[0].id
    dismissToast(id)
    expect(toastHost.toasts).toEqual([])
    // The armed timer must not resurrect the toast it belonged to.
    vi.advanceTimersByTime(6000)
    expect(toastHost.toasts).toEqual([])
    dismissToast(id) // no throw, no state change
  })

  it('ids are unique across toasts', () => {
    showToast('a', 'info')
    showToast('b', 'info')
    expect(new Set(toastHost.toasts.map((t) => t.id)).size).toBe(2)
  })
})

describe('GDK-1504 AdfBody announces both outcomes on the shared host', () => {
  // Source scan, same family as refusal.test.ts: the component is not
  // mounted here; the contract is stated against the code that renders it.
  const adf = readFileSync(join(HERE, '../ui/AdfBody.svelte'), 'utf8')
  const app = readFileSync(join(HERE, '../App.svelte'), 'utf8')
  const host = readFileSync(join(HERE, '../ui/ToastHost.svelte'), 'utf8')

  it('the private copied field is gone — no second copy of the host', () => {
    expect(adf).not.toContain('copiedTimer')
    expect(adf).not.toContain('let copied')
    expect(adf).not.toContain('class="copied"')
  })

  it('success toasts detail.linkCopied — the desk’s own word (§3.6)', () => {
    const copyHref = adf.slice(adf.indexOf('async function copyHref'), adf.indexOf('async function copyHref') + 600)
    expect(copyHref).toContain("showToast(t('detail.linkCopied'), 'success')")
  })

  it('a clipboard refusal is not silent: the desk’s failure sentence, as an error', () => {
    const copyHref = adf.slice(adf.indexOf('async function copyHref'), adf.indexOf('async function copyHref') + 900)
    expect(copyHref).toContain("showToast(t('clipboard.copyFailed'), 'error')")
    // And it is inside the catch, not just anywhere in the function.
    const catchBlock = copyHref.slice(copyHref.indexOf('catch'))
    expect(catchBlock).toContain('showToast')
  })

  it('App.svelte mounts the one host', () => {
    expect(app.match(/<ToastHost/g)?.length).toBe(1)
  })

  it('the host renders error as an alert, everything else as a status', () => {
    expect(host).toContain(`toast.kind === 'error' ? 'alert' : 'status'`)
    expect(host).toContain(`toast.kind === 'error' ? 'assertive' : 'polite'`)
  })
})
