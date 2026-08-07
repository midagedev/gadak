// Selected-issue store — explore calls select/clear; detail panel subscribes.
// URL (?issue=KEY) sync is explore's job (contract §2).
//
// The key itself lives in the right-panel union (panel.svelte.ts): an open
// issue is one of the three things that panel can be showing, so it is read
// from there rather than held here. Opening an issue therefore closes an open
// document or person by construction, with nothing to clear.

import { panel } from './panel.svelte'

class SelectionStore {
	#key = $derived(panel.keyOf('issue'))

	get selectedKey(): string | null {
		return this.#key
	}

	select(key: string) {
		panel.show('issue', key)
	}

	clear() {
		panel.close('issue')
	}

	toggle(key: string) {
		if (this.selectedKey === key) panel.close('issue')
		else panel.show('issue', key)
	}
}

export const selection = new SelectionStore()
