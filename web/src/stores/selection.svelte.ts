// 선택 이슈 스토어 — 탐색(explore)이 select/clear 를 호출하고, 상세 패널(detail)이 구독한다.
// URL(?issue=KEY) 반영은 explore 쪽 책임 (계약 §2).

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
