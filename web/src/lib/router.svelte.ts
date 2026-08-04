/*
 * Issue Navigator — 경량 해시 라우터
 *
 * ⚠️ 파일명 주의: 계약 §2 는 `router.ts` 로 적었으나, 현재 해시를 `$state`(runes) 로
 *  노출하려면 파일이 `.svelte.ts` 여야 컴파일러가 룬을 처리한다(순수 `.ts` 에서는 룬 불가).
 *  따라서 `router.svelte.ts` 로 두고, import 는 `./lib/router.svelte` 로 한다.
 *
 * 책임(파운데이션 범위): 현재 해시를 반응형으로 노출 + `{ path, params }` 파싱/직렬화
 *  + 범용 쿼리파람 get/set. **뷰 상태(필터/디스플레이) 직렬화는 [explore] 소관** —
 *  여기서는 어떤 키에 무슨 뜻이 있는지 해석하지 않는다.
 */

export interface Route {
  /** 해시의 경로부 (선행 `#` 제거, `?` 이전). 기본 `/`. 예: `/board` */
  path: string
  /** 경로 뒤 쿼리스트링을 파싱한 값. */
  params: URLSearchParams
}

function parseHash(hash: string): Route {
  // location.hash 는 "#..." 또는 "" 형태
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  const qIndex = raw.indexOf('?')
  const path = (qIndex === -1 ? raw : raw.slice(0, qIndex)) || '/'
  const query = qIndex === -1 ? '' : raw.slice(qIndex + 1)
  return { path: path.startsWith('/') ? path : '/' + path, params: new URLSearchParams(query) }
}

/** Route → 해시 문자열 (`#/path?a=b`). 빈 쿼리면 `?` 를 붙이지 않는다. */
export function serialize(route: Route): string {
  const qs = route.params.toString()
  return '#' + route.path + (qs ? '?' + qs : '')
}

// 현재 해시 상태 (반응형). hashchange 로 동기화된다.
let current = $state<Route>(parseHash(typeof location !== 'undefined' ? location.hash : ''))

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', () => {
    current = parseHash(location.hash)
  })
}

/**
 * 현재 라우트에 반응형으로 접근. 컴포넌트/`$derived` 안에서 `router.path`,
 * `router.params` 를 읽으면 해시 변경 시 자동 갱신된다.
 */
export const router = {
  get path(): string {
    return current.path
  },
  get params(): URLSearchParams {
    return current.params
  },
}

/** 해시를 통째로 교체(경로 + 파람). history 항목을 새로 쌓는다. */
export function navigate(path: string, params?: URLSearchParams | Record<string, string>): void {
  const sp =
    params instanceof URLSearchParams
      ? params
      : new URLSearchParams((params ?? {}) as Record<string, string>)
  location.hash = serialize({ path, params: sp })
}

/** 단일 쿼리파람 읽기 (없으면 null). */
export function getParam(key: string): string | null {
  return current.params.get(key)
}

/**
 * 쿼리파람 다건 병합 갱신 — 나머지 파람은 보존한다.
 * 값이 `null` 이면 해당 키를 제거. 경로는 그대로 유지.
 * `replace=true` 면 history 스택을 쌓지 않고 현재 항목을 대체(스크롤/타이핑 중 유용).
 */
export function setParams(next: Record<string, string | null>, replace = false): void {
  const sp = new URLSearchParams(current.params)
  for (const [k, v] of Object.entries(next)) {
    if (v === null) sp.delete(k)
    else sp.set(k, v)
  }
  const hash = serialize({ path: current.path, params: sp })
  if (replace) {
    history.replaceState(null, '', hash)
    // replaceState 는 hashchange 를 발화하지 않으므로 수동 동기화
    current = parseHash(hash)
  } else {
    location.hash = hash
  }
}

/** 단일 쿼리파람 설정(설탕). */
export function setParam(key: string, value: string | null, replace = false): void {
  setParams({ [key]: value }, replace)
}
