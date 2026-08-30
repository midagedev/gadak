<script lang="ts">
  /*
   * ADF render container ([detail]).
   * Inserts renderAdf(HTML string) via {@html}; typography/spacing from :global(.adf).
   * Missing ADF or empty render falls back: plain fallback text → emptyLabel.
   * adf.ts try/catches render (empty string on throw) so we only branch on results.
   */
  import { t } from '../../lib/i18n'
  import type { AdfNode, DetailAttachment } from '../../lib/types'
  import { renderAdf, renderCommandBody } from '../../lib/adf'
  import { issueCommandBlocks } from '../../lib/issue-commands'
  import { placeInShell, shellForIssue } from '../../lib/issue-shells'
  import { shells } from '../../lib/issue-shells.svelte'
  import { terminalChrome } from '../../lib/terminal/pane.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'
  import { tryOpenNativeLink } from '../../lib/omnibox'
  import { mediaViewer } from '../../stores/media-viewer.svelte'
  import { write } from '../../stores/write.svelte'

  let {
    node = null,
    issueKey = undefined,
    attachments = [],
    fallback = null,
    emptyLabel = t('detail.noContent'),
    commands = false,
  }: {
    node?: AdfNode | null
    issueKey?: string
    attachments?: DetailAttachment[]
    fallback?: string | null
    emptyLabel?: string
    /** Offer a ▶ on single-line code blocks. Body only — see adf.ts. */
    commands?: boolean
  } = $props()

  const html = $derived(renderAdf(node, { issueKey, attachments, commands }))
  const hasHtml = $derived(html.trim().length > 0)
  const hasFallback = $derived(!!fallback && fallback.trim().length > 0)
  // A markdown body (Linear) has no ADF at all; its fences still deserve the
  // same code cards and the same ▶. Empty string when there is no fence, so
  // a prose-only body keeps the plain branch exactly as it was.
  const fallbackHtml = $derived(
    commands && !hasHtml && hasFallback ? renderCommandBody(fallback, { commands }) : '',
  )

  // The shell this issue's ▶ would reach, and whether the body has any.
  const shell = $derived(commands ? shellForIssue(shells.sessions, issueKey) : null)
  const runnable = $derived(
    commands && issueCommandBlocks(node, fallback).some((b) => b.runnable),
  )

  /**
   * Place the line, then show the shell holding it.
   *
   * Showing it is not decoration: the design is that a person reads the
   * command and presses Enter, which they cannot do if the shell it landed in
   * is off screen. Both verbs here already exist —
   * terminalChrome.toggle() opens the pane, and terminalSessions.select() is
   * the one owner of which session the pane is on — the same channel the
   * strip and a reopen-within-grace go through (lib/terminal/sessions.svelte).
   * Neither is invented and neither file is touched.
   *
   * A closed pane is pointed at the bound session before it opens, so it does
   * not create a fresh shell beside the one that now holds the line. An
   * already-open pane is left exactly as it is: closing it to re-target would
   * detach whatever the person is running, and a detached session is reaped
   * after the reconnect grace — spending someone's live shell to reveal a
   * command is a bad trade.
   */
  async function run(command: string): Promise<void> {
    const target = shell
    if (!target) return
    if (!(await placeInShell(target.id, command))) {
      write.toast(t('detail.placeFailed'), 'error')
      return
    }
    if (!terminalChrome.open) {
      terminalSessions.select(target.id)
      terminalChrome.toggle()
    }
  }

  function onContentClick(event: MouseEvent) {
    const target = event.target as HTMLElement | null
    const runner = target?.closest<HTMLElement>('[data-run-command]')
    if (runner) {
      event.preventDefault()
      // A disabled-looking ▶ is genuinely inert. It stays in the DOM and
      // stays focusable on purpose: hiding it would leave nowhere to learn
      // that shells attach to issues at all, which is the claim this product
      // makes and this is where it is taught.
      if (!shell) return
      void run(runner.dataset.runCommand ?? '')
      return
    }
    const trigger = target?.closest<HTMLElement>('[data-attachment-id]')
    if (trigger) {
      const id = trigger.dataset.attachmentId
      const attachment = attachments.find((item) => item.id === id)
      if (attachment) mediaViewer.open(attachment)
      return
    }
    const anchor = target?.closest('a[href]')
    if (!anchor) return
    const href = anchor.getAttribute('href') ?? ''
    if (tryOpenNativeLink(href)) {
      event.preventDefault()
      event.stopPropagation()
    }
  }
</script>

{#if hasHtml || fallbackHtml}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="adf"
    data-shell={commands ? (shell ? 'attached' : 'none') : undefined}
    data-testid={commands ? 'issue-body' : undefined}
    onclick={onContentClick}
  >
    {@html hasHtml ? html : fallbackHtml}
  </div>
{:else if hasFallback}
  <!-- Plain-text fallback when ADF missing/unparseable (preserve newlines) -->
  <div class="adf whitespace-pre-wrap">{fallback}</div>
{:else}
  <p class="text-micro text-text-muted italic">{emptyLabel}</p>
{/if}

{#if runnable && !shell}
  <!-- Why the ▶ is dead. One quiet line, and it names the way out: a body
       with commands in it and no shell to put them in is the exact moment
       someone can learn that `gadak claim` binds a pane to an issue. -->
  <p class="mt-2 text-micro text-text-muted" data-testid="no-shell-hint">
    {t('detail.noShellForIssue', { key: issueKey ?? '' })}
  </p>
{/if}

<style>
  /*
   * Nodes from {@html} are outside Svelte scoping — style with :global.
   * Colors/spacing use app.css @theme CSS vars (single source).
   */
  .adf :global(p) {
    margin: 0.55em 0;
    line-height: 1.62;
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
    margin: 1em 0 0.4em;
    font-weight: 600;
    line-height: 1.3;
    color: var(--color-text-primary);
  }
  /*
    Body headings must lose to the panel title (20px) that names the thing they
    are inside. They used to run 18/16/14/13 — the top of that ramp sat 2px under
    the panel title, near enough that a page opened looking like it had two
    titles. That was cut to 16/14/13/13, which fixed the top of the ramp and left
    the bottom of it broken: measured on the demo's pages, a body h2 rendered at
    14px directly above 13px paragraphs, a 1px step nobody can see. (2026-08-06)

    Revised the same day: every step is now at least 2px, and the sizes are the
    app's own tokens rather than numbers picked between them.

    Between body (13) and title (15) the scale has nothing, and it should not —
    14 next to 13 is the invisible step this revision exists to remove. So body
    headings get exactly two sizes and the rest of the ramp is weight and color,
    the technique the previous revision had already chosen for h4-h6:

      panel title  22 / 600 display
      h1           15 / 700 display
      h2           15 / 600 display
      h3-h6        13 / 600, secondary
      body         13 / 400, secondary

    h1 and h2 share a size because Confluence's editor makes h2 the top level
    most pages actually use — every page in the demo mirror is written that way.
    Mapping h2 down to body size to keep it under an h1 that pages rarely have
    would cost the common page its only heading size to serve the rare one.
  */
  .adf :global(h1) {
    font-family: var(--font-display);
    font-optical-sizing: auto;
    font-size: var(--text-title);
    font-weight: 700;
    letter-spacing: -0.015em;
    margin-top: 1.3em;
  }
  .adf :global(h2) {
    font-family: var(--font-display);
    font-optical-sizing: auto;
    font-size: var(--text-title);
    letter-spacing: -0.015em;
    margin-top: 1.2em;
  }
  .adf :global(h3),
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
  /* Inline code */
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
  /*
   * Runnable code block header (GDK-1162): the language badge and the ▶ share
   * one strip, so the button sits in the chrome the block already had rather
   * than adding a row. The badge keeps its own type; the strip takes over the
   * background and the rule under it, which is why .adf-code-head resets both
   * on the badge inside it.
   */
  .adf :global(.adf-code-head) {
    display: flex;
    align-items: center;
    /* No badge to push it: the ▶ still belongs on the right. */
    justify-content: flex-end;
    gap: 8px;
    padding-right: 3px;
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .adf :global(.adf-code-head:has(.adf-code-lang)) {
    justify-content: space-between;
  }
  .adf :global(.adf-code-head .adf-code-lang) {
    background: none;
    border-bottom: none;
  }
  /*
   * The same 24px icon-button the detail header uses (copy link, watch,
   * close) — no new control grammar for this. It is muted at rest because a
   * code block is content, not a call to action.
   */
  .adf :global(.adf-code-run) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 22px;
    height: 22px;
    margin: 2px 0;
    border-radius: 6px;
    color: var(--color-text-muted);
    transition:
      color 120ms,
      background-color 120ms;
  }
  .adf :global(.adf-code-run:hover) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }
  /*
   * No shell on this issue. Dimmed rather than removed: the button is where
   * someone learns that a shell can be bound to an issue at all, and the line
   * under the body says how. The click handler is the real gate — this is
   * only what that looks like.
   */
  .adf[data-shell='none'] :global(.adf-code-run) {
    /* The app's own disabled step (disabled:opacity-50 on every button in
       write/), not a number picked for this control. 0.4 was measured on the
       dark capture and read as nearly gone — a mark the line under the body
       points at has to be findable. */
    opacity: 0.5;
    cursor: not-allowed;
  }
  .adf[data-shell='none'] :global(.adf-code-run:hover) {
    background: none;
    color: var(--color-text-muted);
  }
  /* Prose between the fences of a markdown body (renderCommandBody). */
  .adf :global(.adf-plain) {
    white-space: pre-wrap;
    margin: 0.55em 0;
    line-height: 1.62;
    color: var(--color-text-primary);
  }
  .adf :global(.adf-plain:first-child) {
    margin-top: 0;
  }
  .adf :global(.adf-plain:last-child) {
    margin-bottom: 0;
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
  /* Mention chip */
  .adf :global(.adf-mention) {
    display: inline;
    padding: 0.05em 0.35em;
    border-radius: 4px;
    font-weight: 500;
    color: var(--color-accent-text);
    background: color-mix(in srgb, var(--color-accent) 22%, transparent);
  }
  /* Status chip */
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
  /* Panel */
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
  /* Attachment / inline card */
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
  /* Table */
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
  /* Task list */
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

  /*
   * The line under the cursor, marked (user request 2026-08-07: "people read
   * long text by dragging along it").
   *
   * The unit is the innermost block that holds text, which is what `:not(:has(…
   * :hover))` buys: a paragraph inside a list item highlights as the paragraph,
   * not as both, and a nested list marks the item you are on rather than every
   * ancestor of it. Without that guard the alphas stack and depth in the
   * document reads as emphasis.
   *
   * Blocks that carry a background already — code, tables, panels — are left
   * alone; a second translucent layer on top of them is mud, not a highlight.
   *
   * The fill is bg-hover at just over half strength, which is the row-hover
   * token diluted rather than a new color: a body paragraph is a much larger
   * area than a list row, and the full token over that area reads as a
   * selection someone made. Text color never moves, so contrast is whatever it
   * was (the muted-on-panel ratios are a contract).
   *
   * The 3px spread is the padding this cannot have: these elements' margins are
   * load-bearing typography, so the bleed goes in the shadow where it costs no
   * layout. No transition — following a cursor is not an animation, and a fade
   * here lags the eye it is supposed to be tracking.
   */
  @media (hover: hover) {
    .adf
      :global(
        :is(p, li, blockquote, h1, h2, h3, h4, h5, h6, .adf-task-item):hover:not(
            :has(:is(p, li, blockquote, h1, h2, h3, h4, h5, h6, .adf-task-item):hover)
          ):not(:is(.adf-panel, .adf-code, td, th) *)
      ) {
      background: color-mix(in srgb, var(--color-bg-hover) 55%, transparent);
      border-radius: 3px;
      box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-bg-hover) 55%, transparent);
    }
  }
</style>
