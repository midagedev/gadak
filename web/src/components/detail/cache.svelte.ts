/*
 * 상세(detail) 응답 캐시 + prefetch ([detail]).
 *
 * detail API 는 온디맨드라 선택할 때마다 왕복이 생긴다. 같은 이슈를 다시 열거나
 *  리스트에서 hover 로 미리 당겨두면(prefetch) 체감 레이턴시를 없앨 수 있다.
 *
 * - 모듈 레벨 Map 캐시(최대 50건 LRU: Map 삽입 순서를 활용해 가장 오래된 키를 버린다).
 * - 인플라이트 요청도 캐시해 동일 키 동시 요청을 1회로 합친다.
 * - 이슈 updated_at 이 바뀌면 explore/detail 쪽에서 invalidate(key) 로 무효화한다.
 */

import type { DetailComment, DetailResponse } from '../../lib/types'
import { getDetail } from '../../lib/api'

const MAX = 50

/** key(issue_key) → 완료된 응답. Map 삽입 순서 = LRU 순서. */
const cache = new Map<string, DetailResponse>()
/** key → 진행 중 Promise. 중복 요청 합류용. */
const inflight = new Map<string, Promise<DetailResponse>>()

/** LRU: 최근 사용 키를 맨 뒤로 옮긴다. */
function touch(key: string, value: DetailResponse): void {
  cache.delete(key)
  cache.set(key, value)
  // 용량 초과 시 가장 오래된(맨 앞) 키부터 제거
  while (cache.size > MAX) {
    const oldest = cache.keys().next().value
    if (oldest === undefined) break
    cache.delete(oldest)
  }
}

/**
 * 캐시 우선 상세 조회. 캐시에 있으면 즉시(resolved Promise), 없으면 fetch 후 캐시.
 * 동일 키 인플라이트가 있으면 그 Promise 에 합류한다.
 */
export function getDetailCached(key: string): Promise<DetailResponse> {
  const hit = cache.get(key)
  if (hit) {
    touch(key, hit) // LRU 갱신
    return Promise.resolve(hit)
  }
  const pending = inflight.get(key)
  if (pending) return pending

  const p = getDetail(key)
    .then((data) => {
      touch(key, data)
      return data
    })
    .finally(() => {
      inflight.delete(key)
    })
  inflight.set(key, p)
  return p
}

/**
 * fire-and-forget prefetch. 결과를 기다리지 않고 캐시만 데운다.
 * (hover 등에서 호출; 실패는 조용히 무시)
 */
export function prefetchDetail(key: string): void {
  if (cache.has(key) || inflight.has(key)) return
  void getDetailCached(key).catch(() => {})
}

/** 특정 키 무효화(이슈 updated_at 변경 시). */
export function invalidate(key: string): void {
  cache.delete(key)
  inflight.delete(key)
}

/** 전체 무효화(테스트/재로그인 등). */
export function invalidateAll(): void {
  cache.clear()
  inflight.clear()
}

/**
 * 캐시된 상세에 코멘트를 낙관적으로 덧붙인다(코멘트 작성 성공 시).
 * 캐시에 없으면(아직 미로드) noop — 다음 로드에서 서버가 최신 코멘트를 준다.
 */
export function appendComment(key: string, comment: DetailComment): void {
  const hit = cache.get(key)
  if (!hit) return
  cache.set(key, { ...hit, comments: [...hit.comments, comment] })
}
