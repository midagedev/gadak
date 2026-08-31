/*
 * Pick a card up, put it on a column, and the drop is `write.transition` —
 * nothing else (GDK-1176). This is the reactive half: pointer plumbing plus
 * the shared state the board reads back as data attributes; the meaning of a
 * drop lives in ./board-drag.
 *
 * Native pointer events on purpose (no dnd library): the board has exactly
 * one gesture — no reorder, no insertion index, column order is owned by the
 * sort. The ghost is a fixed clone on <body>; the original card never moves,
 * so BoardView's FLIP (which animates *other* people's moves) never finds a
 * dnd transform on an element it measures.
 *
 * Legality is a preview, never a gate: the local transition map — or a lazy
 * GET fired when the gesture arms, never awaited on it — dims columns no
 * transition reaches, and the write path's 400 → rollback + toast stays the
 * only enforcement.
 */

import * as api from './api'
import type { IssueLite, Transition } from './types'
import { transitionsInto, dropVerdict } from './board-drag'
import { effectiveCategory } from './view-config'
import { filters } from '../stores/filters.svelte'
import { write } from '../stores/write.svelte'

/** Pointer travel that turns a press into a drag; anything less is a click. */
const ARM_PX = 5

interface DropChoice {
  key: string
  x: number
  y: number
  candidates: Transition[]
}

class BoardDrag {
  /** The key in flight, or null. Cards render `data-dragging` from this. */
  draggingKey = $state<string | null>(null)
  /** The column under the pointer. Columns render `data-drop` from this. */
  overColumn = $state<string | null>(null)
  /** An ambiguous drop waiting on a name (2+ transitions reach the column). */
  choice = $state<DropChoice | null>(null)

  /** null while unknown (map miss, GET in flight or failed) — preview only. */
  #candidates = $state<Transition[] | null>(null)
  #pending: Promise<Transition[] | null> | null = null
  #seq = 0

  #issue: IssueLite | null = null
  #cardEl: HTMLElement | null = null
  #pointerId = 0
  #startX = 0
  #startY = 0
  #armed = false
  #ghost: HTMLElement | null = null
  #suppressClick = false

  /** Card pointerdown. Arms only on the status axis, never from the shell glyph. */
  start(e: PointerEvent, issue: IssueLite): void {
    if (e.button !== 0 || this.draggingKey || this.choice) return
    if (filters.display.group_by !== 'status_category') return
    if ((e.target as Element | null)?.closest('[data-testid="board-card-shell-enter"]')) return
    const card = e.currentTarget as HTMLElement | null
    if (!card) return
    this.#issue = issue
    this.#cardEl = card
    this.#pointerId = e.pointerId
    this.#startX = e.clientX
    this.#startY = e.clientY
    card.addEventListener('pointermove', this.#onMove)
    card.addEventListener('pointerup', this.#onUp)
    card.addEventListener('pointercancel', this.#onCancel)
  }

  /** Consumed by the card's onclick so a finished drag is not also a select. */
  consumeClick(): boolean {
    const s = this.#suppressClick
    this.#suppressClick = false
    return s
  }

  /** Column state for the board while a drag is up; null otherwise. */
  dropStateFor(colKey: string): 'legal' | 'illegal' | 'over' | null {
    if (!this.draggingKey || !this.#issue) return null
    const verdict = dropVerdict(this.#issue, colKey, this.#candidates)
    if (verdict === 'legal' && this.overColumn === colKey) return 'over'
    return verdict
  }

  choose(tr: Transition): void {
    const c = this.choice
    this.choice = null
    if (c) void write.transition(c.key, tr)
  }

  dismissChoice(): void {
    this.choice = null
  }

  #arm(): void {
    const card = this.#cardEl
    const issue = this.#issue
    if (!card || !issue) return
    this.#armed = true
    this.#suppressClick = true
    this.draggingKey = issue.issue_key
    try {
      card.setPointerCapture(this.#pointerId)
    } catch {
      /* capture can be refused (pointer already gone) — the drag still works */
    }
    window.addEventListener('keydown', this.#onKey, true)
    document.body.style.userSelect = 'none'

    // Best-effort legality: the local map now, a GET refining it when it lands.
    this.#candidates = write.transitionsFor(issue)
    if (!this.#candidates) {
      const seq = ++this.#seq
      this.#pending = api.getTransitions(issue.issue_key).then(
        (res) => {
          if (seq === this.#seq) this.#candidates = res.transitions
          return res.transitions
        },
        () => null, // stay unknown; the drop no-ops or the 400 says why
      )
    }

    // The ghost: a fixed clone stripped of identity so nothing — FLIP queries,
    // tests, screen readers — mistakes it for the card, which stays in place.
    const r = card.getBoundingClientRect()
    const g = card.cloneNode(true) as HTMLElement
    g.removeAttribute('data-board-key')
    g.setAttribute('aria-hidden', 'true')
    for (const el of [g, ...g.querySelectorAll('[data-testid]')]) el.removeAttribute('data-testid')
    g.classList.add('board-drag-ghost', 'shadow-overlay')
    Object.assign(g.style, {
      position: 'fixed',
      left: `${r.left}px`,
      top: `${r.top}px`,
      width: `${r.width}px`,
      margin: '0',
      zIndex: '80',
      pointerEvents: 'none',
    })
    document.body.appendChild(g)
    this.#ghost = g
  }

  #columnAt(e: PointerEvent): string | null {
    const el = document.elementFromPoint(e.clientX, e.clientY)
    return el?.closest<HTMLElement>('[data-board-column]')?.dataset.boardColumn ?? null
  }

  #onMove = (e: PointerEvent): void => {
    if (e.pointerId !== this.#pointerId) return
    if (!this.#armed) {
      if (Math.hypot(e.clientX - this.#startX, e.clientY - this.#startY) < ARM_PX) return
      this.#arm()
    }
    if (this.#ghost) {
      this.#ghost.style.transform = `translate(${e.clientX - this.#startX}px, ${e.clientY - this.#startY}px)`
    }
    this.overColumn = this.#columnAt(e)
  }

  #onUp = (e: PointerEvent): void => {
    if (e.pointerId !== this.#pointerId) return
    const armed = this.#armed
    const issue = this.#issue
    const col = armed ? this.#columnAt(e) : null
    const known = this.#candidates
    const pending = this.#pending
    const { clientX: x, clientY: y } = e
    this.#end()
    if (!armed || !issue || !col) return
    void this.#drop(issue, col, known, pending, x, y)
  }

  #onCancel = (e: PointerEvent): void => {
    if (e.pointerId !== this.#pointerId) return
    this.#end()
  }

  /** Escape drops the drag and keeps the key from any surface underneath. */
  #onKey = (e: KeyboardEvent): void => {
    if (e.key !== 'Escape' || !this.#armed) return
    e.preventDefault()
    e.stopPropagation()
    this.#end()
  }

  async #drop(
    issue: IssueLite,
    col: string,
    known: Transition[] | null,
    pending: Promise<Transition[] | null> | null,
    x: number,
    y: number,
  ): Promise<void> {
    if (effectiveCategory(issue) === col) return // same column — no-op by design
    const list = known ?? (pending ? await pending : null)
    const into = list ? transitionsInto(list, col) : []
    if (into.length === 0) return // unknown or unreachable: the dimming said as much
    if (into.length === 1) {
      void write.transition(issue.issue_key, into[0])
      return
    }
    // Ambiguous: hand the drop point a menu (BoardDropChoice), clamped on-screen.
    this.choice = {
      key: issue.issue_key,
      x: Math.max(8, Math.min(x, window.innerWidth - 216)),
      y: Math.max(8, Math.min(y, window.innerHeight - 40 * into.length - 48)),
      candidates: into,
    }
  }

  #end(): void {
    const card = this.#cardEl
    if (card) {
      card.removeEventListener('pointermove', this.#onMove)
      card.removeEventListener('pointerup', this.#onUp)
      card.removeEventListener('pointercancel', this.#onCancel)
      try {
        card.releasePointerCapture(this.#pointerId)
      } catch {
        /* already released */
      }
    }
    window.removeEventListener('keydown', this.#onKey, true)
    this.#ghost?.remove()
    this.#ghost = null
    document.body.style.userSelect = ''
    this.draggingKey = null
    this.overColumn = null
    this.#armed = false
    this.#issue = null
    this.#cardEl = null
    this.#candidates = null
    this.#pending = null
  }
}

export const boardDrag = new BoardDrag()
