/*
 * Shared user-search rune factory.
 *
 * Five call sites (assignee picker, bulk assignee menu, new-issue dialog, QA
 * field editor, comment mentions) all hit GET users/ with the same debounce +
 * race-guard needs.
 * Debounce, sequence guard (only the latest request wins), and $effect cleanup
 * live here once; each site keeps its own debounceMs / minLength / error handling.
 */

import { searchUsers } from './api'
import type { JiraUser } from './types'

export interface UserSearchOptions {
  /** Default 250. */
  debounceMs?: number
  /** Default 2. Queries shorter than this clear results without fetching. */
  minLength?: number
  onError?: (e: unknown) => void
  /** Fired only for the latest successful response (after race guard). */
  onResults?: (users: JiraUser[]) => void
}

export interface UserSearch {
  readonly results: JiraUser[]
  readonly searching: boolean
}

/**
 * Reactive user search. `getQuery` is read inside an $effect so any rune it
 * touches (e.g. a `$state` query string) re-runs the debounce cycle.
 */
export function createUserSearch(
  getQuery: () => string,
  opts: UserSearchOptions = {},
): UserSearch {
  const debounceMs = opts.debounceMs ?? 250
  const minLength = opts.minLength ?? 2

  let results = $state<JiraUser[]>([])
  let searching = $state(false)
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | null = null

  $effect(() => {
    const q = getQuery().trim()

    if (timer) {
      clearTimeout(timer)
      timer = null
    }

    if (q.length < minLength) {
      results = []
      searching = false
      return
    }

    timer = setTimeout(() => {
      const my = ++seq
      searching = true
      void searchUsers(q)
        .then((res) => {
          if (my !== seq) return
          results = res.users
          opts.onResults?.(res.users)
        })
        .catch((e) => {
          if (my !== seq) return
          results = []
          opts.onError?.(e)
        })
        .finally(() => {
          if (my === seq) searching = false
        })
    }, debounceMs)

    return () => {
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
      // Drop any in-flight response for the previous query.
      seq++
    }
  })

  return {
    get results() {
      return results
    },
    get searching() {
      return searching
    },
  }
}
