/*
 * Merged UI catalog: domain files under messages/ each export
 * Record<key, {en, ko, ja}>. Locale tables are derived once at load so
 * t() stays a dict lookup.
 */

import { detail } from './messages/detail'
import { common } from './messages/common'
import { fields } from './messages/fields'
import { list } from './messages/list'
import { personal } from './messages/personal'
import { settings } from './messages/settings'
import { shell } from './messages/shell'
import { write } from './messages/write'
import type { Locale, Message } from './types'

export type { Locale, Message }
export { LOCALES } from './types'

export const messages = {
  ...common,
  ...fields,
  ...list,
  ...shell,
  ...personal,
  ...detail,
  ...write,
  ...settings,
} as const satisfies Record<string, Message>

export type MessageKey = keyof typeof messages

function tableFor(locale: Locale): Record<MessageKey, string> {
  const out = {} as Record<MessageKey, string>
  for (const key of Object.keys(messages) as MessageKey[]) {
    out[key] = messages[key][locale]
  }
  return out
}

export const en = tableFor('en')
export const ko = tableFor('ko')
export const ja = tableFor('ja')
