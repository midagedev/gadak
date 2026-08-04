<script lang="ts">
  /*
   * ADF 렌더 컨테이너 ([detail]).
   * renderAdf(HTML string) 을 {@html} 로 삽입하고, 타이포/여백은 아래 :global(.adf) 규칙이 담당.
   * ADF 가 없거나 렌더 결과가 비면 fallback 평문 → emptyLabel 순으로 폴백한다.
   * 렌더는 adf.ts 내부에서 try/catch 되어 예외 시 빈 문자열을 주므로 여기선 결과만 분기한다.
   */
  import { t } from '../../lib/i18n'
  import type { AdfNode, DetailAttachment } from '../../lib/types'
  import { renderAdf } from '../../lib/adf'
  import { mediaViewer } from '../../stores/media-viewer.svelte'

  let {
    node = null,
    issueKey = undefined,
    attachments = [],
    fallback = null,
    emptyLabel = t('detail.noContent'),
  }: {
    node?: AdfNode | null
    issueKey?: string
    attachments?: DetailAttachment[]
    fallback?: string | null
    emptyLabel?: string
  } = $props()

  let contentElement = $state<HTMLDivElement | null>(null)
  const html = $derived(renderAdf(node, { issueKey, attachments }))
  const hasHtml = $derived(html.trim().length > 0)
  const hasFallback = $derived(!!fallback && fallback.trim().length > 0)

  function onContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement | null
    const trigger = target?.closest<HTMLElement>('[data-attachment-id]')
    if (!trigger) return
    const id = trigger.dataset.attachmentId
    const attachment = attachments.find((item) => item.id === id)
    if (attachment) mediaViewer.open(attachment)
  }

  $effect(() => {
    const element = contentElement
    if (!element) return
    element.addEventListener('click', onContentClick)
    return () => element.removeEventListener('click', onContentClick)
  })
</script>

{#if hasHtml}
  <div class="adf" bind:this={contentElement}>{@html html}</div>
{:else if hasFallback}
  <!-- ADF 파싱 불가/부재 시 평문 폴백 (줄바꿈 보존) -->
  <div class="adf whitespace-pre-wrap">{fallback}</div>
{:else}
  <p class="text-[12px] text-text-muted italic">{emptyLabel}</p>
{/if}

<style>
  /*
   * {@html} 로 들어온 노드는 Svelte 스코핑 대상이 아니므로 :global 로 스타일링한다.
   * 색/간격은 app.css @theme 의 CSS 변수를 그대로 참조한다(단일 소스).
   */
  .adf :global(p) {
    margin: 0.5em 0;
    line-height: 1.6;
  }
  .adf :global(p:first-child) {
    margin-top: 0;
  }
  .adf :global(p:last-child) {
    margin-bottom: 0;
  }
  .adf :global(h1),
  .adf :global(h2),
  .adf :global(h3),
  .adf :global(h4),
  .adf :global(h5),
  .adf :global(h6) {
    margin: 1em 0 0.4em;
    font-weight: 600;
    line-height: 1.3;
    color: var(--color-text-primary);
  }
  .adf :global(h1) {
    font-size: 18px;
  }
  .adf :global(h2) {
    font-size: 16px;
  }
  .adf :global(h3) {
    font-size: 14px;
  }
  .adf :global(h4),
  .adf :global(h5),
  .adf :global(h6) {
    font-size: 13px;
  }
  .adf :global(a) {
    color: var(--color-accent-text);
    text-decoration: none;
  }
  .adf :global(a:hover) {
    text-decoration: underline;
  }
  .adf :global(strong) {
    font-weight: 600;
    color: var(--color-text-primary);
  }
  .adf :global(ul),
  .adf :global(ol) {
    margin: 0.5em 0;
    padding-left: 1.4em;
  }
  .adf :global(ul) {
    list-style: disc;
  }
  .adf :global(ol) {
    list-style: decimal;
  }
  .adf :global(li) {
    margin: 0.25em 0;
    line-height: 1.55;
  }
  .adf :global(li > p) {
    margin: 0;
  }
  .adf :global(blockquote) {
    margin: 0.6em 0;
    padding: 0.1em 0.9em;
    border-left: 3px solid var(--color-border-strong);
    color: var(--color-text-secondary);
  }
  .adf :global(hr) {
    margin: 1em 0;
    border: none;
    border-top: 1px solid var(--color-border-subtle);
  }
  /* 인라인 code */
  .adf :global(code) {
    font-family: var(--font-mono);
    font-size: 12px;
    padding: 0.1em 0.35em;
    border-radius: 4px;
    background: var(--color-bg-active);
    color: var(--color-accent-text);
  }
  /* code block */
  .adf :global(.adf-code) {
    position: relative;
    margin: 0.6em 0;
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    background: var(--color-bg-base);
    overflow: hidden;
  }
  .adf :global(.adf-code-lang) {
    display: block;
    padding: 0.25em 0.75em;
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .adf :global(.adf-code pre) {
    margin: 0;
    padding: 0.75em;
    overflow-x: auto;
  }
  .adf :global(.adf-code code) {
    padding: 0;
    background: none;
    color: var(--color-text-primary);
    font-size: 12px;
    line-height: 1.5;
  }
  /* 멘션 칩 */
  .adf :global(.adf-mention) {
    display: inline;
    padding: 0.05em 0.35em;
    border-radius: 4px;
    font-weight: 500;
    color: var(--color-accent-text);
    background: color-mix(in srgb, var(--color-accent) 22%, transparent);
  }
  /* 상태 칩 */
  .adf :global(.adf-status) {
    display: inline-block;
    padding: 0.05em 0.5em;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: #fff;
  }
  .adf :global(.adf-date) {
    color: var(--color-text-secondary);
  }
  /* 패널 */
  .adf :global(.adf-panel) {
    margin: 0.6em 0;
    padding: 0.6em 0.9em;
    border-width: 1px;
    border-style: solid;
    border-radius: 8px;
  }
  .adf :global(.adf-panel > p:first-child) {
    margin-top: 0;
  }
  .adf :global(.adf-panel > p:last-child) {
    margin-bottom: 0;
  }
  /* 첨부/인라인 카드 */
  .adf :global(.adf-media),
  .adf :global(.adf-inline-card) {
    display: inline-flex;
    align-items: center;
    gap: 0.3em;
    margin: 0.2em 0;
    padding: 0.2em 0.6em;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: 12px;
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .adf :global(.adf-media:hover),
  .adf :global(.adf-inline-card:hover) {
    background: var(--color-bg-hover);
  }
  .adf :global(.adf-media-block) {
    margin: 0.65em 0;
  }
  .adf :global(.adf-media-group) {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 6px;
    margin: 0.65em 0;
  }
  .adf :global(.adf-media-image) {
    display: block;
    width: 100%;
    max-height: 360px;
    overflow: hidden;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    background: var(--color-bg-base);
    cursor: zoom-in;
  }
  .adf :global(.adf-media-image:hover) {
    border-color: var(--color-border-strong);
  }
  .adf :global(.adf-media-image img) {
    display: block;
    width: 100%;
    max-height: 360px;
    object-fit: contain;
    background: #090b0d;
  }
  .adf :global(.adf-media-video) {
    margin: 0;
    overflow: hidden;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    background: #090b0d;
  }
  .adf :global(.adf-media-video video) {
    display: block;
    width: 100%;
    max-height: 360px;
  }
  .adf :global(.adf-media-video figcaption) {
    overflow: hidden;
    padding: 5px 8px;
    color: var(--color-text-muted);
    background: var(--color-bg-elevated);
    font-size: 11px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* 테이블 */
  .adf :global(.adf-table-wrap) {
    margin: 0.6em 0;
    overflow-x: auto;
  }
  .adf :global(table) {
    border-collapse: collapse;
    font-size: 12px;
    width: 100%;
  }
  .adf :global(th),
  .adf :global(td) {
    border: 1px solid var(--color-border-subtle);
    padding: 0.35em 0.6em;
    text-align: left;
    vertical-align: top;
  }
  .adf :global(th) {
    background: var(--color-bg-elevated);
    font-weight: 600;
    color: var(--color-text-primary);
  }
  /* 태스크 리스트 */
  .adf :global(.adf-task-item) {
    display: flex;
    gap: 0.4em;
    align-items: flex-start;
    margin: 0.15em 0;
  }
  .adf :global(.adf-task-box) {
    flex: none;
    color: var(--color-text-muted);
  }
  .adf :global(.adf-task-done) {
    color: var(--color-text-muted);
    text-decoration: line-through;
  }
  .adf :global(.adf-task-done .adf-task-box) {
    color: var(--color-status-done);
  }
</style>
