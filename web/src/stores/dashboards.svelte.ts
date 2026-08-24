/*
 * Agent dashboards store (GDK-782/793 web host).
 *
 * Owns the list, the open dashboard, and the live-update state machine:
 *
 *  - `version` is the server's change counter (moves on every save/update/
 *    delete). App's focus tick calls checkVersion() while a dashboard is
 *    open — the same 500ms poll that already serves `gadak views open`.
 *  - On a version move we refetch the row. updated_at or config changed →
 *    bump `renderGen`, which re-creates the iframe (full document reload:
 *    the authored HTML is the unit of change; no state preservation duty).
 *    Row gone (404) → close. That is the whole `gadak dashboards save` →
 *    open tab re-renders contract (p95 ≤ 1s: ≤0.5s poll + one row fetch).
 *  - Datasource DATA freshness is separate and lives in DashboardView: the
 *    15s delta poll's lastSync re-runs the datasources and re-pushes (≤2s).
 */

import { getDashboard, getDashboards } from '../lib/api'
import type { DashboardRow } from '../lib/types'
import { column } from './column.svelte'

class DashboardsStore {
  constructor() {
    // The row/error belong to the dashboard currently holding the column —
    // released with it, not on a close that landed after something else took
    // over. Registered here (not called from column) so column stays
    // runtime-import-free.
    column.onLeave('dashboard', () => {
      this.row = null
      this.error = null
    })
  }

  /** Rows without configs, as the sidebar lists them. */
  list = $state<Pick<DashboardRow, 'id' | 'name' | 'updated_at'>[]>([])
  loaded = $state(false)
  /** Server change counter last seen. -1 = not fetched yet. */
  version = $state(-1)

  /** Dashboard currently open (null = list screen). A view onto the column
   *  union (GDK-821): showing any other full-column surface is what closes
   *  this — there is no separate latch to forget (GDK-815). */
  get openId(): string | null {
    return column.keyOf('dashboard')
  }
  /** Full row of the open dashboard (config included). null while loading. */
  row = $state<DashboardRow | null>(null)
  /** 'not_found' when the open id vanished between list and open. */
  error = $state<string | null>(null)
  /**
   * Bumped to force a full iframe replacement. The component keys the frame
   * on this; a save that changed the document must reload it, not patch it.
   */
  renderGen = $state(0)

  #inFlightList = false
  #inFlightRow = false
  /** Row-fetch sequence: a response landing after a newer fetch started is stale and discarded. */
  #rowSeq = 0

  /** Load the list once at app start (App onMount). Server-less (hosted demo) leaves it empty. */
  init(): void {
    void this.loadList()
  }

  async loadList(): Promise<void> {
    if (this.#inFlightList) return
    this.#inFlightList = true
    try {
      const res = await getDashboards()
      this.list = res.dashboards
      this.version = res.version
      this.loaded = true
    } catch (e) {
      // Server-less installs answer 404 on the whole dashboards base; that is
      // "no dashboards", not an error surface — leave loaded=false so the
      // sidebar hides the section (CLI-owned feature behind a serve).
      console.warn('[dashboards] list load failed', e)
    } finally {
      this.#inFlightList = false
    }
  }

  /** Open one dashboard (sidebar row, `dash=` URL param, uifocus hash).
   *  Taking the column is what this is: whatever full-column surface was up
   *  is released by the same assignment (GDK-821). */
  open(id: string): void {
    if (this.openId === id) return
    this.error = null
    this.row = null
    column.show({ view: 'dashboard', id })
    void this.loadRow()
    if (!this.loaded) void this.loadList()
  }

  close(): void {
    if (this.openId === null) return
    // onLeave clears the row; the column lands on the list.
    column.close('dashboard')
  }

  async loadRow(force = false): Promise<void> {
    const id = this.openId
    if (!id) return
    if (!force && this.#inFlightRow) return
    const seq = ++this.#rowSeq
    this.#inFlightRow = true
    try {
      const row = await getDashboard(id)
      // Discard when superseded: a close() or a newer fetch started meanwhile
      // (a response that predates a save must not land as the fresh row).
      if (this.openId !== id || seq !== this.#rowSeq) return
      this.row = row
      this.error = null
    } catch (e) {
      if (this.openId !== id || seq !== this.#rowSeq) return
      this.error = e instanceof Error && e.message.includes('404') ? 'not_found' : 'load_error'
    } finally {
      if (seq === this.#rowSeq) this.#inFlightRow = false
    }
  }

  /**
   * Live authoring update (GDK-793), called from the focus tick only while a
   * dashboard is open. One GET list; on a version move, refetch the row and
   * swap the frame when the document actually changed. Returns true when a
   * re-render was triggered (for the e2e timing contract).
   */
  async checkVersion(): Promise<boolean> {
    if (!this.openId || this.#inFlightList) return false
    const before = this.version
    let res
    try {
      res = await getDashboards()
    } catch {
      return false // transient — next tick retries
    }
    this.list = res.dashboards
    this.version = res.version
    if (res.version === before) return false

    // Something changed. Row still there?
    const id = this.openId
    const listed = res.dashboards.find((d) => d.id === id)
    if (!listed) {
      this.close()
      return true
    }
    // The move was another dashboard's save when this row's updated_at did
    // not move; nothing to swap. (this.row can be null — the initial fetch
    // still in flight — and then the landing row is fresh enough to render.)
    if (this.row && listed.updated_at === this.row.updated_at) return false
    // Force past the in-flight guard: a fetch that started before the save
    // would land the old document, and this version move is our only signal.
    await this.loadRow(true)
    this.renderGen++
    return true
  }
}

export const dashboards = new DashboardsStore()
