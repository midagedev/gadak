/*
 * Issue Navigator — web push / notification preferences store ([personal], contract §2)
 *
 * Web Push subscription + server notification preferences. Loaded when identity
 * appears; cleared when it disappears. Gated by features.push, workspace, and
 * hosted-demo (the snapshot is read-only; its in-page fetch adapter answers the API).
 */

import { t } from '../lib/i18n'
import * as api from '../lib/api'
import { basePath, feature, isHostedDemo, workspaceName } from '../lib/config'
import type { NotificationConfig, NotificationPreferences } from '../lib/types'
import { me } from './me.svelte'

function urlBase64ToUint8Array(value: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (value.length % 4)) % 4)
  const raw = atob((value + padding).replace(/-/g, '+').replace(/_/g, '/'))
  const bytes = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i += 1) bytes[i] = raw.charCodeAt(i)
  return bytes
}

export type PushState =
  | 'unsupported'
  | 'unavailable'
  | 'default'
  | 'denied'
  | 'subscribed'
  | 'unsubscribed'
  | 'loading'

class PushStore {
  /* ── Web Push ── */
  notificationConfig = $state<NotificationConfig | null>(null)
  state = $state<PushState>('default')
  error = $state<string | null>(null)

  get supported(): boolean {
    return (
      typeof window !== 'undefined' &&
      'serviceWorker' in navigator &&
      'PushManager' in window &&
      'Notification' in window
    )
  }

  /** Drop push state when identity disappears. */
  clear(): void {
    this.notificationConfig = null
    this.state = 'default'
  }

  /** Skip config fetch and SW registration when push is off. */
  async load(): Promise<void> {
    // Hosted demo: the static snapshot has no push backend, and sw.js has no
    // business on that origin — the in-page fetch adapter serves the API.
    if (isHostedDemo()) return
    // Workspace mounts: sw.js at the root scope would push for the primary
    // mirror, not this one. Push stays a primary-page feature.
    if (workspaceName() !== '') return
    if (!feature('push') || !me.identified) return
    try {
      this.notificationConfig = await api.getNotificationConfig()
      if (!this.supported) {
        this.state = 'unsupported'
        return
      }
      if (!this.notificationConfig.enabled) {
        this.state = 'unavailable'
        return
      }
      if (Notification.permission === 'denied') {
        this.state = 'denied'
        return
      }
      const registration = await navigator.serviceWorker.register(`${basePath()}sw.js`, {
        scope: basePath(),
      })
      const subscription = await registration.pushManager.getSubscription()
      this.state = subscription ? 'subscribed' : 'unsubscribed'
    } catch (e) {
      console.warn('[me] 웹 알림 설정 로드 실패', e)
      this.state = 'unavailable'
    }
  }

  async enable(): Promise<boolean> {
    if (!this.supported || !this.notificationConfig?.enabled) return false
    this.state = 'loading'
    this.error = null
    try {
      const permission = await Notification.requestPermission()
      if (permission !== 'granted') {
        this.state = permission === 'denied' ? 'denied' : 'unsubscribed'
        return false
      }
      const registration = await navigator.serviceWorker.register(`${basePath()}sw.js`, {
        scope: basePath(),
      })
      const existing = await registration.pushManager.getSubscription()
      const subscription =
        existing ??
        (await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(
            this.notificationConfig.vapid_public_key,
          ),
        }))
      const serialized = subscription.toJSON()
      const endpoint = serialized.endpoint ?? subscription.endpoint
      const p256dh = serialized.keys?.p256dh
      const auth = serialized.keys?.auth
      if (!p256dh || !auth) throw new Error(t('me.noCryptoKey'))
      await api.savePushSubscription({ endpoint, keys: { p256dh, auth } })
      this.state = 'subscribed'
      return true
    } catch (e) {
      console.warn('[me] 웹 알림 활성화 실패', e)
      this.state = 'unsubscribed'
      this.error = t('me.enableNotifFailed')
      return false
    }
  }

  async disable(): Promise<void> {
    if (!this.supported) return
    this.state = 'loading'
    this.error = null
    try {
      const registration = await navigator.serviceWorker.getRegistration(basePath())
      const subscription = await registration?.pushManager.getSubscription()
      if (subscription) {
        await api.deletePushSubscription(subscription.endpoint)
        await subscription.unsubscribe()
      }
      this.state = 'unsubscribed'
    } catch (e) {
      console.warn('[me] 웹 알림 해제 실패', e)
      this.state = 'subscribed'
      this.error = t('me.disableNotifFailed')
    }
  }

  async updatePreferences(patch: Partial<NotificationPreferences>): Promise<void> {
    if (!this.notificationConfig) return
    const previous = this.notificationConfig
    this.notificationConfig = {
      ...previous,
      preferences: { ...previous.preferences, ...patch },
    }
    try {
      this.notificationConfig = await api.updateNotificationPreferences(patch)
    } catch (e) {
      this.notificationConfig = previous
      console.warn('[me] 알림 선호 저장 실패', e)
    }
  }
}

/** App-wide singleton. */
export const push = new PushStore()
