// Selected-issue store — explore calls select/clear; detail panel subscribes.
// URL (?issue=KEY) sync is explore's job (contract §2).
//
// The key itself lives in the right-panel union (panel.svelte.ts): an open
// issue is one of the three things that panel can be showing, so it is read
// from there rather than held here. Opening an issue therefore closes an open
// document or person by construction, with nothing to clear.

import { me } from './me.svelte'
import { panel, type PanelVia } from './panel.svelte'

class SelectionStore {
	#key = $derived(panel.keyOf('issue'))

	get selectedKey(): string | null {
		return this.#key
	}

	/* `via` is the surface that asked, carried through to the panel's debug
	 * trace (GDK-1186). Not a behaviour knob — nothing here branches on it —
	 * so a caller that forgets one still compiles and shows up as `?`. */
	select(key: string, via: PanelVia = '?') {
		/* Recording rides the write, not an effect watching it (GDK-941):
		 * App.svelte used to run two untrack effects over selectedKey, one
		 * reading+writing me.recent — the read-what-you-write shape. The
		 * guard mirrors panel.show's same-key early-return, so a re-select
		 * of the open issue records nothing, exactly as before. pages.select
		 * is the precedent (it records its docs visit inline). */
		if (this.selectedKey !== key) {
			me.recordRecent(key)
			void me.markIssueRead(key)
		}
		panel.show('issue', key, via)
	}

	clear(via: PanelVia = '?') {
		panel.close('issue', via)
	}

	toggle(key: string, via: PanelVia = '?') {
		if (this.selectedKey === key) panel.close('issue', via)
		else this.select(key, via)
	}
}

export const selection = new SelectionStore()
