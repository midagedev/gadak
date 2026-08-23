/*
 * Locale union and the per-key message shape.
 * A catalog entry is one object with every shipped locale; omitting a
 * field is a type error, not a runtime fallback.
 */

export const LOCALES = ['en', 'ko', 'ja'] as const
export type Locale = (typeof LOCALES)[number]

export type Message = {
  readonly en: string
  readonly ko: string
  readonly ja: string
}
