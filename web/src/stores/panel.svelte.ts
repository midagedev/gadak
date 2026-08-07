/*
 * The right panel — what is open in it, as one value.
 *
 * The panel holds one thing at a time: an issue, a document, or a person. That
 * used to be an agreement rather than a fact. Three stores each held a piece of
 * it (`selection.selectedKey`, `pages.selectedKey`, `person.selectedEmail`) and
 * cleared the others on the way in, so "one panel" had to be restated at every
 * entry point — five of them, two living in App as effects because a store
 * could not close what it displaced without importing the store that owned it.
 * Those imports were also a cycle: person → pages → selection.
 *
 * Here it is a single discriminated union, so exclusivity is the shape of the
 * state instead of a rule about it: assigning one target is what closes the
 * other, and there is no order in which two can be open. The three stores keep
 * their public surface — `selectedKey` / `select()` / `clear()` — as views onto
 * this value, so nothing that opens a panel had to learn a new call.
 *
 * Only "what is on the right" lives here. Everything else those stores own (the
 * page index, the document screens, a person's comment list) stays with them.
 */

/** The three kinds of thing the right panel can show. */
export type PanelKind = 'issue' | 'doc' | 'person'

/** What is open. `key` is an issue key, a page key, or a member's email —
 *  whatever identifies that kind to the store that renders it. */
export type PanelTarget = { kind: PanelKind; key: string }

class PanelStore {
  #target = $state<PanelTarget | null>(null)

  /**
   * Per-kind teardown, registered by the store that owns that kind.
   *
   * Only state a store keeps *for the currently open target* belongs here — a
   * request in flight for the open person, say. Registration runs from the
   * owning store's constructor, which is what keeps this file import-free and
   * the old cycle from growing back.
   */
  #onLeave = new Map<PanelKind, () => void>()

  /** What the panel is showing, or null when it is closed. */
  get target(): PanelTarget | null {
    return this.#target
  }

  /** The open key for one kind — null when something else, or nothing, is open.
   *  This is what each store's `selectedKey` reads. */
  keyOf(kind: PanelKind): string | null {
    const t = this.#target
    return t !== null && t.kind === kind ? t.key : null
  }

  onLeave(kind: PanelKind, fn: () => void): void {
    this.#onLeave.set(kind, fn)
  }

  /** Open one thing. Whatever was open closes — there is one value to hold it. */
  show(kind: PanelKind, key: string): void {
    const prev = this.#target
    if (prev !== null && prev.kind === kind && prev.key === key) return
    this.#target = { kind, key }
    if (prev !== null) this.#onLeave.get(prev.kind)?.()
  }

  /** Close, but only if this kind is what is open. A component's Close button
   *  asks about its own panel, and by the time the click lands something else
   *  may have taken the surface. */
  close(kind: PanelKind): void {
    if (this.#target?.kind !== kind) return
    this.#target = null
    this.#onLeave.get(kind)?.()
  }
}

export const panel = new PanelStore()
