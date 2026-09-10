/*
 * The human label for one linked issue — the web's half of retiring the raw
 * direction token (GDK-1215).
 *
 * `direction` is a wire value (`inward` / `outward`, the pair the add form and
 * the dedupe key are built on) and must never reach a person's eyes. The
 * sentence to show is the link type's own, for this side: the backend renders
 * it from the mirror's catalog and sends it as `phrase` (origin.LinkPhrase is
 * that single owner), so this function only decides which of the available
 * answers to use.
 *
 * The ladder, in order: the backend's phrase; the client-side catalog the add
 * form already fetched (an older backend, or a type the mirror's catalog
 * lacks — GDK-1293); the type name; and finally the caller's generic word.
 * There is no rung that renders `direction`. A file-local ternary used to end
 * in `: dir`, which put "outward" on screen for any direction spelling the
 * two named cases missed, while the comment above it claimed the token was
 * never shown.
 */

export interface LinkTypeRow {
  name: string
  inward?: string | null
  outward?: string | null
}

export interface LinkLabelInput {
  type?: string | null
  direction?: string | null
  phrase?: string | null
}

/** The label to render, given what the backend sent and the local catalog. */
export function linkLabel(
  link: LinkLabelInput,
  types: LinkTypeRow[],
  fallback: string,
): string {
  const backend = (link.phrase ?? '').trim()
  if (backend) return backend

  const type = (link.type ?? '').trim()
  const dir = (link.direction ?? '').trim().toLowerCase()
  const row = types.find((r) => r.name.toLowerCase() === type.toLowerCase())
  const phrase = dir === 'inward' ? row?.inward : dir === 'outward' ? row?.outward : undefined
  return (phrase ?? '').trim() || type || fallback
}
