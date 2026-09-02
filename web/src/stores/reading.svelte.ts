/*
 * Reading width for the detail column (GDK-1311).
 *
 * The docked detail track is a third of the window, sized for scanning an
 * issue next to its list. A long description read in it wraps Korean prose
 * every ~30 characters. `wide` hands the column the width instead: the list
 * keeps its minimum track and the detail takes the rest. It is a display
 * preference of this browser — not a place, so it does not touch the URL —
 * and it persists like the sidebar's collapsed sections.
 */
import { STORAGE_KEYS } from '../lib/storage'

function load(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEYS.detailWide) === '1'
  } catch {
    return false
  }
}

// Read from storage until the first toggle, not at import: the storage key
// is partitioned by the site in config.json, which loads after this module
// is evaluated — a module-level read would write under one key and read
// back under another. The getter does not cache into state either: it is
// read inside templates, and writing state from a template evaluation is
// what Svelte 5 refuses as an unsafe mutation.
let wide = $state<boolean | null>(null)

export const reading = {
  get wide(): boolean {
    return wide ?? load()
  },
  toggle(): void {
    wide = !reading.wide
    try {
      if (wide) localStorage.setItem(STORAGE_KEYS.detailWide, '1')
      else localStorage.removeItem(STORAGE_KEYS.detailWide)
    } catch {
      /* storage unavailable: the toggle still works for this page */
    }
  },
}
