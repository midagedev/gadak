/*
 * One queue for every "instant local apply, then read-modify-write the
 * settings document" write-through (theme picker, terminal appearance,
 * the ⌘K theme action). Two pickers clicked in a row each GET the document
 * and PUT it back whole; unserialized, the second GET can race the first
 * PUT and the first change is lost. One chain, so each write starts from
 * the document the previous one left.
 */

import { getSettings, putSettings, type GadakSettings } from './api'
import { isHostedDemo } from './config'

let chain: Promise<void> = Promise.resolve()

/** Apply `patch` to the current document and PUT the result. Rejects with the
 *  transport error; the chain itself survives a failure. Hosted demo has no
 *  settings server, so it resolves without a request. */
export function queueSettingsPatch(
  patch: (current: GadakSettings) => GadakSettings,
): Promise<void> {
  const run = chain.then(
    () => writeThrough(patch),
    () => writeThrough(patch),
  )
  chain = run.catch(() => {
    /* keep the chain alive after a failure */
  })
  return run
}

async function writeThrough(patch: (current: GadakSettings) => GadakSettings): Promise<void> {
  if (isHostedDemo()) return
  const current = await getSettings()
  await putSettings(patch(current))
}

/** A look write-through: the value already applied locally; if the server
 *  does not take it, say so once with `toastKey` instead of failing
 *  silently. One road for the theme picker and the dock appearance (they
 *  had two copies of it — v0.20 audit, GDK-1377). */
export async function writeThroughLook(
  patch: (current: GadakSettings) => GadakSettings,
  toastKey: 'theme.savedLocally' | 'settings.terminalSavedLocally',
  warnLabel: string,
): Promise<void> {
  try {
    await queueSettingsPatch(patch)
  } catch {
    try {
      const { write } = await import('../stores/write.svelte')
      const { t } = await import('./i18n')
      write.toast(t(toastKey), 'info')
    } catch {
      console.warn(`gadak: ${warnLabel} saved locally only`)
    }
  }
}
