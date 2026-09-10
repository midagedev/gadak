/*
 * The count beside a screen's name (GDK-1092 B-5, and the class GDK-1091 A-7
 * belongs to: two totals of different scope sitting side by side, neither
 * saying which scope it is).
 *
 * The Documents header said "Documents 0" while the wiki held 71 pages. Both
 * numbers were true: the badge counted the active tab (Viewed — the pages this
 * account has opened), the library had 71. Nothing on the screen said the
 * badge had been narrowed, so a first-run reader read it as the library.
 *
 * The rule, and the only thing this module is:
 *
 *   The denominator beside a screen's name is the LIBRARY total — one owner,
 *   the same meaning on every tab and under every filter. A number that has
 *   been narrowed is written as a fraction of it; a number that has not is
 *   written alone.
 *
 * Before this, the denominator was `tab === 'viewed' ? recentlyViewed.length
 * : index.length`, so the fraction the filter drew meant "of the library" on
 * two tabs and "of what you have opened" on the third — and the un-narrowed
 * Viewed tab drew no fraction at all, which is the read that broke.
 *
 * Formatting stays the caller's: locale digits come from lib/i18n, which this
 * module deliberately does not import so the rule can be unit-tested as
 * arithmetic.
 */

/**
 * The header badge for `shown` rows out of a library of `total`.
 * Equal (nothing has been narrowed) → the bare number; otherwise the fraction
 * that names the scope. `format` is the caller's number formatter.
 */
export function headerCount(
  shown: number,
  total: number,
  format: (n: number) => string,
): string {
  return shown === total ? format(shown) : `${format(shown)} / ${format(total)}`
}
