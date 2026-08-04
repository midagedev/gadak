/*
 * Issue Navigator — IndexedDB 캐시 (idb)
 *
 * Linear 식 "브라우저를 DB로" 전략의 영속 계층. 재방문 시 네트워크 전에
 * 이 캐시에서 메모리 풀을 하이드레이션해 cold start 를 없앤다.
 *
 * 스토어:
 *  - issues: 키 = issue_key (IssueLite 전량)
 *  - meta:   키 = 'sync' (server_time / sync_version / members 단일 레코드)
 */

import { openDB, type DBSchema, type IDBPDatabase } from 'idb'
import type { CacheMeta, IssueLite, WriteMetaCache } from './types'

const DB_NAME = 'issue-navigator'
const DB_VERSION = 1

interface IssueDB extends DBSchema {
  issues: {
    key: string // issue_key
    value: IssueLite
  }
  meta: {
    key: string // 'sync' | 'write'
    value: CacheMeta | WriteMetaCache
  }
}

/**
 * open 이 이 시간 안에 끝나지 않으면 실패로 간주한다.
 * IndexedDB open 은 다른 탭의 pending delete/upgrade 뒤에 "조용히 큐잉"될 수 있어
 * (에러도 이벤트도 없이 영원히 대기) 타임아웃 없이는 앱 전체가 스켈레톤에 갇힌다.
 * 실패해도 호출부(스토어)가 잡아서 네트워크 전용(메모리 풀)으로 동작한다.
 */
const OPEN_TIMEOUT_MS = 2_000

let dbPromise: Promise<IDBPDatabase<IssueDB>> | null = null

function db(): Promise<IDBPDatabase<IssueDB>> {
  if (!dbPromise) {
    const opening = openDB<IssueDB>(DB_NAME, DB_VERSION, {
      upgrade(database) {
        if (!database.objectStoreNames.contains('issues')) {
          database.createObjectStore('issues', { keyPath: 'issue_key' })
        }
        if (!database.objectStoreNames.contains('meta')) {
          database.createObjectStore('meta', { keyPath: 'key' })
        }
      },
      // 다른 탭이 상위 버전으로 업그레이드(또는 삭제)하려 할 때 커넥션을 양보한다.
      // 이게 없으면 DB_VERSION 을 올려 배포하는 순간, 구버전 탭을 열어둔 모든
      // 사용자의 새 탭이 업그레이드 대기에 걸려 무한 스켈레톤이 된다.
      blocking() {
        void opening.then((conn) => conn.close()).catch(() => {})
        dbPromise = null // 이 탭은 이후 메모리 전용으로 동작, 리로드 시 새 스키마 사용
      },
      terminated() {
        dbPromise = null // 브라우저가 강제 종료한 커넥션 — 다음 호출에서 재오픈
      },
    })
    dbPromise = Promise.race([
      opening,
      new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error('indexeddb-open-timeout')), OPEN_TIMEOUT_MS),
      ),
    ])
    // 타임아웃으로 실패해도 늦게나마 열렸다면 다음 호출이 쓸 수 있게 교체하고,
    // 진짜 실패면 리셋해서 재시도 여지를 남긴다.
    dbPromise.catch(() => {
      opening.then(
        (conn) => {
          dbPromise = Promise.resolve(conn)
        },
        () => {
          dbPromise = null
        },
      )
    })
  }
  return dbPromise
}

/** 캐시된 전체 이슈 로드 (하이드레이션용). */
export async function getAllIssues(): Promise<IssueLite[]> {
  return (await db()).getAll('issues')
}

/** 이슈 다건 upsert (bootstrap 전량 또는 delta upserted). 한 트랜잭션으로 처리. */
export async function putIssues(issues: IssueLite[]): Promise<void> {
  if (issues.length === 0) return
  const conn = await db()
  const tx = conn.transaction('issues', 'readwrite')
  const store = tx.objectStore('issues')
  for (const issue of issues) store.put(issue)
  await tx.done
}

/** 이슈 다건 삭제 (delta deleted_keys). */
export async function deleteIssues(keys: string[]): Promise<void> {
  if (keys.length === 0) return
  const conn = await db()
  const tx = conn.transaction('issues', 'readwrite')
  const store = tx.objectStore('issues')
  for (const key of keys) store.delete(key)
  await tx.done
}

/** bootstrap 전량으로 스토어를 교체(오래된 tombstone 잔재 제거). */
export async function replaceAllIssues(issues: IssueLite[]): Promise<void> {
  const conn = await db()
  const tx = conn.transaction('issues', 'readwrite')
  const store = tx.objectStore('issues')
  await store.clear()
  for (const issue of issues) store.put(issue)
  await tx.done
}

export async function getMeta(): Promise<CacheMeta | undefined> {
  return (await db()).get('meta', 'sync') as Promise<CacheMeta | undefined>
}

export async function putMeta(meta: CacheMeta): Promise<void> {
  await (await db()).put('meta', meta)
}

/** 쓰기 메타(transition 맵/create-meta) 캐시 — 부팅 시 로드해 드롭다운을 0ms 로 만든다. */
export async function getWriteMeta(): Promise<WriteMetaCache | undefined> {
  return (await db()).get('meta', 'write') as Promise<WriteMetaCache | undefined>
}

export async function putWriteMeta(meta: WriteMetaCache): Promise<void> {
  await (await db()).put('meta', meta)
}

/** 전체 비우기 (수동 리셋/디버그용). */
export async function clearAll(): Promise<void> {
  const conn = await db()
  await conn.clear('issues')
  await conn.clear('meta')
}
