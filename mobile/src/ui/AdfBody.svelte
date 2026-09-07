<script lang="ts">
  /*
   * ADF body for the phone (GDK-1497): descriptions, comments, and wiki
   * pages rendered through the web's renderer (web/src/lib/adf.ts) instead
   * of flattened text. The renderer is pure — it takes its runtime config
   * as options — so this component is the phone's whole side of the parity:
   * pass the options, own the two things HTML cannot do by itself here
   * (attachment bytes and taps), and style the emitted nodes on phone
   * tokens.
   *
   * The fallback branch keeps what these screens already did with text:
   * bodyParagraphs' blank-line split, no markdown parser.
   */
  import { onDestroy } from 'svelte'
  import { fade } from 'svelte/transition'
  import { renderAdf } from '../../../web/src/lib/adf'
  import { API_V1, requestBlob } from '../lib/api'
  import { classifyAdfTarget } from '../lib/adf-links'
  import { openIssue } from '../lib/store.svelte'
  import { bodyParagraphs } from '../lib/domain'
  import { t } from '../lib/i18n'
  import type { AdfNode, DetailAttachment } from '../lib/types'

  let {
    doc = null,
    fallback = '',
    issueKey = undefined,
    attachments = [],
  }: {
    doc?: AdfNode | null
    fallback?: string
    issueKey?: string
    attachments?: DetailAttachment[]
  } = $props()

  // The same option vocabulary the web caller passes (AdfContent.svelte),
  // minus its two web-only members: `commands` stays off — the ▶ places a
  // line at a desktop shell's prompt — and `browseUrl` stays unset, so an
  // unresolved attachment renders as text, never as a link whose only
  // destination is leaving the app. Issue taps are recognized by href shape
  // instead (lib/adf-links.ts) because the renderer marks nothing.
  const html = $derived(
    renderAdf(doc, {
      commands: false,
      issueKey,
      attachments,
      apiBase: API_V1 + 'issues/',
    }),
  )
  const hasHtml = $derived(html.trim().length > 0)
  const paragraphs = $derived(bodyParagraphs(fallback ?? ''))

  /*
   * Attachment images. The renderer emits src="/api/v1/issues/…", a path
   * that is a real URL only through the dev proxy; the packaged app reaches
   * the paired endpoint through requestBlob (bearer included). So each img
   * parks its path in data-src and loses src — the alt text reads meanwhile
   * — and gets an object URL when the bytes arrive. A failed fetch leaves
   * the alt text standing; it is never a broken-image glyph, and a tap on
   * the image retries.
   *
   * Videos and file chips keep the renderer's href/src as-is (dev plays,
   * packaged does not) — bytes-through-blob for a whole video is a memory
   * bet this round does not take. Named as a gap in the round report.
   */
  let root = $state<HTMLElement | null>(null)
  let blobUrls: string[] = []

  function releaseBlobUrls(): void {
    for (const url of blobUrls) URL.revokeObjectURL(url)
    blobUrls = []
  }

  /** content_url → the path requestBlob dials (API_V1 is apiUrl's prefix). */
  function attachmentPath(attachment: DetailAttachment): string | null {
    if (!attachment.content_url.startsWith(API_V1)) return null
    return attachment.content_url.slice(API_V1.length)
  }

  async function loadAttachmentImage(img: HTMLImageElement, path: string): Promise<void> {
    try {
      const blob = await requestBlob(path)
      const url = URL.createObjectURL(blob)
      blobUrls.push(url)
      img.src = url
    } catch {
      // Alt text stays; a tap retries.
    }
  }

  function primeAttachmentImages(): void {
    if (!root) return
    for (const img of root.querySelectorAll<HTMLImageElement>('.adf-media-image img')) {
      const src = img.getAttribute('src')
      if (!src || img.dataset.src) continue
      img.dataset.src = src
      img.removeAttribute('src')
      const attachment = attachments.find((a) => a.content_url === src)
      const path = attachment && attachmentPath(attachment)
      if (path) void loadAttachmentImage(img, path)
    }
  }

  // Reads html so a body swap re-primes the new nodes and releases the old
  // body's object URLs. The images primed here live in {@html} — outside
  // Svelte's scoping, hence :global styles below.
  $effect(() => {
    void html
    primeAttachmentImages()
    return releaseBlobUrls
  })
  onDestroy(releaseBlobUrls)

  /*
   * One delegated tap handler (the same shape as AdfContent.svelte): the
   * classifier decides what a tap means. Issue → the same openIssue a list
   * row uses; anything else with an href → never navigate (the phone has no
   * opener plugin; adding one is a bigger decision than this round), copy
   * the URL and say so.
   */
  let copied = $state(false)
  let copiedTimer: ReturnType<typeof setTimeout> | undefined
  onDestroy(() => clearTimeout(copiedTimer))

  async function copyHref(href: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(href)
      copied = true
      clearTimeout(copiedTimer)
      copiedTimer = setTimeout(() => (copied = false), 1600)
    } catch {
      // Clipboard refused (no gesture, no permission): the link simply
      // stays unopened. The phone has no second channel to announce
      // through yet — a toast host is a lead decision.
    }
  }

  function onTap(event: MouseEvent): void {
    const target = event.target as HTMLElement | null
    const action = classifyAdfTarget(target)
    if (!action) return
    event.preventDefault()
    if (action.kind === 'issue') {
      openIssue(action.key)
      return
    }
    if (action.kind === 'attachment') {
      // No image viewer on the phone — the image is inline. The tap earns
      // its keep once: when the bytes never arrived, it retries.
      const trigger = target?.closest<HTMLElement>('[data-attachment-id]')
      const img = trigger?.querySelector<HTMLImageElement>('img[data-src]')
      if (img && !img.getAttribute('src')) {
        const attachment = attachments.find((a) => a.id === action.id)
        const path = attachment && attachmentPath(attachment)
        if (path) void loadAttachmentImage(img, path)
      }
      return
    }
    void copyHref(action.href)
  }
</script>

{#if hasHtml}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="adf" bind:this={root} onclick={onTap}>
    {@html html}
  </div>
{:else if paragraphs.length > 0}
  {#each paragraphs as p, i (i)}
    <p class="adf-plain">{p}</p>
  {/each}
{/if}
{#if copied}
  <span class="copied" transition:fade={{ duration: 100 }} role="status">{t('detail.linkCopied')}</span>
{/if}

<style>
  /*
   * The renderer's nodes are outside Svelte scoping — :global, like the
   * web's AdfContent. Colors/spacing ride the shared tokens (app.css
   * @theme); sizes are the phone's prose contract: body --text-body (16)
   * with a reading line-height, headings stepping 18/18/17 above it and
   * under the screen title (19). The web's panel tints are baked Tailwind
   * classes in the HTML (border-status-new/40 …) that the phone's Tailwind
   * root never scans — each panel is re-tinted by matching its escaped
   * class with the same token and the same /40 and /10 alphas those
   * utilities encode.
   */
  .adf {
    font-size: var(--text-body);
    line-height: 1.6;
    overflow-wrap: anywhere;
  }
  .adf :global(p) {
    margin: 0.9em 0;
    line-height: 1.6;
    color: var(--color-text-primary);
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
    margin: 1.5em 0 0.5em;
    font-weight: 600;
    line-height: 1.3;
    color: var(--color-text-primary);
  }
  .adf :global(h1) {
    font-family: var(--font-display);
    font-optical-sizing: auto;
    font-size: 18px;
    font-weight: 700;
    letter-spacing: -0.015em;
    margin-top: 1.4em;
  }
  .adf :global(h2) {
    font-family: var(--font-display);
    font-optical-sizing: auto;
    font-size: 18px;
    letter-spacing: -0.015em;
    margin-top: 1.4em;
  }
  .adf :global(h3) {
    font-size: 17px;
  }
  .adf :global(h4),
  .adf :global(h5),
  .adf :global(h6) {
    font-size: var(--text-body);
    color: var(--color-text-secondary);
  }
  .adf :global(h1:first-child),
  .adf :global(h2:first-child),
  .adf :global(h3:first-child) {
    margin-top: 0;
  }
  .adf :global(a) {
    color: var(--color-accent-text);
    text-decoration: none;
  }
  .adf :global(a:active) {
    text-decoration: underline;
  }
  @media (hover: hover) {
    .adf :global(a:hover) {
      text-decoration: underline;
    }
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
    margin: 0.3em 0;
    line-height: 1.6;
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
  .adf :global(code) {
    font-family: var(--font-mono);
    font-size: 0.92em;
    padding: 0.05em 0.3em;
    border-radius: 4px;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
  }
  .adf :global(.adf-code) {
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
    /* The web block size — between the phone's micro (12) and body (16). */
    font-size: 13px;
    line-height: 1.55;
  }
  .adf :global(.adf-mention) {
    display: inline;
    padding: 0.05em 0.35em;
    border-radius: 4px;
    font-weight: 500;
    color: var(--color-accent-text);
    background: color-mix(in srgb, var(--color-accent) 22%, transparent);
  }
  .adf :global(.adf-status) {
    display: inline-block;
    padding: 0.05em 0.5em;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    /* The web pins #fff on the lozenge fill; bg-base is that text in both
       themes (white ground light, dark ground dark). */
    color: var(--color-bg-base);
  }
  .adf :global(.adf-date) {
    color: var(--color-text-secondary);
  }
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
  .adf :global(.adf-panel.border-status-new\/40) {
    border-color: color-mix(in srgb, var(--color-status-new) 40%, transparent);
    background: color-mix(in srgb, var(--color-status-new) 10%, transparent);
  }
  .adf :global(.adf-panel.border-accent-text\/40) {
    border-color: color-mix(in srgb, var(--color-accent-text) 40%, transparent);
    background: color-mix(in srgb, var(--color-accent-text) 10%, transparent);
  }
  .adf :global(.adf-panel.border-status-done\/40) {
    border-color: color-mix(in srgb, var(--color-status-done) 40%, transparent);
    background: color-mix(in srgb, var(--color-status-done) 10%, transparent);
  }
  .adf :global(.adf-panel.border-status-stale\/40) {
    border-color: color-mix(in srgb, var(--color-status-stale) 40%, transparent);
    background: color-mix(in srgb, var(--color-status-stale) 10%, transparent);
  }
  .adf :global(.adf-panel.border-status-reopen\/40) {
    border-color: color-mix(in srgb, var(--color-status-reopen) 40%, transparent);
    background: color-mix(in srgb, var(--color-status-reopen) 10%, transparent);
  }
  .adf :global(.adf-media),
  .adf :global(.adf-inline-card) {
    display: inline-flex;
    align-items: center;
    gap: 0.3em;
    margin: 0.2em 0;
    padding: 0.2em 0.6em;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    padding: 0;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    background: var(--color-bg-base);
  }
  .adf :global(.adf-media-image img) {
    display: block;
    width: 100%;
    max-height: 360px;
    object-fit: contain;
    /* The web's plate hex (#090b0d) as the elevated token. */
    background: var(--color-bg-elevated);
  }
  .adf :global(.adf-media-video) {
    margin: 0;
    overflow: hidden;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    background: var(--color-bg-elevated);
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
  .adf :global(.adf-table-wrap) {
    margin: 0.6em 0;
    overflow-x: auto;
  }
  .adf :global(table) {
    border-collapse: collapse;
    font-size: 13px;
    line-height: 1.5;
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

  /* Fallback paragraphs: the screens' established text look, not the web's
     prose scale — a body with no ADF must not change appearance. */
  .adf-plain {
    margin: 0 0 12px;
    color: var(--color-text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .adf-plain:last-child {
    margin-bottom: 0;
  }

  /* Copy acknowledgment: a quiet pill above the composer slab. The inset
     rides the --safe-bottom token (app.css owns env() alone — §4.1). */
  .copied {
    position: fixed;
    left: 50%;
    transform: translateX(-50%);
    bottom: calc(max(var(--safe-bottom), 12px) + 76px);
    z-index: 40;
    padding: 6px 12px;
    border-radius: 9999px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    font-size: var(--text-micro);
    color: var(--color-text-primary);
  }
</style>
