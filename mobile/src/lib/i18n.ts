/*
 * Vocabulary has one owner: the desktop catalog (DESIGN.md §3.6).
 *
 * This module is the *only* place in the phone that reaches across the tree
 * into web/. Everything else imports `./i18n` (or `../lib/i18n`), so the
 * relative depth of the crossing is written once and the rule — "the phone
 * never authors a word the desk already owns" — has a single seam to guard.
 *
 * web/src/lib/i18n is pure TS: it reads localStorage / navigator.language and
 * stamps <html lang>, all of which the webview supports. It calls initLocale()
 * at import time so t() is correct even before App.svelte's explicit call.
 */

// fieldLabel rides the same seam (GDK-875): the Detail meta line labels the
// due date with whatever the desk's field renderer calls that field — the
// catalog's field.due, trilingual already, so no phone-authored word exists.
export { collator, fieldLabel, initLocale, locale, setLocale, t } from '../../../web/src/lib/i18n'
export type { Locale, MessageKey } from '../../../web/src/lib/i18n'
