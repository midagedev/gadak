<script lang="ts">
  /*
   * Hosted-demo-only GitHub / About surface (GDK-335). App mounts this when
   * VITE_HOSTED_DEMO=1 && isHostedDemo(); gadak serve never takes that branch.
   * Copy stays English — the public demo has no i18n.
   */
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'

  const REPO = 'https://github.com/midagedev/gadak'
  const DEMO_VIDEO = `${import.meta.env.BASE_URL}web-demo.mp4`
  const CLAIM =
    'A local SQLite file of your Jira — so "which epic is stuck?" is one query, not an unaskable one.'
  const BREW = 'brew install midagedev/tap/gadak'

  const POPOVER_LINKS: { href: string; label: string; external: boolean }[] = [
    { href: DEMO_VIDEO, label: 'Watch the 60s demo', external: true },
    { href: 'https://github.com/midagedev/gadak/issues', label: 'Report an issue', external: true },
    { href: 'mailto:midagedev@gmail.com', label: 'midagedev@gmail.com', external: false },
    { href: 'https://x.com/midagedev', label: '@midagedev', external: true },
  ]

  let open = $state(false)

  // Spend Esc so one keystroke cannot also clear the detail panel.
  // preventDefault is what DetailPanel declines; stopPropagation is what the
  // shell keymap needs — it does not read defaultPrevented, and its
  // svelte:window listener is registered first.
  function onEsc(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !open) return
    e.preventDefault()
    e.stopPropagation()
    open = false
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="absolute right-3 top-0 z-20 flex items-center gap-2 py-1.5"
  data-testid="hosted-links"
  onkeydown={onEsc}
  use:onEscape={onEsc}
  use:onOutsideClick={{ handler: () => (open = false), enabled: open }}
>
  <a
    href={REPO}
    target="_blank"
    rel="noopener noreferrer"
    class="whitespace-nowrap text-[12px] text-accent-text hover:underline"
    data-testid="hosted-links-github"
  >
    GitHub
  </a>
  <button
    type="button"
    class="whitespace-nowrap text-[12px] text-accent-text hover:underline"
    data-testid="hosted-links-about"
    aria-expanded={open}
    aria-haspopup="dialog"
    onclick={() => (open = !open)}
  >
    About
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 w-80 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-overlay"
      data-testid="hosted-links-popover"
      role="dialog"
      aria-label="About gadak"
    >
      <p class="px-2 py-1 text-[12px] text-text-primary">{CLAIM}</p>
      <pre
        class="my-1 cursor-text overflow-x-auto whitespace-nowrap rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono text-[12px] text-text-primary select-all">{BREW}</pre>
      {#each POPOVER_LINKS as link (link.href)}
        <a
          href={link.href}
          target={link.external ? '_blank' : undefined}
          rel={link.external ? 'noopener noreferrer' : undefined}
          class="flex min-h-control-sm w-full items-center rounded px-2 py-1 text-left text-[12px] text-accent-text hover:bg-bg-hover hover:underline"
        >
          {link.label}
        </a>
      {/each}
    </div>
  {/if}
</div>
