// Selected-issue store — explore calls select/clear; detail panel subscribes.
// URL (?issue=KEY) sync is explore's job (contract §2).

class SelectionStore {
	selectedKey = $state<string | null>(null)

	select(key: string) {
		this.selectedKey = key
	}

	clear() {
		this.selectedKey = null
	}

	toggle(key: string) {
		this.selectedKey = this.selectedKey === key ? null : key
	}
}

export const selection = new SelectionStore()
