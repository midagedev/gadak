/*
 * Single owner of "the app is in onboarding".
 *
 * ListView used to derive this locally. SidebarNav needs the same answer to
 * hide write/query chrome that undercuts the wizard (GDK-299), so the
 * expression lives here once — do not copy it.
 *
 * First run vs. "mirror is empty, sync will fill it". Setup is incomplete when
 * there is no stored credential or no project list; once anything has synced
 * (pool > 0) this is false forever, so onboarding cannot come back.
 * A standalone workspace has no Jira site to connect — the wizard would ask
 * for a token the origin cannot use. Kind comes from config.json, never
 * from an empty site URL.
 *
 *  me.authChecked is required: before identity settles we don't know credentials,
 *  and without waiting onboarding flashes one frame at boot.
 *  identity === stored Jira credential, so use me.identified for that check.
 *  onboardingHold: the wizard's optional last step runs after the mirror is
 *  full, so "pool is empty" would hand the pane to the list underneath it.
 */

import { config, isStandaloneWorkspace } from '../lib/config'
import { issues } from './issues.svelte'
import { me } from './me.svelte'

export type OnboardingReason = 'hold' | 'no-credential' | 'no-projects'

/*
 * "The wizard is holding the pane" — true only while the optional last step is
 * on screen. The list decides to show onboarding from an empty pool, which
 * stops being a usable signal the moment the first sync lands: the 15s delta
 * poll drops rows in seconds later and would swap step 4 for the issue list
 * mid-sentence. Leaving the step (finish or skip) clears it, and so does
 * unmounting, so nothing can strand the wizard on a filled mirror.
 */
export const onboardingHold = $state({ active: false })

class OnboardingStore {
  /**
   * Which clause made the gate true, or null when the app is not in
   * onboarding. Hold wins so the debug hook matches the OR order.
   */
  reason = $derived.by((): OnboardingReason | null => {
    if (onboardingHold.active) return 'hold'
    if (
      issues.pool.size === 0 &&
      me.authChecked &&
      !isStandaloneWorkspace() &&
      (!me.identified || config().projects.length === 0)
    ) {
      return me.identified ? 'no-projects' : 'no-credential'
    }
    return null
  })

  needsOnboarding = $derived(this.reason !== null)
}

export const onboarding = new OnboardingStore()
