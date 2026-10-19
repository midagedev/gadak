/*
 * The active locale as reactive state (GDK-2034).
 *
 * setLocale used to hard-reload the page so every module would re-read its
 * catalogue; that threw away whatever was not persisted (a half-written
 * issue, an open detail edit, a live terminal). Reading the locale inside a
 * template, `$derived` or `$effect` now subscribes to it, so every string
 * that reaches t() / localeTag() re-renders when it changes.
 *
 * Why createSubscriber and not a `$state` cell: the i18n runtime is imported
 * by nearly every unit test, and the default vitest project has no svelte
 * compiler on purpose (vitest.config.ts), so a runes module here would take
 * all of them down. `svelte/reactivity` is plain JS: in a component it is
 * the real tracker, in node it resolves to the server build where
 * subscribing is a no-op and a read is just a read.
 */

import { createSubscriber } from 'svelte/reactivity'
import type { Locale } from './types'

let current: Locale = 'en'
let notify: (() => void) | undefined

const subscribe = createSubscriber((update) => {
  notify = update
  return () => {
    notify = undefined
  }
})

export function getLocale(): Locale {
  subscribe()
  return current
}

export function writeLocale(next: Locale): void {
  if (next === current) return
  current = next
  notify?.()
}
