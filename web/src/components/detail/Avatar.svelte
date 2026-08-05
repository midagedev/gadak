<script lang="ts">
  /*
   * Avatar ([detail]). Member profile image; initials on fail/missing.
   * Build initials from name/email; use image when member (issues.memberOf) is present.
   */
  import type { Member } from '../../lib/types'
  import { memberOrgColor, memberTooltip } from '../../lib/member-visual'
  import { initials } from '../../lib/format'

  let {
    member = undefined,
    name = null,
    email = null,
    size = 20,
  }: {
    member?: Member | undefined
    name?: string | null
    email?: string | null
    size?: number
  } = $props()

  // Fall back to initials if the image fails to load
  let broken = $state(false)
  const src = $derived(member?.profile_image ?? null)
  const label = $derived(member?.display_name ?? member?.name ?? name ?? email ?? '')
  const ini = $derived(initials(member?.name ?? name, member?.email ?? email))
  const orgColor = $derived(memberOrgColor(member))
  const tooltip = $derived(memberTooltip(member, label))

  $effect(() => {
    void src
    broken = false
  })
</script>

<span
  class="inline-flex flex-none items-center justify-center overflow-hidden rounded-full bg-bg-active text-[10px] font-semibold text-text-secondary select-none"
  style:width="{size}px"
  style:height="{size}px"
  title={tooltip}
  style:box-shadow={orgColor ? `0 0 0 1px ${orgColor}` : undefined}
>
  {#if src && !broken}
    <img
      {src}
      alt={label}
      class="h-full w-full object-cover"
      loading="lazy"
      onerror={() => (broken = true)}
    />
  {:else}
    {ini}
  {/if}
</span>
