/*
 * Shared async resource rune factory.
 *
 * Panel loaders (issue detail, document detail, …) share the same pattern:
 * key change → async reload, generation race guard, errorKind mapping, clear
 * when the key is empty. Optional `watch` re-triggers without changing the key
 * (e.g. write.detailNonce after a comment lands in the detail cache).
 */

import { ApiError } from './api'
import { reachability } from './reachability.svelte'

export type ResourceErrorKind = 'notfound' | 'network'

export interface ResourceOptions {
  /** Extra reactive deps that force a reload while the key is unchanged. */
  watch?: () => unknown
}

export interface Resource<T> {
  readonly data: T | null
  readonly errorKind: null | ResourceErrorKind
  readonly loading: boolean
  reload(): void
}

export function createResource<T>(
  getKey: () => string | null | undefined,
  loader: (key: string) => Promise<T>,
  opts?: ResourceOptions,
): Resource<T> {
  let data = $state<T | null>(null)
  let errorKind = $state<null | ResourceErrorKind>(null)
  let loading = $state(false)
  let gen = 0

  async function load(k: string): Promise<void> {
    const my = ++gen
    errorKind = null
    loading = true
    try {
      const d = await loader(k)
      if (my !== gen) return
      data = d
    } catch (e) {
      if (my !== gen) return
      const status = e instanceof ApiError ? e.status : 0
      errorKind = status === 404 ? 'notfound' : 'network'
      data = null
    } finally {
      if (my === gen) loading = false
    }
  }

  function reload(): void {
    const k = getKey()
    if (k) void load(k)
  }

  $effect(() => {
    const k = getKey()
    // Subscribe to optional extra deps (detailNonce, etc.).
    opts?.watch?.()
    if (!k) {
      gen++ // invalidate in-flight
      data = null
      errorKind = null
      loading = false
      return
    }
    void load(k)
  })

  // GDK-477: when the offline banner clears, retry a network-failed panel once.
  // 404 stays put — the issue is gone, the server is not.
  let wasOffline = false
  $effect(() => {
    const down = reachability.offline
    if (wasOffline && !down && errorKind === 'network') reload()
    wasOffline = down
  })

  return {
    get data() {
      return data
    },
    get errorKind() {
      return errorKind
    },
    get loading() {
      return loading
    },
    reload,
  }
}
