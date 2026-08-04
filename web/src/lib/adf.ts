/*
 * ADF (Atlassian Document Format) → HTML 렌더러 ([detail]).
 *
 * 입력은 Jira description / comment 의 ADF 원본(AdfNode 트리), 출력은 HTML string.
 * 컴포넌트는 이 문자열을 {@html} 로 삽입하고, 타이포/여백 스타일은 AdfContent.svelte 의
 *  전역(:global(.adf)) 규칙이 담당한다. 특수 요소(멘션/상태/패널/첨부 등)만 Tailwind
 *  유틸리티 클래스를 문자열에 직접 박아 색을 맞춘다.
 *
 * ── XSS 안전 규율 ──
 *  1. 모든 텍스트는 esc() 로 escape 한다(<,>,&,",' 치환).
 *  2. 태그는 이 파일이 생성하는 화이트리스트만 존재한다(사용자 입력이 태그가 되지 않음).
 *  3. href 는 safeHref() 로 http(s) 만 허용, 그 외엔 링크로 만들지 않는다.
 *  4. 색상 등 style 값은 정규식 검증(hex) 을 통과한 것만 인라인한다.
 *  5. 미지원 노드는 자식으로 텍스트 폴백(재귀) — 절대 원본 HTML 을 흘리지 않는다.
 */

import { config, jiraBrowseUrl } from './config'
import type { AdfNode, DetailAttachment } from './types'

/** 렌더 옵션. media 폴백 링크를 위해 이슈 키를 넘길 수 있다. */
export interface AdfRenderOptions {
  /** media(첨부) 폴백에서 Jira 원본 이슈로 링크 걸 때 사용. */
  issueKey?: string
  /** Jira media UUID/파일명과 실제 redacted-tool 첨부 URL 매핑. */
  attachments?: DetailAttachment[]
}

/** HTML 특수문자 escape. 텍스트/속성 값 모두 이걸 통과시킨다. */
function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** http(s) URL 만 허용. 통과하면 escape 된 href, 아니면 null. */
function safeHref(url: unknown): string | null {
  if (typeof url !== 'string') return null
  const trimmed = url.trim()
  if (!/^https?:\/\//i.test(trimmed)) return null
  return esc(trimmed)
}

/**
 * 백엔드가 발급한 same-origin 첨부 경로만 media src로 허용한다.
 *
 * apiBase 는 런타임 설정이지만 검사는 그대로 화이트리스트다: 반드시 apiBase 로 시작하고,
 * 남은 경로가 `<issueKey>/attachments/<id>/content/` 모양이어야 한다. 점·슬래시가 허용되지
 * 않으므로 `..` 트래버설과 `//host` 형태의 protocol-relative URL 은 모두 걸러진다.
 */
function safeMediaUrl(url: unknown): string | null {
  if (typeof url !== 'string') return null
  const trimmed = url.trim()
  const base = config().apiBase
  if (!base || !trimmed.startsWith(base)) return null
  const tail = trimmed.slice(base.length)
  return /^[A-Za-z0-9_-]+\/attachments\/[A-Za-z0-9_-]+\/content\/$/.test(tail) ? esc(trimmed) : null
}

/** #rgb / #rrggbb / #rrggbbaa 형태의 hex 색만 허용(textColor 마크용). */
function safeColor(c: unknown): string | null {
  if (typeof c !== 'string') return null
  return /^#[0-9a-fA-F]{3,8}$/.test(c.trim()) ? c.trim() : null
}

/** attrs 에서 문자열 값을 안전하게 뽑는다. */
function attrStr(node: AdfNode, key: string): string | undefined {
  const v = node.attrs?.[key]
  return typeof v === 'string' ? v : undefined
}

/** attrs 에서 숫자 값을 안전하게 뽑는다. */
function attrNum(node: AdfNode, key: string): number | undefined {
  const v = node.attrs?.[key]
  return typeof v === 'number' ? v : undefined
}

/* ── 마크(inline 서식) 적용 ── */

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
      // 미지원 마크는 서식 없이 텍스트만 유지
      return html
  }
}

/** text 노드: escape 후 마크들을 안쪽→바깥쪽으로 감싼다. */
function renderText(node: AdfNode): string {
  let html = esc(node.text ?? '')
  for (const mark of node.marks ?? []) html = applyMark(html, mark)
  return html
}

/** 자식 배열을 이어붙인다. */
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
  const name = esc(attachment.filename || '첨부 파일')
  const id = esc(attachment.id)
  if (attachment.is_image && !compact) {
    return `<button type="button" class="adf-media-image" data-attachment-id="${id}" aria-label="${name} 크게 보기"><img src="${src}" alt="${name}" loading="lazy" decoding="async"></button>`
  }
  if (attachment.is_video && !compact) {
    return `<figure class="adf-media-video"><video src="${src}" controls preload="metadata" playsinline aria-label="${name}"></video><figcaption>${name}</figcaption></figure>`
  }
  return `<a class="adf-media" href="${src}" target="_blank" rel="noopener noreferrer">📎 ${name}</a>`
}

/** 패널 타입 → Tailwind 색 조합(테두리/배경 틴트/텍스트). */
const PANEL_STYLES: Record<string, string> = {
  info: 'border-status-new/40 bg-status-new/10',
  note: 'border-accent/40 bg-accent/10',
  success: 'border-status-done/40 bg-status-done/10',
  warning: 'border-status-stale/40 bg-status-stale/10',
  error: 'border-status-reopen/40 bg-status-reopen/10',
}

/** Jira status 노드 색 이름 → 배경 hex(칩). 키 lookup 이므로 안전. */
const STATUS_COLORS: Record<string, string> = {
  neutral: '#3a4048',
  grey: '#3a4048',
  purple: '#5b4b8a',
  blue: '#2a4b7c',
  red: '#7c2a2a',
  yellow: '#7c6a2a',
  green: '#2a5c3a',
}

/** 단일 노드를 HTML 로. */
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
      // codeBlock 자식은 (마크 없는) text 노드 — escape 만 하면 된다.
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
      const name = raw.replace(/^@/, '') || '불명'
      return `<span class="adf-mention">@${esc(name)}</span>`
    }

    case 'emoji': {
      // 우선순위: 이미 유니코드인 text → id(코드포인트) 변환 → shortName 폴백
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
      const name = attrStr(node, 'alt') ?? '첨부 파일'
      const link = opts.issueKey ? jiraBrowseUrl(opts.issueKey) : null
      const label = `📎 첨부: ${esc(name)}`
      return link
        ? `<a class="adf-media" href="${esc(link)}" target="_blank" rel="noopener noreferrer">${label}</a>`
        : `<span class="adf-media">${label}</span>`
    }

    case 'mediaInline': {
      const attachment = findAttachment(node, opts)
      if (attachment) return renderAttachment(attachment, true)
      const name = attrStr(node, 'alt') ?? '첨부'
      return `<span class="adf-media">📎 ${esc(name)}</span>`
    }

    default:
      // 미지원 노드: 텍스트가 있으면 escape, 없으면 자식 재귀 폴백
      if (typeof node.text === 'string') return esc(node.text)
      return renderChildren(node, opts)
  }
}

/** 코드포인트 문자열("1f604" 또는 "1f1f0-1f1f7") → 이모지. 실패 시 null. */
function codepointsToEmoji(id: string): string | null {
  try {
    const cps = id.split('-').map((h) => parseInt(h, 16))
    if (cps.some((n) => Number.isNaN(n) || n <= 0 || n > 0x10ffff)) return null
    return String.fromCodePoint(...cps)
  } catch {
    return null
  }
}

/** epoch(ms) 문자열 → YYYY-MM-DD. */
function formatEpoch(ts: string): string {
  const n = Number(ts)
  if (Number.isNaN(n)) return ts
  const d = new Date(n)
  if (Number.isNaN(d.getTime())) return ts
  return d.toISOString().slice(0, 10)
}

/**
 * ADF 문서(또는 인라인 노드) → HTML string.
 * 파싱 예외는 삼켜서 빈 문자열을 반환한다(호출부에서 평문 폴백 처리).
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
