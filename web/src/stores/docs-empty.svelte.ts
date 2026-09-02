/*
 * Empty-docs owner (sidebar CTA + Documents body).
 *
 * Seven causes, one decision table (`lib/docs-empty.ts`). This store is the
 * single place that gathers the table's inputs — including the Confluence
 * sync-run fetch and the page index's loadFailed — so the sidebar row and the
 * body EmptyState cannot disagree.
 *
 * `bind()` starts the fetch effect. Do not call it from the constructor:
 * App import runs before loadConfig (sidebar-sections.svelte.ts documents
 * that trap). Callers invoke it at component init, after config.json is on.
 */

import { getSyncRuns } from '../lib/api'
import { config, hasServerVerb } from '../lib/config'
import { isLocalOrigin } from '../lib/workspace'
import {
  docsEmptyCopy,
  docsEmptyState,
  type DocsEmptyCopy,
  type DocsEmptyRun,
  type DocsEmptyState,
} from '../lib/docs-empty'
import { t } from '../lib/i18n'
import { busyLabel, fetchingDocuments } from '../lib/mirror-status'
import { pages } from './pages.svelte'

class DocsEmptyStore {
  confluenceRuns = $state<DocsEmptyRun[] | null>(null)
  #inFlight = false
  #bound = false

  /**
   * Register the fetch effect once for the process. Idempotent — every
   * surface that reads this store calls it so none of them depends on
   * another having mounted first. `$effect.root` so the effect is not tied
   * to the first caller's lifetime (DocsView unmounts; the sidebar does not).
   */
  bind(): void {
    if (this.#bound) return
    this.#bound = true
    $effect.root(() => {
      $effect(() => {
        this.#syncRuns()
      })
    })
  }

  /**
   * Same guard SidebarNav used: only ask when the answer changes what the
   * section says — configured, holding no pages, nothing in flight. Re-runs
   * after a pull because fetchingDocuments reads mirrorSyncing.
   */
  #syncRuns(): void {
    if (!config().confluenceEnabled || pages.bySpace.length > 0 || fetchingDocuments()) return
    if (this.#inFlight) return
    this.#inFlight = true
    void getSyncRuns('confluence')
      .then((doc) => {
        this.confluenceRuns = doc.runs
      })
      .finally(() => {
        this.#inFlight = false
      })
  }

  get state(): DocsEmptyState {
    return docsEmptyState({
      hasDocsServer: hasServerVerb('docs'),
      confluenceEnabled: config().confluenceEnabled,
      fetchingDocuments: fetchingDocuments(),
      indexLoadFailed: pages.loadFailed,
      confluenceRuns: this.confluenceRuns,
      localOrigin: isLocalOrigin(config()),
    })
  }

  get copy(): DocsEmptyCopy {
    return docsEmptyCopy(this.state)
  }

  /** Sidebar row title + hint. Body headings keep their own title. */
  get text(): { title: string; hint: string } {
    const copy = this.copy
    return {
      title: copy.titlePrefersBusy ? (busyLabel() ?? t(copy.titleKey)) : t(copy.titleKey),
      hint: copy.hintKey ? t(copy.hintKey) : '',
    }
  }
}

/** App-wide singleton. */
export const docsEmpty = new DocsEmptyStore()
