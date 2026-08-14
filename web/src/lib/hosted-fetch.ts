/*
 * Hosted-demo in-page fetch adapter (replaces the former snapshot service worker).
 *
 * Active only when VITE_HOSTED_DEMO=1 (compile-time) AND config().hostedDemo
 * (runtime). installHostedFetch() is called before loadConfig(); the wrapper
 * therefore checks isHostedDemo() per request so config.json itself is never
 * intercepted. Regular gadak serve / desktop bundles never set the flag.
 *
 * <img src> bypasses window.fetch, so bootstrap/detail JSON is walked and any
 * string matching …/api/v1/issues/{key}/attachments/{id}/content/ is rewritten
 * to {basePath()}attachments/{id}.
 */

import { basePath, isHostedDemo } from './config'

let installed = false

/** Live-shaped attachment URL → static file. Fresh regex each call (no /g lastIndex). */
const CONTENT_URL_RE =
  /(?:https?:\/\/[^/]+)?(?:\/[A-Za-z0-9._~-]*)*?\/api\/v1\/issues\/[A-Za-z0-9_-]+\/attachments\/([A-Za-z0-9_-]+)\/content\/?/

export function installHostedFetch(): void {
  if (import.meta.env.VITE_HOSTED_DEMO !== '1') return
  if (installed) return
  if (typeof window === 'undefined' || typeof window.fetch !== 'function') return
  installed = true

  const nativeFetch = window.fetch.bind(window)

  window.fetch = function hostedFetch(
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> {
    if (!isHostedDemo()) return nativeFetch(input, init)

    const href = requestURL(input)
    let parsed: URL
    try {
      parsed = new URL(href, window.location.href)
    } catch {
      return nativeFetch(input, init)
    }
    if (parsed.origin !== window.location.origin) {
      return nativeFetch(input, init)
    }
    const apiTail = matchAPI(parsed.pathname)
    if (apiTail === null) return nativeFetch(input, init)
    return handleAPI(nativeFetch, input, init, parsed, apiTail)
  }

  // Returning visitors still have the old snapshot service worker controlling
  // this origin. Fire-and-forget: an already-open tab keeps that controller
  // until reload.
  if (navigator.serviceWorker) {
    void navigator.serviceWorker.getRegistrations().then((rs) => {
      for (const r of rs) void r.unregister()
    })
  }
}

function requestURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

function requestMethod(input: RequestInfo | URL, init?: RequestInit): string {
  if (init?.method) return init.method.toUpperCase()
  if (typeof Request !== 'undefined' && input instanceof Request) {
    return input.method.toUpperCase()
  }
  return 'GET'
}

/** Path tail under {basePath()}api/v1/, or null. Mirrors the former worker matchAPI. */
function matchAPI(pathname: string): string | null {
  const prefix = `${basePath()}api/v1/`
  if (!pathname.startsWith(prefix)) return null
  return pathname.slice(prefix.length)
}

async function handleAPI(
  nativeFetch: typeof fetch,
  input: RequestInfo | URL,
  init: RequestInit | undefined,
  url: URL,
  apiTail: string,
): Promise<Response> {
  const method = requestMethod(input, init)

  if (method !== 'GET' && method !== 'HEAD') {
    return jsonResponse({ error: 'demo_read_only' }, 501)
  }

  if (apiTail === 'auth/me/' || apiTail === 'auth/me') {
    return jsonResponse({ email: null })
  }

  if (!apiTail.startsWith('issues/')) {
    return jsonResponse({ error: 'not_found' }, 404)
  }
  const tail = apiTail.slice('issues/'.length)
  // The former worker matched `delta?` against pathname (query never appears
  // there). Honour the same strings plus URL.search so `delta/?since=` works.
  const tailWithSearch = `${tail}${url.search}`

  if (tail === 'bootstrap/' || tail === 'bootstrap') {
    return staticFile(nativeFetch, 'bootstrap.json', 'application/json', true)
  }

  let m = /^([A-Za-z0-9_-]+)\/detail\/?$/.exec(tail)
  if (m) {
    return staticFile(nativeFetch, `detail/${m[1]}.json`, 'application/json', true)
  }

  m = /^([A-Za-z0-9_-]+)\/attachments\/([A-Za-z0-9_-]+)\/content\/?$/.exec(tail)
  if (m) {
    return staticFile(nativeFetch, `attachments/${m[2]}`, 'image/png', false)
  }

  if (
    tail === 'delta/' ||
    tail === 'delta' ||
    tailWithSearch.startsWith('delta?') ||
    tailWithSearch.startsWith('delta/?')
  ) {
    return jsonResponse({
      server_time: new Date().toISOString(),
      upserted: [],
      deleted_keys: [],
      members_version: '',
      sync_health: { overall: 'ok', sources: [] },
    })
  }
  if (tail === 'views/' || tail === 'views') {
    return jsonResponse({ views: [] })
  }
  if (tail === 'watches/' || tail === 'watches') {
    return jsonResponse({ keys: [] })
  }
  if (
    tail === 'feed/' ||
    tail === 'feed' ||
    tailWithSearch.startsWith('feed?') ||
    tailWithSearch.startsWith('feed/?')
  ) {
    return jsonResponse({
      items: [],
      unread_counts: { all: 0, mentions: 0, watched: 0, assigned: 0 },
    })
  }

  if (
    tail.startsWith('search/') ||
    tail === 'search' ||
    tail.startsWith('search?') ||
    tailWithSearch.startsWith('search?') ||
    tailWithSearch.startsWith('search/?')
  ) {
    return jsonResponse({ error: 'not_found' }, 404)
  }

  return jsonResponse({ error: 'not_found' }, 404)
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'no-store',
    },
  })
}

async function staticFile(
  nativeFetch: typeof fetch,
  rel: string,
  contentType: string,
  rewriteUrls: boolean,
): Promise<Response> {
  const fileURL = new URL(rel, `${window.location.origin}${basePath()}`).href
  try {
    const res = await nativeFetch(fileURL, { cache: 'force-cache' })
    if (!res.ok) {
      return jsonResponse({ error: 'not_found' }, 404)
    }
    if (rewriteUrls) {
      const parsed: unknown = await res.json()
      return new Response(JSON.stringify(rewriteContentUrls(parsed)), {
        status: 200,
        headers: {
          'Content-Type': contentType,
          'Cache-Control': 'public, max-age=3600',
        },
      })
    }
    const buf = await res.arrayBuffer()
    return new Response(buf, {
      status: 200,
      headers: {
        'Content-Type': contentType,
        'Cache-Control': 'public, max-age=3600',
      },
    })
  } catch {
    return jsonResponse({ error: 'not_found' }, 404)
  }
}

function rewriteContentUrls(value: unknown): unknown {
  if (typeof value === 'string') return rewriteContentUrlString(value)
  if (Array.isArray(value)) return value.map(rewriteContentUrls)
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      out[k] = rewriteContentUrls(v)
    }
    return out
  }
  return value
}

function rewriteContentUrlString(s: string): string {
  return s.replace(new RegExp(CONTENT_URL_RE.source, 'g'), (_m, id: string) => {
    return `${basePath()}attachments/${id}`
  })
}
