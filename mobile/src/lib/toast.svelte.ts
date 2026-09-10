/*
 * The app-level toast store (GDK-1504). Before this, AdfBody carried its
 * own `copied` pill — a private $state, a private timer, and a
 * position:fixed style — and the second announcer would have copied the
 * field into every screen that needed one. Worse, the clipboard *refusal*
 * had nothing to announce through at all: the catch was silent, and a link
 * that did not copy read exactly like a link that did.
 *
 * The cadence is the desk's (web/src/stores/write.svelte.ts TOAST_MS), so
 * the two apps time the same event the same way. The state is Svelte-5
 * module state like store.svelte.ts's `app`; ToastHost (mounted once in
 * App.svelte) reads `toastHost.toasts`, and every write goes through
 * showToast/dismissToast — the timers live next to the state they arm.
 *
 * Deliberately NOT in resetSessionState(): a toast is a ≤6s surface, not
 * session data. Resetting would kill the very feedback an unpair would
 * want to show, and auto-dismiss already bounds any cross-workspace leak
 * at one toast's lifetime.
 */

export type ToastKind = 'error' | 'info' | 'success'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const TOAST_MS = { error: 6000, info: 3000, success: 2500 }

let nextId = 0
const timers = new Map<number, ReturnType<typeof setTimeout>>()

export const toastHost = $state({ toasts: [] as Toast[] })

/** Shows one toast. The message arrives already translated (t() at the call site). */
export function showToast(message: string, kind: ToastKind = 'info'): void {
  const id = ++nextId
  toastHost.toasts = [...toastHost.toasts, { id, kind, message }]
  timers.set(id, setTimeout(() => dismissToast(id), TOAST_MS[kind]))
}

/** Dismisses now — a tap, or the timer that armed this id. Idempotent. */
export function dismissToast(id: number): void {
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
  toastHost.toasts = toastHost.toasts.filter((toast) => toast.id !== id)
}
