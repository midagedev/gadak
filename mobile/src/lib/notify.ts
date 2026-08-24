/*
 * Local notification transport — the banner half of the quiet queue
 * (GDK-802, ux-report Q5). APNs does not exist in this product: a banner
 * is a local notification raised from a foreground feed poll, and it goes
 * out only when the app is NOT visible — while the user is looking at the
 * app, the queue screen refreshes in place and raising a banner would be
 * the noise the quiet queue exists to avoid.
 *
 * Grouping uses the plugin's `group` option (the iOS threadIdentifier
 * analog — same-issue notifications stack, e.g. three comments on GDK-1),
 * and the tap payload rides in `extra.issue_key`. The badge stays off on
 * purpose: an iOS badge counts unread notifications, not "new rows in my
 * queue", and the latter use is an HIG violation — nothing here ever sets
 * one. No sound/silent overrides either: iOS applies the mute switch and
 * Focus to local notifications by default; the plugin exposes no
 * interruption level to force Passive (device-measurement TODO).
 *
 * The tab callback is routing-agnostic: onopen(issueKey | null) — null
 * means "no target", and the caller (A-nav) lands the queue, per ux-report
 * Q5 ("issue_key 없는 피드 행 → 큐"). The permission prompt timing is a
 * product decision wired by A-nav: ensurePermission() is asked for after
 * pairing succeeds AND the first promotion-eligible event arrives — never
 * on first run. This module only provides the door.
 */
import {
  isPermissionGranted,
  onAction,
  requestPermission,
  sendNotification,
} from '@tauri-apps/plugin-notification'

/** One banner to raise while the app is off-screen. Copy is the caller's. */
export interface Banner {
  title: string
  body: string
  /**
   * The issue the banner is about — the grouping key and the tap target.
   * null when no issue is attached (tap lands the queue).
   */
  issueKey: string | null
}

/**
 * Whether the app is on screen. Environments with no document at all
 * (node/vitest) count as visible: banners are suppressed, which keeps
 * test and SSR-adjacent paths quiet.
 */
export function appIsVisible(): boolean {
  return typeof document === 'undefined' ? true : document.visibilityState === 'visible'
}

/**
 * The visibility policy in one place: returns whether a banner went out.
 * Called by feed.ts per promoted item; screen-level suppression ("already
 * viewing that issue → no banner", ux-report Q5) belongs to the caller
 * that knows the UI state (A-nav), not to the transport.
 */
export function showBanner(b: Banner): boolean {
  if (appIsVisible()) return false
  sendNotification({
    title: b.title,
    body: b.body,
    group: b.issueKey ?? undefined,
    extra: { issue_key: b.issueKey },
  })
  return true
}

/**
 * True when banners may be shown. Idempotent from the caller's side: an
 * already-granted answer never re-prompts, and after one OS prompt the
 * system itself stops asking (denials return immediately).
 */
export async function ensurePermission(): Promise<boolean> {
  if (await isPermissionGranted()) return true
  return (await requestPermission()) === 'granted'
}

let tapHandler: ((issueKey: string | null) => void) | null = null
let tapBound = false

/**
 * Routes notification taps: extra.issue_key → onopen(key), anything else
 * (missing, empty, wrong type) → onopen(null) → the queue. Re-binding
 * replaces the handler; the plugin listener is registered once.
 */
export function bindNotificationTap(onopen: (issueKey: string | null) => void): void {
  tapHandler = onopen
  if (tapBound) return
  tapBound = true
  void Promise.resolve(
    onAction((notification) => {
      const extra = (notification as { extra?: unknown } | null)?.extra
      const key = (extra as { issue_key?: unknown } | null)?.issue_key
      tapHandler?.(typeof key === 'string' && key !== '' ? key : null)
    }),
  ).catch(() => {
    /* plugin unreachable outside the packaged app — nothing to route */
  })
}
