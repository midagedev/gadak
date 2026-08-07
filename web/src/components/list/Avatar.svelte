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

  /*
    Initials fallback palette (8 steps). Rebuilt 2026-08-06 by search rather than
    by eye, against four things the old set broke:

      · White initials clear AA. Four of the eight old swatches did not —
        #ca8a04 measured 2.94:1. That caps L* near 49, since 4.5:1 against white
        means relative luminance 0.175.
      · One tonal family. The old spread was L* 43-62, so some avatars read as
        filled and others as washed out. This one is L* 43-50.
      · Red belongs to meaning, not to people. The old #dc2626 was the reopen
        badge's own red sitting in the same row as the badge, and #7c3aed
        reached chroma 102 — louder than any signal it shared a row with. The
        whole hue band around the reopen red is now excluded, brand indigo with
        it, and nothing exceeds chroma 72.
      · Spread around the wheel. Four of the old eight crowded the green-cyan
        band (hues 124/147/185/233, closest pair only dE2000 17.3); two do now.

    Worst pair here is dE2000 19.1. That is the measured ceiling, not a target
    missed: at L*~46 with AA-white initials, sRGB has little chroma left in cyan
    and blue, so eight slots cannot be pushed further apart without dropping to
    five slots (dE 33.7) or giving up one of the constraints above. The closest
    approach to a status color is #007eb6 against the "new" blue (dE 11), which
    survives because the two never share a shape — an 8px dot at the row's head
    against a 20px lettered disc at its tail.
  */
  const PALETTE = [
    '#ca045a',
    '#ba3a0c',
    '#a86800',
    '#6e6804',
    '#2a8800',
    '#00866c',
    '#007eb6',
    '#ac4ec4',
  ]
  const bg = $derived(PALETTE[colorIndex(email ?? name ?? '')])

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
  {:else if email || name}
    <span
      class="flex h-full w-full items-center justify-center font-semibold text-white"
      style:background={bg}
      style:font-size="{Math.max(10, Math.round(size * 0.5))}px"
    >
      {ini}
    </span>
  {:else}
    <span
      class="flex h-full w-full items-center justify-center border border-dashed border-[#5b6b80] text-text-muted"
      style:font-size="{Math.round(size * 0.5)}px"
      aria-hidden="true"
    >
      ·
    </span>
  {/if}
</svelte:element>
