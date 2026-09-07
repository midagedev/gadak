/*
 * One owner for "put text on the user's clipboard, honestly" (GDK-178).
 *
 * Desktop: navigator.clipboard is dead inside the wails webview — measured
 * on the installed build: writeText rejected while the old catch-and-confirm
 * idiom still toasted "copied". The pasteboard is reached through wails' own
 * runtime binding instead (GDK-1470). The app injects /wails/runtime.js into
 * every index.html it serves (desktop/main.go injectWailsRuntime), so the
 * module resolves on this same custom-scheme origin, and Clipboard.SetText
 * crosses the bridge the framework already owns — the same message a
 * hand-rolled HTTP route would have delivered to the same Go call.
 *
 * The dynamic import carries @vite-ignore so Vite neither bundles nor
 * rewrites the path: one built bundle serves `gadak serve` in a browser
 * (where the module does not exist and this branch never runs) and the app.
 *
 * Everywhere: the boolean is the truth. Callers toast success or failure
 * from the return value — never before it. A copy affordance that lies is
 * worse than one that fails aloud. Note what the boolean now means on
 * desktop: the runtime binding resolves once the message is delivered and
 * discards the pasteboard's own bool (wails messageprocessor_clipboard.go),
 * so false here is "the bridge refused", not "the pasteboard refused".
 */

import { isDesktop } from './config'
import { WAILS_RUNTIME_URL } from './terminal/wails-stream'

export async function copyText(text: string): Promise<boolean> {
  if (isDesktop()) {
    try {
      // Widened to `string` for the same reason loadWailsStreamFactory does
      // it: as a literal, TypeScript tries to resolve a module this
      // repository does not contain — the running app produces it.
      const url: string = WAILS_RUNTIME_URL
      const mod = (await import(/* @vite-ignore */ url)) as {
        Clipboard?: { SetText?: (text: string) => Promise<void> }
      }
      if (typeof mod.Clipboard?.SetText !== 'function') {
        throw new Error('wails runtime: no Clipboard.SetText export')
      }
      await mod.Clipboard.SetText(text)
      return true
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
