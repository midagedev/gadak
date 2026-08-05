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
    name = null,
    size = 20,
    onclick,
  }: {
    email: string | null
    name?: string | null
    size?: number
    onclick?: (e: MouseEvent) => void
  } = $props()

  const member = $derived(email ? issues.memberOf(email) : undefined)
  const displayName = $derived(member?.name ?? name ?? email ?? t('common.unassigned'))
  const img = $derived(member?.profile_image ?? null)
  const ini = $derived(initials(member?.name ?? name, email))
  const orgColor = $derived(memberOrgColor(member))
  const tooltip = $derived(memberTooltip(member, displayName))

  // Initials fallback palette (8 steps, dark tones)
  const PALETTE = [
    '#4f46e5',
    '#0891b2',
    '#0d9488',
    '#65a30d',
    '#ca8a04',
    '#dc2626',
    '#db2777',
    '#7c3aed',
  ]
  const bg = $derived(PALETTE[colorIndex(email ?? name ?? '')])

  let broken = $state(false)
  $effect(() => {
    // Reset broken state when email changes
    void email
    broken = false
  })

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
      onerror={() => (broken = true)}
    />
  {:else if email || name}
    <span
      class="flex h-full w-full items-center justify-center font-medium text-white"
      style:background={bg}
      style:font-size="{Math.round(size * 0.42)}px"
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
