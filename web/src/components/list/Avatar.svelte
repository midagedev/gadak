<script lang="ts">
  /*
   * Assignee avatar ([explore]). profile_image first; initials on fail/missing.
   * Click = add assignee filter (parent handles onclick). Name in tooltip.
   */
  import { t } from '../../lib/i18n'
  import { issues } from '../../stores/issues.svelte'
  import { initials, colorIndex } from '../../lib/format'
  import { memberOrgColor, memberTooltip } from '../../lib/member-visual'

  let {
    email,
    accountId = null,
    name = null,
    size = 20,
    onclick,
  }: {
    email: string | null
    accountId?: string | null
    name?: string | null
    size?: number
    onclick?: (e: MouseEvent) => void
  } = $props()

  const member = $derived(
    (accountId ? issues.memberOfAccountId(accountId) : undefined) ??
      (email ? issues.memberOf(email) : undefined),
  )
  const displayName = $derived(member?.name ?? name ?? email ?? t('common.unassigned'))
  const img = $derived(member?.profile_image ?? null)
  const ini = $derived(initials(member?.name ?? name, email ?? accountId))
  const orgColor = $derived(memberOrgColor(member))
  const tooltip = $derived(memberTooltip(member, displayName))

  /*
    Initials fallback palette (8 steps). Hex lives in app.css (--color-avatar-*);
    light values keep the 2026-08-06/08-13 constraints (AA initials, one tonal
    family, no reopen-red / brand indigo, wheel spread). Dark lifts L only.
    Initials cut the page colour (--color-bg-base), not white.
  */
  const PALETTE = [
    'var(--color-avatar-0)',
    'var(--color-avatar-1)',
    'var(--color-avatar-2)',
    'var(--color-avatar-3)',
    'var(--color-avatar-4)',
    'var(--color-avatar-5)',
    'var(--color-avatar-6)',
    'var(--color-avatar-7)',
  ]
  const bg = $derived(PALETTE[colorIndex(accountId ?? email ?? name ?? '')])

  /* Which image URL failed, rather than a bare "did it fail" flag. This row is
     recycled through the virtual list, so the next member arrives in the same
     component instance — a flag would carry the previous one's broken image
     into it. Keyed on the URL, a new src simply is not the one that broke. */
  let brokenSrc = $state<string | null>(null)
  const broken = $derived(brokenSrc !== null && brokenSrc === img)

  const Tag = $derived(onclick ? 'button' : 'span')
</script>

<svelte:element
  this={Tag}
  type={onclick ? 'button' : undefined}
  role={onclick ? 'button' : undefined}
  {onclick}
  title={tooltip}
  class="inline-flex flex-none items-center justify-center overflow-hidden rounded-full align-middle {onclick
    ? 'cursor-pointer transition-transform hover:scale-110'
    : ''}"
  style:width="{size}px"
  style:height="{size}px"
  style:box-shadow={orgColor ? `0 0 0 1px ${orgColor}` : undefined}
>
  {#if img && !broken}
    <img
      src={img}
      alt={displayName}
      class="h-full w-full object-cover"
      loading="lazy"
      onerror={() => (brokenSrc = img)}
    />
  {:else if email || name || accountId}
    <span
      class="flex h-full w-full items-center justify-center font-semibold text-bg-base"
      style:background={bg}
      style:font-size="{Math.max(10, Math.round(size * 0.5))}px"
    >
      {ini}
    </span>
  {:else}
    <span
      class="flex h-full w-full items-center justify-center border border-dashed border-border-strong text-text-muted"
      style:font-size="{Math.round(size * 0.5)}px"
      aria-hidden="true"
    >
      ·
    </span>
  {/if}
</svelte:element>
