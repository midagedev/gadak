/*
 * "The reader asked to save the current view" — a one-shot latch between the
 * palette and the view-settings menu (GDK-732).
 *
 * The palette cannot name a view: its input is a search box, and a row whose
 * label echoes whatever is typed collides with every other row that quotes
 * the query (e2e/omnibox.spec.ts had to skip instant-create for exactly that
 * reason; a second echoing row made the skip insufficient). So the palette
 * does what it already does for triage — `requestMenu` — and hands off to the
 * affordance that owns naming, rather than growing a second one.
 *
 * One-shot: the menu clears the flag as it opens, so a later re-render does
 * not reopen a popover the reader dismissed.
 */
class SaveViewRequest {
  #pending = $state(false)

  get pending(): boolean {
    return this.#pending
  }

  /** Ask the list toolbar's view-settings menu to open on its save popover. */
  request(): void {
    this.#pending = true
  }

  /** Consumed by the menu. */
  clear(): void {
    this.#pending = false
  }
}

export const saveViewRequest = new SaveViewRequest()
