/*
 * ADF (Atlassian Document Format) → HTML renderer ([detail]).
 *
 * Input is the raw ADF of a Jira description / comment (AdfNode tree); output is an HTML string.
 * Components insert it via {@html}; typography/spacing live in AdfContent.svelte's
 *  global (:global(.adf)) rules. Only specials (mention/status/panel/attachment, …) get
 *  Tailwind utility classes baked into the string for color.
 *
 * ── XSS safety rules ──
 *  1. Every text node goes through esc() (<,>,&,",' replaced).
 *  2. Only tags this file emits exist (user input never becomes a tag).
 *  3. hrefs pass safeHref() — http(s) only; otherwise no link.
 *  4. Style values (color, …) are inlined only after hex-regex validation.
 *  5. Unsupported nodes fall back to escaped text / recursive children — never raw HTML.
 */

import { t } from './i18n'
import { config, jiraBrowseUrl } from './config'
import type { AdfNode, DetailAttachment } from './types'

/** Render options. Pass issue key for media fallback links. */
export interface AdfRenderOptions {
  /** Used when media (attachment) fallbacks link out to the original Jira issue. */
  issueKey?: string
  /** Map of Jira media UUID/filename → local attachment proxy URL. */
  attachments?: DetailAttachment[]
}

/** Escape HTML specials. All text/attribute values go through this. */
function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** Allow http(s) URLs only. Returns escaped href on pass, else null. */
function safeHref(url: unknown): string | null {
  if (typeof url !== 'string') return null
  const trimmed = url.trim()
  if (!/^https?:\/\//i.test(trimmed)) return null
  return esc(trimmed)
}

/**
 * Allow only same-origin attachment paths issued by the backend as media src.
 *
 * apiBase is runtime config but the check is still a whitelist: must start with apiBase,
 * and the remainder must look like `<issueKey>/attachments/<id>/content/`. Dots and
 * extra slashes are rejected, so `..` traversal and `//host` protocol-relative URLs fail.
 */
function safeMediaUrl(url: unknown): string | null {
  if (typeof url !== 'string') return null
  const trimmed = url.trim()
  const base = config().apiBase
  if (!base || !trimmed.startsWith(base)) return null
  const tail = trimmed.slice(base.length)
  return /^[A-Za-z0-9_-]+\/attachments\/[A-Za-z0-9_-]+\/content\/$/.test(tail) ? esc(trimmed) : null
}

/** Allow #rgb / #rrggbb / #rrggbbaa hex colors only (for textColor marks). */
function safeColor(c: unknown): string | null {
  if (typeof c !== 'string') return null
  return /^#[0-9a-fA-F]{3,8}$/.test(c.trim()) ? c.trim() : null
}

/** Safely pull a string value from attrs. */
function attrStr(node: AdfNode, key: string): string | undefined {
  const v = node.attrs?.[key]
  return typeof v === 'string' ? v : undefined
}

/** Safely pull a number value from attrs. */
function attrNum(node: AdfNode, key: string): number | undefined {
  const v = node.attrs?.[key]
  return typeof v === 'number' ? v : undefined
}

/* ── Apply marks (inline formatting) ── */

function applyMark(html: string, mark: { type: string; attrs?: Record<string, unknown> }): string {
  switch (mark.type) {
    case 'strong':
      return `<strong>${html}</strong>`
    case 'em':
      return `<em>${html}</em>`
    case 'code':
      return `<code>${html}</code>`
    case 'strike':
      return `<s>${html}</s>`
    case 'underline':
      return `<u>${html}</u>`
    case 'subsup': {
      const t = mark.attrs?.type
      return t === 'sub' ? `<sub>${html}</sub>` : `<sup>${html}</sup>`
    }
    case 'textColor': {
      const color = safeColor(mark.attrs?.color)
      return color ? `<span style="color:${color}">${html}</span>` : html
    }
    case 'link': {
      const href = safeHref(mark.attrs?.href)
      return href
        ? `<a href="${href}" target="_blank" rel="noopener noreferrer">${html}</a>`
        : html
    }
    default:
      // Unsupported mark: keep text with no formatting
      return html
  }
}

/** text node: escape, then wrap marks inside-out. */
function renderText(node: AdfNode): string {
  let html = esc(node.text ?? '')
  for (const mark of node.marks ?? []) html = applyMark(html, mark)
  return html
}

/** Concatenate rendered children. */
function renderChildren(node: AdfNode, opts: AdfRenderOptions): string {
  return (node.content ?? []).map((c) => renderNode(c, opts)).join('')
}

function findAttachment(node: AdfNode, opts: AdfRenderOptions): DetailAttachment | null {
  const mediaId = attrStr(node, 'id')
  const alt = attrStr(node, 'alt')?.trim().toLocaleLowerCase()
  if (mediaId) {
    const exact = opts.attachments?.find((attachment) => attachment.media_id === mediaId)
    if (exact) return exact
  }
  if (alt) {
    return (
      opts.attachments?.find(
        (attachment) => attachment.filename.trim().toLocaleLowerCase() === alt,
      ) ?? null
    )
  }
  return null
}

function renderAttachment(attachment: DetailAttachment, compact = false): string {
  const src = safeMediaUrl(attachment.content_url)
  if (!src) return ''
  const name = esc(attachment.filename || t('common.attachmentFile'))
  const id = esc(attachment.id)
  if (attachment.is_image && !compact) {
    return `<button type="button" class="adf-media-image" data-attachment-id="${id}" aria-label="${t('detail.enlarge', { name })}"><img src="${src}" alt="${name}" loading="lazy" decoding="async"></button>`
  }
  if (attachment.is_video && !compact) {
    return `<figure class="adf-media-video"><video src="${src}" controls preload="metadata" playsinline aria-label="${name}"></video><figcaption>${name}</figcaption></figure>`
  }
  return `<a class="adf-media" href="${src}" target="_blank" rel="noopener noreferrer">📎 ${name}</a>`
}

/** Panel type → Tailwind color combo (border / bg tint / text). */
const PANEL_STYLES: Record<string, string> = {
  info: 'border-status-new/40 bg-status-new/10',
  // The other four key off status inks, which sit high on the ladder in both
  // themes. --color-accent is a fill, not an ink: in dark it lands below the
  // panel ground and the note outline all but disappears (measured 1.30 vs
  // the family's 1.90–2.19). accent-text is the ink counterpart, same hue.
  note: 'border-accent-text/40 bg-accent-text/10',
  success: 'border-status-done/40 bg-status-done/10',
  warning: 'border-status-stale/40 bg-status-stale/10',
  error: 'border-status-reopen/40 bg-status-reopen/10',
}

/** Jira status-node color name → chip background. Tokens live in app.css. */
const STATUS_COLORS: Record<string, string> = {
  neutral: 'var(--color-lozenge-neutral)',
  grey: 'var(--color-lozenge-neutral)',
  purple: 'var(--color-lozenge-purple)',
  blue: 'var(--color-lozenge-blue)',
  red: 'var(--color-lozenge-red)',
  yellow: 'var(--color-lozenge-yellow)',
  green: 'var(--color-lozenge-green)',
}

/** Single node → HTML. */
function renderNode(node: AdfNode, opts: AdfRenderOptions): string {
  switch (node.type) {
    case 'doc':
      return renderChildren(node, opts)

    case 'text':
      return renderText(node)

    case 'paragraph':
      return `<p>${renderChildren(node, opts)}</p>`

    case 'heading': {
      const level = Math.min(6, Math.max(1, attrNum(node, 'level') ?? 1))
      return `<h${level}>${renderChildren(node, opts)}</h${level}>`
    }

    case 'hardBreak':
      return '<br>'

    case 'rule':
      return '<hr>'

    case 'bulletList':
      return `<ul>${renderChildren(node, opts)}</ul>`

    case 'orderedList':
      return `<ol>${renderChildren(node, opts)}</ol>`

    case 'listItem':
      return `<li>${renderChildren(node, opts)}</li>`

    case 'blockquote':
      return `<blockquote>${renderChildren(node, opts)}</blockquote>`

    case 'codeBlock': {
      const lang = attrStr(node, 'language')
      const badge = lang
        ? `<span class="adf-code-lang">${esc(lang)}</span>`
        : ''
      // codeBlock children are (unmarked) text nodes — escape only.
      const code = (node.content ?? []).map((c) => esc(c.text ?? '')).join('')
      return `<div class="adf-code">${badge}<pre><code>${code}</code></pre></div>`
    }

    case 'panel': {
      const type = attrStr(node, 'panelType') ?? 'info'
      const cls = PANEL_STYLES[type] ?? PANEL_STYLES.info
      return `<div class="adf-panel ${cls}">${renderChildren(node, opts)}</div>`
    }

    case 'table':
      return `<div class="adf-table-wrap"><table>${renderChildren(node, opts)}</table></div>`

    case 'tableRow':
      return `<tr>${renderChildren(node, opts)}</tr>`

    case 'tableHeader':
      return `<th>${renderChildren(node, opts)}</th>`

    case 'tableCell':
      return `<td>${renderChildren(node, opts)}</td>`

    case 'mention': {
      const raw = attrStr(node, 'text') ?? ''
      const name = raw.replace(/^@/, '') || t('adf.unknownMention')
      return `<span class="adf-mention">@${esc(name)}</span>`
    }

    case 'emoji': {
      // Priority: already-unicode text → id (codepoint) convert → shortName fallback
      const text = attrStr(node, 'text')
      if (text) return esc(text)
      const id = attrStr(node, 'id')
      const cp = id && /^[0-9a-fA-F-]+$/.test(id) ? codepointsToEmoji(id) : null
      if (cp) return cp
      const short = attrStr(node, 'shortName') ?? ''
      return esc(short)
    }

    case 'status': {
      const text = attrStr(node, 'text') ?? ''
      const color = STATUS_COLORS[attrStr(node, 'color') ?? 'neutral'] ?? STATUS_COLORS.neutral
      return `<span class="adf-status" style="background:${color}">${esc(text)}</span>`
    }

    case 'date': {
      const ts = attrStr(node, 'timestamp')
      const label = ts ? formatEpoch(ts) : ''
      return `<span class="adf-date">📅 ${esc(label)}</span>`
    }

    case 'inlineCard': {
      const href = safeHref(node.attrs?.url)
      if (!href) return ''
      return `<a class="adf-inline-card" href="${href}" target="_blank" rel="noopener noreferrer">🔗 ${href}</a>`
    }

    case 'taskList':
      return `<div class="adf-task-list">${renderChildren(node, opts)}</div>`

    case 'taskItem': {
      const done = attrStr(node, 'state') === 'DONE'
      const box = done ? '☑' : '☐'
      const doneCls = done ? ' adf-task-done' : ''
      return `<div class="adf-task-item${doneCls}"><span class="adf-task-box">${box}</span><span>${renderChildren(node, opts)}</span></div>`
    }

    case 'mediaSingle':
      return `<div class="adf-media-block">${renderChildren(node, opts)}</div>`

    case 'mediaGroup':
      return `<div class="adf-media-group">${renderChildren(node, opts)}</div>`

    case 'media': {
      const attachment = findAttachment(node, opts)
      if (attachment) return renderAttachment(attachment)
      const name = attrStr(node, 'alt') ?? t('common.attachmentFile')
      const link = opts.issueKey ? jiraBrowseUrl(opts.issueKey) : null
      const label = t('detail.attachmentLabel', { name: esc(name) })
      return link
        ? `<a class="adf-media" href="${esc(link)}" target="_blank" rel="noopener noreferrer">${label}</a>`
        : `<span class="adf-media">${label}</span>`
    }

    case 'mediaInline': {
      const attachment = findAttachment(node, opts)
      if (attachment) return renderAttachment(attachment, true)
      const name = attrStr(node, 'alt')
      const label = name
        ? t('detail.attachmentLabel', { name: esc(name) })
        : esc(t('common.attachment'))
      return `<span class="adf-media">${label}</span>`
    }

    default:
      // Unsupported node: escape text if present, else recursive children fallback
      if (typeof node.text === 'string') return esc(node.text)
      return renderChildren(node, opts)
  }
}

/** Codepoint string ("1f604" or "1f1f0-1f1f7") → emoji. null on failure. */
function codepointsToEmoji(id: string): string | null {
  try {
    const cps = id.split('-').map((h) => parseInt(h, 16))
    if (cps.some((n) => Number.isNaN(n) || n <= 0 || n > 0x10ffff)) return null
    return String.fromCodePoint(...cps)
  } catch {
    return null
  }
}

/** epoch(ms) string → YYYY-MM-DD. */
function formatEpoch(ts: string): string {
  const n = Number(ts)
  if (Number.isNaN(n)) return ts
  const d = new Date(n)
  if (Number.isNaN(d.getTime())) return ts
  return d.toISOString().slice(0, 10)
}

/**
 * ADF document (or inline node) → HTML string.
 * Swallows parse exceptions and returns '' (caller can fall back to plain text).
 */
export function renderAdf(doc: AdfNode | null | undefined, opts: AdfRenderOptions = {}): string {
  if (!doc) return ''
  try {
    return renderNode(doc, opts)
  } catch (e) {
    console.warn('[adf] 렌더 실패', e)
    return ''
  }
}
