/*
 * One owner for "put text on the user's clipboard, honestly" (GDK-178).
 *
 * Desktop: navigator.clipboard is dead inside the wails webview — measured
 * on the installed build: writeText rejected while the old catch-and-confirm
 * idiom still toasted "copied". The webview posts to /desktop/clipboard
 * (desktop/main.go) and the app writes the system pasteboard.
 *
 * Everywhere: the boolean is the truth. Callers toast success or failure
 * from the return value — never before it. A copy affordance that lies is
 * worse than one that fails aloud.
 */

import { isDesktop } from './config'

export async function copyText(text: string): Promise<boolean> {
  if (isDesktop()) {
    try {
      const res = await fetch('/desktop/clipboard', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text }),
      })
      return res.status === 204
    } catch {
      return false
    }
  }
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}
