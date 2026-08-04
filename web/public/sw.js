// Service worker for Web Push notifications.
// The scope is whatever path the app is served from, so notification clicks
// return to that same deployment without hardcoding a base path.
const DEFAULT_URL = self.registration.scope

self.addEventListener('push', (event) => {
  let payload = {}
  try {
    payload = event.data ? event.data.json() : {}
  } catch {
    payload = { body: event.data ? event.data.text() : '' }
  }

  const unreadCount = Number(payload.unread_count || 0)
  const notification = self.registration.showNotification(payload.title || 'scry', {
    body: payload.body || 'New activity.',
    tag: payload.tag || 'scry-feed',
    renotify: true,
    data: { url: payload.url || DEFAULT_URL },
  })
  const badge =
    unreadCount > 0 && typeof self.registration.setAppBadge === 'function'
      ? self.registration.setAppBadge(unreadCount)
      : Promise.resolve()
  event.waitUntil(Promise.all([notification, badge]))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url || DEFAULT_URL, self.location.origin).href
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then(async (clients) => {
      for (const client of clients) {
        if (new URL(client.url).origin !== self.location.origin) continue
        await client.navigate(target)
        return client.focus()
      }
      return self.clients.openWindow(target)
    }),
  )
})
