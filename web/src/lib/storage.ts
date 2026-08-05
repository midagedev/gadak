/*
 * localStorage 키 헬퍼 + 1회 마이그레이션.
 * 옛 접두어 `issue-nav:` → `scry:`. 앱 부팅 시 한 번만 호출한다.
 */

/** 현재 사용 키. */
export const STORAGE_KEYS = {
  /** 폴백 전용 — 서버가 즐겨찾기를 받아주지 않는 환경(호스티드 데모)에서만 집합을 담는다. */
  favorites: 'scry:favorites',
  /**
   * 즐겨찾기 표시 순서. 집합의 주인은 서버(`favorites` 테이블)지만 그 테이블에는
   * 순서 컬럼이 없고 추가순으로만 돌려준다. 드래그로 만든 순서는 이 브라우저의
   * 표시 취향이라 여기 남긴다 — 집합과 순서의 주인이 다르다는 뜻이고, 의도한 것이다.
   */
  favoritesOrder: 'scry:favorites-order',
  recent: 'scry:recent',
  personalViews: 'scry:personal-views',
  lastView: 'scry:last-view',
} as const

/** recency.ts 용 접두어 (`scry:recent:` + kind). */
export const RECENT_KIND_PREFIX = 'scry:recent:'

/** 마이그레이션 전용 — 런타임 읽기/쓰기는 전부 scry: 를 쓴다. */
const LEGACY = {
  favorites: 'issue-nav:favorites',
  recent: 'issue-nav:recent',
  personalViews: 'issue-nav:personal-views',
  lastView: 'issue-nav:last-view',
  recentKindPrefix: 'issue-nav:recent:',
} as const

const EXACT_MIGRATIONS: [string, string][] = [
  [LEGACY.favorites, STORAGE_KEYS.favorites],
  [LEGACY.recent, STORAGE_KEYS.recent],
  [LEGACY.personalViews, STORAGE_KEYS.personalViews],
  [LEGACY.lastView, STORAGE_KEYS.lastView],
]

/**
 * 옛 `issue-nav:*` 키를 `scry:*` 로 1회 이관한다.
 * 새 키가 이미 있으면 값 덮어쓰지 않고 옛 키만 지운다. 되돌리지 않는다.
 * 사생활 모드 등 localStorage 예외는 조용히 무시(기존 호출부와 동일).
 */
export function migrateStorageKeys(): void {
  try {
    for (const [oldKey, newKey] of EXACT_MIGRATIONS) {
      const oldVal = localStorage.getItem(oldKey)
      if (oldVal === null) continue
      if (localStorage.getItem(newKey) === null) {
        localStorage.setItem(newKey, oldVal)
      }
      localStorage.removeItem(oldKey)
    }

    // recency 접두어 스캔: issue-nav:recent:<kind> → scry:recent:<kind>
    // (exact `issue-nav:recent` 는 위에서 이미 처리됨 — 콜론 접두어만 매칭)
    const legacyRecentKeys: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i)
      if (k && k.startsWith(LEGACY.recentKindPrefix)) legacyRecentKeys.push(k)
    }
    for (const oldKey of legacyRecentKeys) {
      const kind = oldKey.slice(LEGACY.recentKindPrefix.length)
      const newKey = RECENT_KIND_PREFIX + kind
      const oldVal = localStorage.getItem(oldKey)
      if (oldVal !== null && localStorage.getItem(newKey) === null) {
        localStorage.setItem(newKey, oldVal)
      }
      localStorage.removeItem(oldKey)
    }
  } catch {
    /* private mode / unavailable — ignore */
  }
}
