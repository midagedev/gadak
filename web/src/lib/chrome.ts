/*
 * Chrome vocabulary — the class strings for the two roles an inline scrap of
 * text can play in a field row (GDK-142 V14).
 *
 * The audit found 'Unassigned' (a value that is empty) and 'Add a label' (an
 * action you can take) drawn in byte-identical classes: `text-text-muted
 * italic` on both. Nothing but the wording told a reader which one clicks.
 *
 *  - EMPTY_VALUE  — muted + italic. Italic is the app's "there is nothing
 *    here" mark; it never appears on something you can press.
 *  - INLINE_ACTION — the 쪽빛 accent ink, upright. Same thread as an issue
 *    key, which is the other thing in a row you can press.
 *
 * Both are exported as constants rather than spelled inline so the rule has a
 * single owner and lib/chrome-vocabulary.test.ts can assert that no component
 * re-spells either costume by hand.
 */

/** A field whose value is absent — 'Unassigned', 'None', an empty body. */
export const EMPTY_VALUE = 'text-text-muted italic'

/** An inline affordance inside a field row — 'Add a label'. Never italic. */
export const INLINE_ACTION = 'text-accent-text'
