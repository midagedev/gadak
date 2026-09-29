/*
 * The status-category fold — one owner for the alias table and the
 * unknown-key answer (GDK-2042).
 *
 * There were three copies of this fold on the front end and no two agreed:
 * view-config's effectiveCategory knew `todo` and `completed`, the
 * grouper's groupCategory knew neither, and the phone's domain.ts folded
 * unknown keys to 'inprogress' while the web folded them to 'new'. So a
 * `completed` row sat in Done under a filter and in New under a group
 * header, and an unmirrored row read in progress on the phone and new on
 * the desk. The table below is the union of the three; every caller reads
 * it here.
 *
 * One entry did NOT survive the union: the grouper carried `'in progress'`,
 * the phrase with a space. That is a status DISPLAY NAME, not a category
 * key — no origin sends it (Jira's REST keys are new/indeterminate/done,
 * gadak's are new/inprogress/done, and neither demo fixture holds it), and
 * keying on a display name is the one thing this whole vocabulary exists to
 * prevent: a Korean account spells the same status 진행 중. It arrived by
 * inheritance, lifted unchanged out of the filters store with the grouper
 * (GDK-1993), never justified. Folding it made isUnattendedInProgress
 * recognize the English spelling of a name while refusing the Korean and
 * Japanese ones — the defect in miniature. It is dropped here (GDK-2042).
 *
 * Go owns the contract: internal/statuscat/category.go (Category /
 * KnownCategory) is the canonical fold and this file is its front-end
 * mirror, kept because saved-view status_category axes, raw transition
 * keys and mirrored rows arrive as raw REST keys the client must fold
 * without a round trip. The unknown-key answer is Go's — 'new', which can
 * only ever miss a reopen, never invent one. status-category.test.ts pins
 * the parity with the Go source. Never fold a status display name: they
 * are localized per account ("진행 중").
 */

/** The three buckets data-model.md documents — everything keys on these. */
export type StatusCategory = 'new' | 'inprogress' | 'done'

/**
 * Every key an origin or an older web mapper has been seen to send, onto
 * the three buckets. Compared lowercased. Every member is a REST key or a
 * gadak token — never a status display name (see the note above).
 */
export const CATEGORY_ALIASES: Readonly<Record<string, StatusCategory>> = {
  new: 'new',
  todo: 'new',
  inprogress: 'inprogress',
  indeterminate: 'inprogress',
  done: 'done',
  complete: 'done',
  completed: 'done',
}

/**
 * The category only when the key is one the table knows — the twin of Go's
 * KnownCategory (internal/statuscat/category.go). Null for an unknown or
 * empty key, so a caller can tell "folded" from "recognized".
 */
export function knownCategory(key: string | null | undefined): StatusCategory | null {
  return CATEGORY_ALIASES[(key ?? '').toLowerCase()] ?? null
}

/**
 * Fold a raw status_category key onto the three buckets. Unknown and empty
 * keys fold to 'new' — the same answer as internal/statuscat.Category: an
 * unknown key can only ever miss a reopen, never invent one.
 */
export function categoryOf(key: string | null | undefined): StatusCategory {
  return knownCategory(key) ?? 'new'
}
