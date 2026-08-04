/* Hosted-demo service worker.
 *
 * Only shipped when VITE_HOSTED_DEMO=1. Intercepts fetch() under the app's
 * apiBase / authBase and rewrites them onto the static snapshot files that
 * `scry export-static` baked next to the UI:
 *
 *   bootstrap/                         → bootstrap.json
 *   {key}/detail/                      → detail/{key}.json
 *   {key}/attachments/{id}/content/    → attachments/{id}
 *   delta|views|watches|feed|me        → empty collection / null identity
 *   search/                            → 404 (client toasts; demo limit)
 *   POST|PUT|PATCH|DELETE              → 501 demo_read_only
 *
 * Scope is the Vite base path (e.g. /scry/ on GitHub Pages project sites).
 */
/* eslint-disable no-restricted-globals */

const SCOPE = self.registration.scope // trailing slash, e.g. https://x.github.io/scry/

self.addEventListener('install', (event) => {
  // Activate on first visit without waiting for a tab close.
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)

  // Only same-origin under this SW's scope.
  if (url.origin !== self.location.origin) return
  if (!url.pathname.startsWith(scopePath())) return

  const api = matchAPI(url.pathname)
  if (!api) return // let the network handle static assets / navigation

  event.respondWith(handleAPI(req, api))
})

/** Path of the SW scope without origin, always with trailing slash. */
function scopePath() {
  return new URL(SCOPE).pathname
}

/**
 * If pathname is under {scope}api/v1/{issues|auth}/…, return the relative API
 * tail (e.g. "issues/bootstrap/", "auth/me/"). Otherwise null.
 */
function matchAPI(pathname) {
  const prefix = scopePath() + 'api/v1/'
  if (!pathname.startsWith(prefix)) return null
  return pathname.slice(prefix.length)
}

async function handleAPI(req, apiTail) {
  const method = req.method.toUpperCase()

  // Writes are never available on the hosted snapshot.
  if (method !== 'GET' && method !== 'HEAD') {
    return jsonResponse({ error: 'demo_read_only' }, 501)
  }

  // auth/me/ → anonymous
  if (apiTail === 'auth/me/' || apiTail === 'auth/me') {
    return jsonResponse({ email: null })
  }

  // issues/…
  if (!apiTail.startsWith('issues/')) {
    return jsonResponse({ error: 'not_found' }, 404)
  }
  const tail = apiTail.slice('issues/'.length)

  // bootstrap/
  if (tail === 'bootstrap/' || tail === 'bootstrap') {
    return staticFile('bootstrap.json', 'application/json')
  }

  // {key}/detail/
  let m = /^([A-Za-z0-9_-]+)\/detail\/?$/.exec(tail)
  if (m) {
    return staticFile(`detail/${m[1]}.json`, 'application/json')
  }

  // {key}/attachments/{id}/content/
  m = /^([A-Za-z0-9_-]+)\/attachments\/([A-Za-z0-9_-]+)\/content\/?$/.exec(tail)
  if (m) {
    return staticFile(`attachments/${m[2]}`, 'image/png')
  }

  // Empty collections the client expects or quietly tolerates.
  if (tail === 'delta/' || tail.startsWith('delta?') || tail.startsWith('delta/?')) {
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
  if (tail === 'feed/' || tail.startsWith('feed?') || tail.startsWith('feed/?')) {
    return jsonResponse({
      items: [],
      unread_counts: { all: 0, mentions: 0, watched: 0, assigned: 0 },
    })
  }

  // Server full-text search is not available offline — client toasts on 404.
  if (tail.startsWith('search/') || tail.startsWith('search?')) {
    return jsonResponse({ error: 'not_found' }, 404)
  }

  // Everything else (settings, credential, meta/write, transitions, …): 404.
  // The UI hides surfaces on a clean 404 (see server.go comment).
  return jsonResponse({ error: 'not_found' }, 404)
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'no-store',
    },
  })
}

/** Fetch a file relative to the SW scope (same directory tree as index.html). */
async function staticFile(rel, contentType) {
  const url = new URL(rel, SCOPE).href
  try {
    const res = await fetch(url, { cache: 'force-cache' })
    if (!res.ok) {
      return jsonResponse({ error: 'not_found' }, 404)
    }
    // Re-wrap so we own Content-Type (static hosts may send text/plain).
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
